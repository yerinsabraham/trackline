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
	"path/filepath"
	"time"

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

	d, err := runner.Run(raw, runner.Options{Host: resolved, Root: *root, Now: time.Now()})
	if err != nil {
		fmt.Fprintf(os.Stderr, "trackline: %v\n", err)
		allow(resolved)
	}

	record(*logPath, *root, d)

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
