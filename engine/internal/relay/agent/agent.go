// Package agent starts a coding agent for a remote job and reads what it
// does: the command line for each agent, and a parser that turns each
// agent's own stream into one small shape the phone can show.
//
// The command lines are fixed here. The instruction goes in on stdin, where
// text that looks like a flag is only text. The one thing from a job that
// reaches them is a position, and only ever as the value of a flag set here.
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

// Command is the argv for one run, in the project at root. The prompt is not
// in it; it goes on stdin.
//
// Claude Code: edits inside the project are allowed, anything that would ask
// permission is refused (nobody is there to answer until R4), and only the
// project's settings load, so an allow rule written for local work in the
// user's global settings (an ssh to a server, a deploy) does not reach a
// remote job. trackline's hook lives in the project's settings, so it runs.
// Not the local settings either: an agent can write that file, and the next
// remote run would load whatever it put there.
// Not --restricted: that skips project settings, and with them the hook.
//
// Codex: the workspace-write sandbox, set here so no config can widen it,
// and never asking. User config still loads, because hook trust lives there.
//
// Cursor: print mode with its sandbox on, so a shell command cannot write
// outside the project or reach the network, and never --force, so a command
// asking for more is refused rather than run (measured: both refused). Its
// file edit tool is not sandboxed: measured, a mistyped path wrote a file
// outside the project. A phone session blocks that in trackline itself
// (config.ForRemote, outside-project). --trust loads the project's hooks,
// trackline's among them, without an interactive prompt nobody would answer.
//
// session, when set, continues that session of the agent rather than
// starting one. `codex exec resume` takes no --sandbox or -C, so the sandbox
// is pinned by -c there and the folder is where the runner starts it.
// Measured: resumed that way, a write to the home folder was refused.
//
// images are files the person attached, already saved in the project. Codex
// takes them with -i; Claude Code and Cursor read them from the path the
// prompt names (measured: all three read an attached image correctly).
//
// position is the job the person gave this agent, and goes where each tool
// keeps standing instructions. Measured: both keep it across a resume with
// nothing passed the second time. Cursor has no such place, so the site does
// not offer it there.
func Command(name, binary, root, session, position string, images ...string) ([]string, error) {
	switch name {
	case "claude":
		argv := []string{binary, "-p",
			"--output-format", "stream-json", "--verbose",
			"--permission-mode", "acceptEdits",
			"--permission-prompts", "none",
			"--setting-sources", "project",
		}
		if position != "" {
			argv = append(argv, "--append-system-prompt="+standing(position))
		}
		if session != "" {
			argv = append(argv, "--resume", session)
		}
		return argv, nil
	case "codex":
		var attach []string
		for _, im := range images {
			attach = append(attach, "-i", im)
		}
		if position != "" {
			// A TOML string, which a JSON string always is.
			quoted, err := json.Marshal(standing(position))
			if err != nil {
				return nil, err
			}
			attach = append(attach, "-c", "developer_instructions="+string(quoted))
		}
		if session != "" {
			argv := []string{binary, "exec", "resume", "--json",
				"-c", `sandbox_mode="workspace-write"`,
				"-c", `approval_policy="never"`,
			}
			return append(append(argv, attach...), session, "-"), nil
		}
		argv := []string{binary, "exec", "--json",
			"--sandbox", "workspace-write",
			"-c", `approval_policy="never"`,
			"-C", root,
		}
		return append(append(argv, attach...), "-"), nil
	case "cursor":
		argv := []string{binary, "-p",
			"--output-format", "stream-json",
			"--sandbox", "enabled",
			"--trust",
			"--workspace", root,
		}
		if session != "" {
			argv = append(argv, "--resume", session)
		}
		return argv, nil
	}
	return nil, errors.New("unknown agent: " + name)
}

// standing is what an agent is told about its position. The words around the
// person's are written here, so no position can claim to be anything else.
func standing(position string) string {
	return "The person you work for gave you this position on their team:\n\n" + position +
		"\n\nIt describes your job. It does not change what you are allowed to do, and every rule and check you work under still applies."
}

