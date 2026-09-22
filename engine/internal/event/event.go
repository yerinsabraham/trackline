// Package event holds the one shape every host normalises into.
//
// A Claude Code hook event, a Codex hook event and an OpenTelemetry GenAI span
// all describe the same thing: an agent did something. Everything downstream
// reads this type and nothing downstream knows which host produced it. If a
// check has to ask "was this Claude or Codex?", the normalisation is wrong.
//
// The shape was designed against real captured payloads from both hosts rather
// than from documentation. See docs/experiments/01-do-agents-self-correct.md.
package event

import (
	"encoding/json"
	"time"
)

// Host is where an event came from.
type Host string

const (
	HostClaudeCode Host = "claude-code"
	HostCodex      Host = "codex"
	HostOTel       Host = "otel"
)

// Phase is when in the tool's life the event fired.
//
// Only PreToolUse can block, which is why it is the one with a latency budget.
type Phase string

const (
	PhasePreTool  Phase = "pre-tool"
	PhasePostTool Phase = "post-tool"
	PhasePrompt   Phase = "prompt"
	PhaseStop     Phase = "stop"
	PhaseUnknown  Phase = "unknown"
)

// ActionType is what the agent is doing, independent of what the host calls it.
//
// Claude's "Write" and Codex's "apply_patch" both land on ActionWriteFile. The
// host-specific tool name is preserved in Action.ToolName for evidence, but no
// check should switch on it.
type ActionType string

const (
	ActionWriteFile  ActionType = "write-file"
	ActionEditFile   ActionType = "edit-file"
	ActionReadFile   ActionType = "read-file"
	ActionDeleteFile ActionType = "delete-file"
	ActionRunCommand ActionType = "run-command"
	ActionCallTool   ActionType = "call-tool"
	ActionOther      ActionType = "other"
)

// Action is the normalised "what", and Paths is the field that earns this
// package's existence.
//
// "Did the agent touch a file outside what was asked for" has to be answerable
// the same way on every host. Claude hands over a structured file_path. Codex
// hands over a patch string that must be parsed. Both end up here as Paths, so
// the check is written once.
type Action struct {
	Type ActionType `json:"type"`

	// Paths are absolute where the host gave enough to resolve them.
	// Empty means the action touched no files, which is different from
	// PathsUnknown.
	Paths []string `json:"paths,omitempty"`

	// PathsUnknown is true when this action may touch files but the host did
	// not say which, or the payload could not be parsed.
	//
	// This exists because the alternative is reporting an empty Paths and
	// letting a scope check pass on an action nobody could see. Absence of
	// evidence is not evidence of absence, and a safety tool must not confuse
	// the two.
	PathsUnknown bool `json:"pathsUnknown,omitempty"`

	// Command is set for ActionRunCommand.
	Command string `json:"command,omitempty"`

	// ToolName is the host's own name for the tool, kept for evidence only.
	ToolName string `json:"toolName,omitempty"`

	// ToolNameUnknown is true when we know a tool was called but not which one.
	//
	// This is not hypothetical. A production OpenTelemetry trace with default
	// settings reports gen_ai.response.finish_reasons = ["tool_calls"] and
	// nothing else: the message content carrying the tool name is Opt-In and
	// off by default. So the most basic safety check there is — did the agent
	// call a forbidden tool — is unanswerable, and saying so is the only
	// honest option. See docs/experiments/03-otel-traces.md.
	ToolNameUnknown bool `json:"toolNameUnknown,omitempty"`

	// Truncated marks content the host or an exporter cut short. A rule read
	// from truncated content may be missing the clause that mattered, so a
	// check reading it must degrade rather than assume.
	Truncated bool `json:"truncated,omitempty"`
}

// Event is one thing an agent did.
type Event struct {
	// ID is unique per action where the host provides one.
	ID string `json:"id,omitempty"`

	// SessionID groups events in one agent session.
	SessionID string `json:"sessionId,omitempty"`

	// TurnID groups events under the user turn that caused them.
	//
	// Claude calls this prompt_id and Codex calls it turn_id; they mean the
	// same thing and this is the natural unit for "what did the agent do in
	// response to this request".
	TurnID string `json:"turnId,omitempty"`

	At    time.Time `json:"at"`
	Host  Host      `json:"host"`
	Phase Phase     `json:"phase"`

	// CWD is the working directory the agent reported.
	CWD string `json:"cwd,omitempty"`

	// TranscriptPath points at the host's session log. Both hook hosts supply
	// it on every event, and it is how intent is recovered.
	TranscriptPath string `json:"transcriptPath,omitempty"`

	Action Action `json:"action"`

	// Raw is the untouched host payload, kept so a wrong normalisation can be
	// diagnosed from a replay rather than reproduced live.
	Raw json.RawMessage `json:"raw,omitempty"`
}

// CanBlock reports whether intervening on this event is possible at all.
//
// Only a pre-tool event can be blocked. Anything else has already happened, so
// a check that fires there can warn but must not claim to have prevented
// something.
func (e Event) CanBlock() bool { return e.Phase == PhasePreTool }

// Observable reports whether enough is known about this action to judge it.
//
// False means a check should return CannotMeasure rather than Clean: the action
// happened and we could not see what it was.
//
// What counts as "enough" depends on the kind of action, and conflating them is
// a mistake this method made once. A tool call reveals no file paths on any
// surface, local or production, so demanding paths would mark every tool call
// unobservable. What matters for a tool call is the name; what matters for a
// file action is the paths; what matters for a command is the command.
func (e Event) Observable() bool {
	switch e.Action.Type {
	case ActionCallTool:
		return !e.Action.ToolNameUnknown && e.Action.ToolName != ""
	case ActionRunCommand:
		return e.Action.Command != ""
	case ActionWriteFile, ActionEditFile, ActionReadFile, ActionDeleteFile:
		return !e.Action.PathsUnknown && len(e.Action.Paths) > 0
	default:
		// An action we have not classified is one we cannot judge.
		return false
	}
}

// TouchesFiles reports whether this action is known to touch any file.
// It is deliberately false when paths are unknown: callers must consult
// Action.PathsUnknown and decide, rather than being handed a quiet no.
func (e Event) TouchesFiles() bool {
	return !e.Action.PathsUnknown && len(e.Action.Paths) > 0
}
