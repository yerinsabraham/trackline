package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yerinsabraham/trackline/engine/internal/cloud/account"
	"github.com/yerinsabraham/trackline/engine/internal/cloud/outbox"
	"github.com/yerinsabraham/trackline/engine/internal/cloud/payload"
	"github.com/yerinsabraham/trackline/engine/internal/config"
	"github.com/yerinsabraham/trackline/engine/internal/event"
	"github.com/yerinsabraham/trackline/engine/internal/intent"
	"github.com/yerinsabraham/trackline/engine/internal/relay"
	"github.com/yerinsabraham/trackline/engine/internal/relay/held"
	"github.com/yerinsabraham/trackline/engine/internal/runner"
)

// live is one thing a host said during a turn that is not an action.
type live struct {
	kind, session, turn, cwd, text string
	transcript, helper             string
}

// parseLive recognises the events that keep a chat current.
func parseLive(raw []byte) (live, bool) {
	var p struct {
		Event   string `json:"hook_event_name"`
		Session string `json:"session_id"`
		Prompt  string `json:"prompt_id"`
		Turn    string `json:"turn_id"`
		CWD     string `json:"cwd"`
		Text    string `json:"prompt"`
		Type    string `json:"notification_type"`
		Message string `json:"message"`
		Tool    string `json:"tool_name"`
		Log     string `json:"transcript_path"`
		Helper  string `json:"agent_type"`
	}
	if json.Unmarshal(raw, &p) != nil {
		return live{}, false
	}
	l := live{session: p.Session, turn: first(p.Prompt, p.Turn), cwd: p.CWD, transcript: p.Log, helper: p.Helper}
	switch p.Event {
	case "UserPromptSubmit":
		l.kind, l.text = payload.LivePrompt, p.Text
	case "Notification":
		if p.Type != "permission_prompt" {
			return live{}, false
		}
		l.kind, l.text = payload.LiveWaiting, p.Message
	case "PermissionRequest":
		l.kind, l.text = payload.LiveWaiting, p.Tool
	case "PostToolUse":
		l.kind = payload.LiveResumed
	default:
		return live{}, false
	}
	return l, true
}

// hostOf maps a hook's host to the event's. Cursor keeps a chat current through
// its own events, so it has none here.
func hostOf(host string) (event.Host, bool) {
	switch host {
	case runner.HostClaude:
		return event.HostClaudeCode, true
	case runner.HostCodex:
		return event.HostCodex, true
	}
	return "", false
}

// mark is where this computer remembers something about a session, between
// hooks, under a name a session id cannot steer.
func mark(dir, session, what string) string {
	return filepath.Join(dir, "live", intent.PeerHash(session)+what)
}

// queueLive sends what a host said. A phone run has its own view through its
// job, and the words that start a turn nobody typed are not the person's.
func queueLive(root, hostName string, l live) {
	host, ok := hostOf(hostName)
	if !ok || l.session == "" || os.Getenv(config.RemoteEnv) != "" {
		return
	}
	dir, err := account.Dir()
	if err != nil {
		return
	}
	waiting := mark(dir, l.session, ".waiting")
	resumed := false
	switch l.kind {
	case payload.LiveResumed:
		resumed = os.Remove(waiting) == nil
	case payload.LiveWaiting:
		os.MkdirAll(filepath.Dir(waiting), 0o700)
		os.WriteFile(waiting, nil, 0o600)
	case payload.LivePrompt:
		held.Started(dir, l.session)
		os.Remove(waiting)
		if strings.HasPrefix(strings.TrimSpace(l.text), "<task-notification>") ||
			strings.HasPrefix(strings.TrimSpace(l.text), intent.PeerPrefix) {
			return
		}
	}
	if root == "" {
		root = l.cwd
	}
	if _, err := os.Stat(filepath.Join(dir, "credentials.json")); err != nil || root == "" {
		return
	}
	projectRoot, p, ok := account.ProjectFor(root)
	if !ok {
		return
	}
	box := outbox.Box{Dir: dir}
	if l.kind == payload.LiveResumed {
		// A host writes its transcript just after the hook before a step, so
		// the words that led up to it are read again once it has finished.
		queueSaid(box, dir, projectRoot, p, host, l.session, l.turn, l.transcript, l.helper)
	}
	if l.kind != payload.LiveResumed || resumed {
		o := payload.Options{
			Root: projectRoot, ProjectID: p.ID, ShareRequest: p.ShareRequests,
			ID: "lv_" + strings.TrimPrefix(outbox.NewID(), "ev_"),
		}
		if v, err := payload.BuildLive(host, l.session, l.turn, l.kind, l.text, time.Now(), o); err == nil {
			box.PutLive(v)
		}
	}
	if box.NeedsSender(time.Now()) {
		startSync()
	}
}

// sayTail is enough for the agent's last words, without paying for the session.
const sayTail = 128 << 10

// queueSaid sends what the agent last said, when that has changed. It runs
// before every action, so all it does otherwise is read a small piece of the
// transcript. A helper's actions name the same chat but are not its words.
func queueSaid(box outbox.Box, dir, root string, p account.Project, host event.Host, session, turn, transcript, helper string) {
	if !p.ShareReplies || transcript == "" || helper != "" || os.Getenv(config.RemoteEnv) != "" {
		return
	}
	said := intent.LastSaid(tailOf(transcript))
	if said == "" {
		return
	}
	seen := mark(dir, session, ".said")
	sum := intent.PeerHash(said)
	if b, err := os.ReadFile(seen); err == nil && string(b) == sum {
		return
	}
	v, err := payload.BuildLive(host, session, turn, payload.LiveSay, said, time.Now(), payload.Options{
		Root: root, ProjectID: p.ID, ShareRequest: p.ShareRequests,
		ID: "lv_" + strings.TrimPrefix(outbox.NewID(), "ev_"),
	})
	if err != nil || box.PutLive(v) != nil {
		return
	}
	os.MkdirAll(filepath.Dir(seen), 0o700)
	os.WriteFile(seen, []byte(sum), 0o600)
}

func tailOf(path string) []byte {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	if st, err := f.Stat(); err == nil && st.Size() > sayTail {
		if _, err := f.Seek(st.Size()-sayTail, io.SeekStart); err != nil {
			return nil
		}
	}
	b, _ := io.ReadAll(f)
	// The first line of a tail is usually cut. Only a whole line can be read.
	if i := bytes.IndexByte(b, '\n'); i >= 0 {
		return b[i+1:]
	}
	return b
}

// exitWake is what makes Claude Code wake the chat and read stderr.
const exitWake = 2

// waitForMessage runs in the background from the end of a turn until something
// ends it: a message from the phone, the next turn, or the chat closing. Nothing
// runs unless the person turned on messages to open chats at this computer.
func waitForMessage(raw []byte) {
	var p struct {
		Session string `json:"session_id"`
	}
	dir, err := account.Dir()
	if err != nil || json.Unmarshal(raw, &p) != nil || p.Session == "" {
		return
	}
	s, err := relay.Load()
	if err != nil || !s.OpenChats {
		return
	}
	began := time.Now()
	parent := os.Getppid()
	for time.Since(began) < held.Life {
		if text, ok := held.Take(dir, p.Session, time.Now()); ok {
			fmt.Fprint(os.Stderr, text)
			os.Exit(exitWake)
		}
		if held.StartedSince(dir, p.Session, began) || os.Getppid() != parent {
			return
		}
		held.Seen(dir, p.Session)
		time.Sleep(2 * time.Second)
	}
}
