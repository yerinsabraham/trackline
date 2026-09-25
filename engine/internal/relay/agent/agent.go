// Package agent starts a coding agent for a remote job and reads what it
// does: the command line for each agent, and a parser that turns each
// agent's own stream into one small shape the phone can show.
//
// The command lines are fixed here. Nothing in a job reaches them: the
// instruction goes in on stdin, where text that looks like a flag is only
// text.
package agent

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
)

// Binaries are the executables looked for on the laptop, by job agent name.
var Binaries = map[string]string{"claude": "claude", "codex": "codex", "cursor": "cursor-agent"}

// Hosts maps a job's agent name to the name trackline's hook reports under,
// to check the hook is wired before an agent runs unwatched.
var Hosts = map[string]string{"claude": "claude-code", "codex": "codex", "cursor": "cursor"}

// ErrNotYet is an agent the phone can name that the runner does not start
// yet. Cursor waits on R0: its own worker may serve better than wrapping it.
var ErrNotYet = errors.New("remote does not start this agent yet")

// Command is the argv for one run, in the project at root. The prompt is not
// in it; it goes on stdin.
//
// Claude Code: edits inside the project are allowed, anything that would ask
// permission is refused (nobody is there to answer until R4), and only the
// project's settings load, so an allow rule written for local work in the
// user's global settings (an ssh to a server, a deploy) does not reach a
// remote job. trackline's hook lives in the project's settings, so it runs.
// Not --restricted: that skips project settings, and with them the hook.
//
// Codex: the workspace-write sandbox, set here so no config can widen it,
// and never asking. User config still loads, because hook trust lives there.
//
// session, when set, continues that session of the agent rather than
// starting one. `codex exec resume` takes no --sandbox or -C, so the sandbox
// is pinned by -c there and the folder is where the runner starts it.
// Measured: resumed that way, a write to the home folder was refused.
func Command(name, binary, root, session string) ([]string, error) {
	switch name {
	case "claude":
		argv := []string{binary, "-p",
			"--output-format", "stream-json", "--verbose",
			"--permission-mode", "acceptEdits",
			"--permission-prompts", "none",
			"--setting-sources", "project,local",
		}
		if session != "" {
			argv = append(argv, "--resume", session)
		}
		return argv, nil
	case "codex":
		if session != "" {
			return []string{binary, "exec", "resume", "--json",
				"-c", `sandbox_mode="workspace-write"`,
				"-c", `approval_policy="never"`,
				session, "-",
			}, nil
		}
		return []string{binary, "exec", "--json",
			"--sandbox", "workspace-write",
			"-c", `approval_policy="never"`,
			"-C", root,
			"-",
		}, nil
	case "cursor":
		return nil, ErrNotYet
	}
	return nil, errors.New("unknown agent: " + name)
}

// Event is one thing the agent did, as the phone shows it.
type Event struct {
	// Kind is session (the agent's own session id, in Text), say (the agent
	// talking), tool (an action: Tool names it, Text says on what),
	// permission (the runner thinks macOS may need local approval), denied
	// (an action refused for want of permission), done (the final answer), or
	// error.
	Kind string `json:"kind"`
	Text string `json:"text,omitempty"`
	Tool string `json:"tool,omitempty"`
}

// MaxText bounds one event. The phone shows the gist; the whole session is
// on the laptop.
const MaxText = 4000

func clip(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= MaxText {
		return s
	}
	cut := MaxText
	for cut > 0 && s[cut]&0xC0 == 0x80 {
		cut--
	}
	return s[:cut] + "…"
}

// Parser reads an agent's output one line at a time.
type Parser interface {
	Line(line []byte) []Event
}

// NewParser returns the parser for an agent. root makes paths relative, so
// the phone shows src/app.ts, not the laptop's home folder.
func NewParser(name, root string) Parser {
	if name == "codex" {
		return &codex{root: root}
	}
	return &claude{root: root}
}

