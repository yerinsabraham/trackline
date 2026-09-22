// Package claudecode normalises a Claude Code hook payload into an event.
//
// Field names here were taken from payloads captured from a live session, not
// from documentation. The captured shape carried: cwd, effort, hook_event_name,
// permission_mode, prompt_id, scratchpad_dir, session_id, tool_input, tool_name,
// tool_use_id, transcript_path.
package claudecode

import (
	"encoding/json"
	"path/filepath"
	"time"

	"github.com/yerinsabraham/trackline/engine/internal/event"
)

type payload struct {
	HookEventName  string          `json:"hook_event_name"`
	SessionID      string          `json:"session_id"`
	PromptID       string          `json:"prompt_id"`
	ToolUseID      string          `json:"tool_use_id"`
	ToolName       string          `json:"tool_name"`
	CWD            string          `json:"cwd"`
	TranscriptPath string          `json:"transcript_path"`
	ToolInput      json.RawMessage `json:"tool_input"`
}

// toolInput covers the fields the file-touching tools use. A tool whose input
// matches none of these is reported with PathsUnknown rather than as touching
// nothing.
type toolInput struct {
	FilePath     string `json:"file_path"`
	Path         string `json:"path"`
	Command      string `json:"command"`
	NotebookPath string `json:"notebook_path"`

	// What is being written, under the name each tool uses for it.
	Content   string `json:"content"`
	NewString string `json:"new_string"`
	OldString string `json:"old_string"`
	NewSource string `json:"new_source"`
}

// body returns whichever field carries the written content.
func (in toolInput) body() string {
	for _, s := range []string{in.Content, in.NewString, in.NewSource} {
		if s != "" {
			return s
		}
	}
	return ""
}

var phases = map[string]event.Phase{
	"PreToolUse":       event.PhasePreTool,
	"PostToolUse":      event.PhasePostTool,
	"UserPromptSubmit": event.PhasePrompt,
	"Stop":             event.PhaseStop,
}

// actionTypes maps Claude's tool names onto normalised actions.
//
// Anything absent is ActionOther with PathsUnknown set, which is the honest
// answer for a tool the adapter has not been taught: it may well touch files
// and we cannot see them.
var actionTypes = map[string]event.ActionType{
	"Write":        event.ActionWriteFile,
	"Edit":         event.ActionEditFile,
	"NotebookEdit": event.ActionEditFile,
	"Read":         event.ActionReadFile,
	"Bash":         event.ActionRunCommand,
}

// Parse turns a raw Claude Code hook payload into an Event.
//
// It never returns a partially-populated event with a nil error: either the
// payload parsed, or the caller gets an error and decides. A hook that guesses
// on malformed input is worse than one that declines.
func Parse(raw []byte, now time.Time) (event.Event, error) {
	var p payload
	if err := json.Unmarshal(raw, &p); err != nil {
		return event.Event{}, err
	}

	phase, ok := phases[p.HookEventName]
	if !ok {
		phase = event.PhaseUnknown
	}

	return event.Event{
		ID:             p.ToolUseID,
		SessionID:      p.SessionID,
		TurnID:         p.PromptID,
		At:             now,
		Host:           event.HostClaudeCode,
		Phase:          phase,
		CWD:            p.CWD,
		TranscriptPath: p.TranscriptPath,
		Raw:            append([]byte(nil), raw...),
		Action:         action(p),
	}, nil
}

func action(p payload) event.Action {
	a := event.Action{ToolName: p.ToolName}

	t, known := actionTypes[p.ToolName]
	if !known {
		// An unrecognised tool might touch anything. Say so.
		a.Type = event.ActionOther
		a.PathsUnknown = true
		return a
	}
	a.Type = t

	var in toolInput
	if len(p.ToolInput) > 0 {
		if err := json.Unmarshal(p.ToolInput, &in); err != nil {
			a.PathsUnknown = true
			return a
		}
	}

	if t == event.ActionRunCommand {
		a.Command = in.Command
		// A shell command can touch anything. Extracting paths from arbitrary
		// shell is a separate problem, and it is not solved by pretending the
		// command touches nothing.
		a.PathsUnknown = true
		return a
	}

	a.Body, a.Truncated = event.TrimBody(in.body())
	if in.OldString != "" {
		a.PriorBody, _ = event.TrimBody(in.OldString)
	}

	for _, c := range []string{in.FilePath, in.Path, in.NotebookPath} {
		if c != "" {
			a.Paths = append(a.Paths, resolve(p.CWD, c))
		}
	}
	if len(a.Paths) == 0 {
		a.PathsUnknown = true
	}
	return a
}

func resolve(cwd, p string) string {
	if filepath.IsAbs(p) || cwd == "" {
		return filepath.Clean(p)
	}
	return filepath.Join(cwd, p)
}
