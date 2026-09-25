// Command hook is the binary an agent runs before each tool call.
//
// It is the only part of trackline with a latency budget: it runs before every
// tool call, synchronously, and a real session makes hundreds. Measured
// startup is around 6.5ms compiled, against 88ms for the same logic in Node.
// See docs/experiments/02-hook-latency.md.
//
// Two rules govern everything here.
//
// It never panics. On Codex a hook that crashes or times out is treated as
// permission to proceed, so a crash does not fail safe, it fails silent.
//
// It never blocks unless configured to. Phase 2 ships warn-only: findings are
// recorded and the action proceeds. A tool that interrupts wrongly gets muted
// on the first day and uninstalled on the second, so the right to interrupt is
// earned with a measured false-alarm rate, not assumed.
//
// Usage, from a PreToolUse hook:
//
//	trackline-hook [-host auto|claude|codex|cursor] [-root DIR] [-log FILE]
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/yerinsabraham/trackline/engine/internal/cloud/account"
	"github.com/yerinsabraham/trackline/engine/internal/cloud/outbox"
	"github.com/yerinsabraham/trackline/engine/internal/cloud/payload"
	"github.com/yerinsabraham/trackline/engine/internal/event"
	"github.com/yerinsabraham/trackline/engine/internal/runner"
	"github.com/yerinsabraham/trackline/engine/internal/verdict"
)

// Exit codes, as the hosts read them.
const (
	exitAllow = 0 // proceed
	exitBlock = 2 // stop, and hand stderr back to the model
)

func main() {
	// Resolved before anything can fail, because how to say "allow" depends
	// on it.
	resolved := runner.HostAuto

	// Nothing below may take the process down. A crashed hook is a disabled
	// hook, and a disabled safety tool that nobody noticed is worse than no
	// safety tool at all.
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "trackline: internal error, allowing the action: %v\n", r)
			allow(resolved)
		}
	}()

	host := flag.String("host", runner.HostAuto, "claude, codex, cursor, or auto to detect")
	root := flag.String("root", "", "project root (defaults to the event's cwd)")
	logPath := flag.String("log", "", "findings log (defaults to <root>/.trackline/findings.jsonl)")
	flag.Parse()
	resolved = *host

	raw, err := io.ReadAll(os.Stdin)
	if err != nil || len(raw) == 0 {
		// No readable event means nothing to judge. Allow.
		allow(resolved)
	}
	if resolved == runner.HostAuto {
		resolved = runner.Detect(raw)
	}

	// The end of a turn: nothing to judge, only the agent's reply to keep.
	if t, ok := parseTurnEnd(raw, resolved); ok {
		queueReply(*root, t)
		allow(resolved)
	}

	d, err := runner.Run(raw, runner.Options{Host: resolved, Root: *root, Now: time.Now()})
	if err != nil {
		fmt.Fprintf(os.Stderr, "trackline: %v\n", err)
		allow(resolved)
	}

	record(*logPath, *root, d)
	queue(*root, d)

	if d.Block {
		block(resolved, d.Message)
	}
	allow(resolved)
}

// allow lets the action proceed.
//
// Cursor treats a permission hook's unreadable output as a denial, so it gets
// a JSON answer. That answer is an empty object, not "permission":"allow":
// an explicit allow can override Cursor's own approval prompt, and a watcher
// must never grant what the user would otherwise have been asked about.
func allow(host string) {
	if host == runner.HostCursor {
		fmt.Print("{}")
	}
	os.Exit(exitAllow)
}

// block stops the action and gives the agent the reason.
//
// Claude Code and Codex read the reason from stderr on exit 2. Cursor reads
// agent_message from JSON on stdout, which is the documented path for a reason
// the model sees; exit 2 there is also a denial, but carries no message.
func block(host, message string) {
	if host == runner.HostCursor {
		out, _ := json.Marshal(map[string]string{
			"permission":    "deny",
			"agent_message": message,
			"user_message":  message,
		})
		fmt.Print(string(out))
		os.Exit(exitAllow)
	}
	fmt.Fprint(os.Stderr, message)
	os.Exit(exitBlock)
}

// record appends findings to the log.
//
// Warn-only means the log is the product: it is what a false-alarm rate is
// measured from, and what `inspect` reads. Failing to write it must still not
// interfere with the user's work, so errors here are reported and swallowed.
func record(logPath, root string, d runner.Decision) {
	if logPath == "" {
		base := root
		if base == "" {
			base = d.Report.Event.CWD
		}
		if base == "" {
			return
		}
		logPath = filepath.Join(base, ".trackline", "findings.jsonl")
	}

	// Everything the check concluded is written, not just the findings.
	// A "cannot measure" is a result worth keeping: it is the difference
	// between an action that was checked and found clean, and one that was
	// never checked at all.
	entry := struct {
		At      time.Time        `json:"at"`
		Session string           `json:"session,omitempty"`
		Turn    string           `json:"turn,omitempty"`
		Host    string           `json:"host"`
		Tool    string           `json:"tool,omitempty"`
		Paths   []string         `json:"paths,omitempty"`
		Mode    string           `json:"mode"`
		Blocked bool             `json:"blocked"`
		Results []verdict.Result `json:"results"`
	}{
		At:      time.Now().UTC(),
		Session: d.Report.Event.SessionID,
		Turn:    d.Report.Event.TurnID,
		Host:    string(d.Report.Event.Host),
		Tool:    d.Report.Event.Action.ToolName,
		Paths:   d.Report.Event.Action.Paths,
		Mode:    string(d.Mode),
		Blocked: d.Block,
		Results: d.Report.Results,
	}

	b, err := json.Marshal(entry)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	f.Write(append(b, '\n'))
}