func rel(root, p string) string {
	if r, err := filepath.Rel(root, p); err == nil && !strings.HasPrefix(r, "..") {
		return r
	}
	return p
}

// on says what a tool call was aimed at: a file, a command, a search.
func on(root string, input map[string]any) string {
	for _, k := range []string{"file_path", "notebook_path", "path"} {
		if v, ok := input[k].(string); ok && v != "" {
			return rel(root, v)
		}
	}
	for _, k := range []string{"command", "pattern", "url", "query", "description"} {
		if v, ok := input[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

type claude struct{ root string }

func (p *claude) Line(line []byte) []Event {
	var e struct {
		Type      string `json:"type"`
		Subtype   string `json:"subtype"`
		SessionID string `json:"session_id"`
		Message   struct {
			Content []struct {
				Type  string         `json:"type"`
				Text  string         `json:"text"`
				Name  string         `json:"name"`
				Input map[string]any `json:"input"`
			} `json:"content"`
		} `json:"message"`
		Result            string `json:"result"`
		IsError           bool   `json:"is_error"`
		PermissionDenials []struct {
			ToolName  string         `json:"tool_name"`
			ToolInput map[string]any `json:"tool_input"`
		} `json:"permission_denials"`
	}
	if json.Unmarshal(line, &e) != nil {
		return nil
	}
	var out []Event
	switch e.Type {
	case "system":
		if e.Subtype == "init" && e.SessionID != "" {
			out = append(out, Event{Kind: "session", Text: e.SessionID})
		}
	case "assistant":
		for _, c := range e.Message.Content {
			switch c.Type {
			case "text":
				if t := clip(c.Text); t != "" {
					out = append(out, Event{Kind: "say", Text: t})
				}
			case "tool_use":
				out = append(out, Event{Kind: "tool", Tool: c.Name, Text: clip(on(p.root, c.Input))})
			}
		}
	case "result":
		for _, d := range e.PermissionDenials {
			out = append(out, Event{Kind: "denied", Tool: d.ToolName, Text: clip(on(p.root, d.ToolInput))})
		}
		kind := "done"
		if e.IsError {
			kind = "error"
		}
		out = append(out, Event{Kind: kind, Text: clip(e.Result)})
	}
	return out
}

type codex struct {
	root string
	last string
}

func (p *codex) Line(line []byte) []Event {
	var e struct {
		Type     string `json:"type"`
		ThreadID string `json:"thread_id"`
		Message  string `json:"message"`
		Error    struct {
			Message string `json:"message"`
		} `json:"error"`
		Item struct {
			Type    string `json:"type"`
			Text    string `json:"text"`
			Command string `json:"command"`
			Changes []struct {
				Path string `json:"path"`
				Kind string `json:"kind"`
			} `json:"changes"`
		} `json:"item"`
	}
	if json.Unmarshal(line, &e) != nil {
		return nil
	}
	switch e.Type {
	case "thread.started":
		if e.ThreadID != "" {
			return []Event{{Kind: "session", Text: e.ThreadID}}
		}
	case "item.started":
		if e.Item.Type == "command_execution" {
			return []Event{{Kind: "tool", Tool: "shell", Text: clip(e.Item.Command)}}
		}
	case "item.completed":
		switch e.Item.Type {
		case "agent_message":
			p.last = clip(e.Item.Text)
			return []Event{{Kind: "say", Text: p.last}}
		case "file_change":
			var paths []string
			for _, c := range e.Item.Changes {
				paths = append(paths, rel(p.root, c.Path))
			}
			return []Event{{Kind: "tool", Tool: "edit", Text: clip(strings.Join(paths, ", "))}}
		}
	case "turn.completed":
		// Codex has no separate final answer: its last message is it.
		return []Event{{Kind: "done", Text: p.last}}
	case "turn.failed", "error":
		msg := e.Error.Message
		if msg == "" {
			msg = e.Message
		}
		return []Event{{Kind: "error", Text: clip(msg)}}
	}
	return nil
}