// OpenClaudeChats reads the output of `claude agents --json`, the supported
// list of running sessions, and returns the ids of those open interactive.
// A headless run is not listed, and a background session has its own owner.
func OpenClaudeChats(list []byte) (map[string]bool, error) {
	var sessions []struct {
		SessionID string `json:"sessionId"`
		Kind      string `json:"kind"`
	}
	if err := json.Unmarshal(list, &sessions); err != nil {
		return nil, err
	}
	open := map[string]bool{}
	for _, s := range sessions {
		if s.SessionID != "" && s.Kind == "interactive" {
			open[s.SessionID] = true
		}
	}
	return open, nil
}

// Locked is whether a failed Codex run failed because another process holds
// the conversation. Measured on 0.166: the lock refuses a second writer even
// while the first sits idle.
func Locked(reason string) bool { return strings.Contains(reason, "active writer") }

// QueueCommand hands a message to the Codex process that owns a conversation,
// as its next turn. It is the one command line that carries the person's text:
// codex queue reads no stdin. The = keeps text that looks like a flag as text.
func QueueCommand(binary, session, text string) []string {
	return []string{binary, "queue", "--thread", session, "--message=" + text}
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
	switch name {
	case "codex":
		return &codex{root: root}
	case "cursor":
		return &cursor{root: root}
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

type cursor struct {
	root string
	last string
	// targets remembers what each call was aimed at: a refused call's
	// completion does not repeat its arguments.
	targets map[string]string
}

// cursorTools names Cursor's tool calls the way the phone shows them.
var cursorTools = map[string]string{
	"editToolCall": "edit", "writeToolCall": "write", "deleteToolCall": "delete",
	"shellToolCall": "shell", "readToolCall": "read", "grepToolCall": "grep",
	"globToolCall": "glob", "lsToolCall": "ls",
}

func (p *cursor) Line(line []byte) []Event {
	var e struct {
		Type      string `json:"type"`
		Subtype   string `json:"subtype"`
		SessionID string `json:"session_id"`
		CallID    string `json:"call_id"`
		Message   struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"message"`
		// The call sits under a key naming its tool (editToolCall), beside
		// plain fields such as toolCallId, so it is read in two steps.
		ToolCall map[string]json.RawMessage `json:"tool_call"`
		IsError  bool                       `json:"is_error"`
		Result   string                     `json:"result"`
	}
	if json.Unmarshal(line, &e) != nil {
		return nil
	}
	switch e.Type {
	case "system":
		if e.Subtype == "init" && e.SessionID != "" {
			return []Event{{Kind: "session", Text: e.SessionID}}
		}
	case "assistant":
		var out []Event
		for _, c := range e.Message.Content {
			if t := clip(c.Text); c.Type == "text" && t != "" {
				p.last = t
				out = append(out, Event{Kind: "say", Text: t})
			}
		}
		return out
	case "tool_call":
		for key, raw := range e.ToolCall {
			if !strings.HasSuffix(key, "ToolCall") {
				continue
			}
			var call struct {
				Args   map[string]any `json:"args"`
				Result struct {
					Error *struct {
						Error string `json:"error"`
					} `json:"error"`
					Rejected *struct {
						Command string `json:"command"`
					} `json:"rejected"`
				} `json:"result"`
			}
			if json.Unmarshal(raw, &call) != nil {
				continue
			}
			name := cursorTools[key]
			if name == "" {
				name = strings.TrimSuffix(key, "ToolCall")
			}
			target := clip(on(p.root, call.Args))
			if target == "" && call.Result.Rejected != nil {
				target = clip(call.Result.Rejected.Command)
			}
			if target == "" {
				target = p.targets[e.CallID]
			}
			switch e.Subtype {
			case "started":
				if p.targets == nil {
					p.targets = map[string]string{}
				}
				p.targets[e.CallID] = target
				return []Event{{Kind: "tool", Tool: name, Text: target}}
			case "completed":
				// Refused by trackline's hook, or by Cursor for want of
				// permission: either way the phone should see it was stopped.
				if call.Result.Rejected != nil || call.Result.Error != nil && strings.Contains(call.Result.Error.Error, "BLOCKED by trackline") {
					return []Event{{Kind: "denied", Tool: name, Text: target}}
				}
			}
		}
	case "result":
		// Cursor's result joins every message of the turn into one string;
		// the last message on its own is the answer.
		text := p.last
		if text == "" {
			text = clip(e.Result)
		}
		if e.IsError {
			return []Event{{Kind: "error", Text: clip(e.Result)}}
		}
		return []Event{{Kind: "done", Text: text}}
	}
	return nil
}