// queue hands the event to the outbox when this project is connected to an
// account, and starts a sender if none is running.
//
// Nothing here touches the network. A machine that was never connected pays
// for one failed stat; a connected one for two small reads, one small write
// and, now and then, starting a process it does not wait for. The hook's
// latency is measured with and without this in docs/experiments.
func queue(root string, d runner.Decision) {
	if root == "" {
		root = d.Report.Event.CWD
	}
	dir, err := account.Dir()
	if err != nil || root == "" {
		return
	}
	if _, err := os.Stat(filepath.Join(dir, "credentials.json")); err != nil {
		return
	}
	projectRoot, p, ok := account.ProjectFor(root)
	if !ok {
		return
	}
	ev, err := payload.Build(d.Report.Event, d.Report.Results, payload.Options{
		Root:         projectRoot,
		ProjectID:    p.ID,
		ShareRequest: p.ShareRequests,
		Mode:         string(d.Mode),
		Blocked:      d.Block,
		ID:           outbox.NewID(),
	})
	if err != nil {
		return
	}
	box := outbox.Box{Dir: dir}
	if box.Put(ev) != nil || !box.NeedsSender(time.Now()) {
		return
	}
	startSync()
}

// turnEnd is what the hosts send when the agent has finished answering.
type turnEnd struct {
	host          event.Host
	session, turn string
	cwd, text     string
}

// parseTurnEnd recognises the end of a turn. Claude Code and Codex send Stop
// with last_assistant_message (both checked by running them, 2026-09-25).
// Cursor sends afterAgentResponse with the text; its CLI does not fire it, so
// that path is unverified until seen from the editor.
func parseTurnEnd(raw []byte, host string) (turnEnd, bool) {
	var p struct {
		Event        string   `json:"hook_event_name"`
		Session      string   `json:"session_id"`
		Conversation string   `json:"conversation_id"`
		Prompt       string   `json:"prompt_id"`
		Turn         string   `json:"turn_id"`
		Generation   string   `json:"generation_id"`
		CWD          string   `json:"cwd"`
		Roots        []string `json:"workspace_roots"`
		Last         string   `json:"last_assistant_message"`
		Text         string   `json:"text"`
	}
	if json.Unmarshal(raw, &p) != nil {
		return turnEnd{}, false
	}
	switch {
	case p.Event == "Stop" && host == runner.HostCursor:
		// Cursor also runs Claude-format hooks, so its end of turn can
		// arrive as Stop. The backend keeps one reply per turn.
		text := p.Last
		if text == "" {
			text = p.Text
		}
		return turnEnd{event.HostCursor, first(p.Conversation, p.Session), p.Generation, first(p.CWD, firstOf(p.Roots)), text}, true
	case p.Event == "Stop" && host == runner.HostCodex:
		return turnEnd{event.HostCodex, p.Session, p.Turn, p.CWD, p.Last}, true
	case p.Event == "Stop":
		return turnEnd{event.HostClaudeCode, p.Session, p.Prompt, p.CWD, p.Last}, true
	case p.Event == "afterAgentResponse":
		return turnEnd{event.HostCursor, first(p.Conversation, p.Session), p.Generation, first(p.CWD, firstOf(p.Roots)), p.Text}, true
	}
	return turnEnd{}, false
}

func first(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func firstOf(s []string) string {
	if len(s) > 0 {
		return s[0]
	}
	return ""
}

// queueReply sends the reply the way queue sends an action, and only for a
// connected project that agreed to share its conversation.
func queueReply(root string, t turnEnd) {
	if root == "" {
		root = t.cwd
	}
	dir, err := account.Dir()
	if err != nil || root == "" {
		return
	}
	if _, err := os.Stat(filepath.Join(dir, "credentials.json")); err != nil {
		return
	}
	projectRoot, p, ok := account.ProjectFor(root)
	if !ok || !p.ShareReplies {
		return
	}
	r, err := payload.BuildReply(t.host, t.session, t.turn, t.text, time.Now(), payload.Options{
		Root:      projectRoot,
		ProjectID: p.ID,
		ID:        "rp_" + strings.TrimPrefix(outbox.NewID(), "ev_"),
	})
	if err != nil {
		return
	}
	box := outbox.Box{Dir: dir}
	if box.PutReply(r) != nil || !box.NeedsSender(time.Now()) {
		return
	}
	startSync()
}

// startSync runs `trackline sync` from beside this binary, detached. Its
// output goes nowhere: a host waits for the hook's stdout and stderr to close,
// and a child holding them open would make the hook as slow as the network.
func startSync() {
	self, err := os.Executable()
	if err != nil {
		return
	}
	name := "trackline"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	bin := filepath.Join(filepath.Dir(self), name)
	if _, err := os.Stat(bin); err != nil {
		return
	}
	cmd := exec.Command(bin, "sync", "--quiet")
	detach(cmd)
	if cmd.Start() == nil {
		cmd.Process.Release()
	}
}
