// Package codex normalises a Codex hook payload into an event.
//
// Field names were taken from a payload captured from a live Codex session. The
// captured shape carried: cwd, hook_event_name, model, permission_mode,
// session_id, tool_input, tool_name, tool_use_id, transcript_path, turn_id.
//
// Codex and Claude Code share eight of those keys. The one that differs by name
// is the turn grouping: Codex calls it turn_id, Claude calls it prompt_id, and
// they mean the same thing.
package codex

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"time"

	"github.com/yerinsabraham/trackline/engine/internal/event"
	"github.com/yerinsabraham/trackline/engine/internal/shell"
)

type payload struct {
	HookEventName  string          `json:"hook_event_name"`
	SessionID      string          `json:"session_id"`
	TurnID         string          `json:"turn_id"`
	ToolUseID      string          `json:"tool_use_id"`
	ToolName       string          `json:"tool_name"`
	CWD            string          `json:"cwd"`
	TranscriptPath string          `json:"transcript_path"`
	ToolInput      json.RawMessage `json:"tool_input"`
}

type toolInput struct {
	Command  string `json:"command"`
	FilePath string `json:"file_path"`
	Path     string `json:"path"`
}

var phases = map[string]event.Phase{
	"PreToolUse":       event.PhasePreTool,
	"PostToolUse":      event.PhasePostTool,
	"UserPromptSubmit": event.PhasePrompt,
	"Stop":             event.PhaseStop,
}

// Parse turns a raw Codex hook payload into an Event.
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
		TurnID:         p.TurnID,
		At:             now,
		Host:           event.HostCodex,
		Phase:          phase,
		CWD:            p.CWD,
		TranscriptPath: p.TranscriptPath,
		Raw:            append([]byte(nil), raw...),
		Action:         action(p),
	}, nil
}

func action(p payload) event.Action {
	a := event.Action{ToolName: p.ToolName}

	var in toolInput
	if len(p.ToolInput) > 0 {
		if err := json.Unmarshal(p.ToolInput, &in); err != nil {
			a.Type = event.ActionOther
			a.PathsUnknown = true
			return a
		}
	}

	// Lowercased: codex-cli 0.155 names its shell tool "Bash", where the
	// Phase 0 capture had "shell". Matching case exactly turned every command
	// in a session into an action nobody could read.
	switch strings.ToLower(p.ToolName) {
	case "apply_patch":
		// The patch is both the instruction and the content, so it is the body.
		a.SetBody(in.Command)
		ops, ok := parsePatch(in.Command)
		if !ok {
			// A patch that would not parse may have touched anything.
			a.Type = event.ActionWriteFile
			a.PathsUnknown = true
			return a
		}
		// A patch can mix adds, edits and deletes. The action type reports the
		// most destructive operation present, because that is the one a policy
		// decision should be made against.
		a.Type = event.ActionEditFile
		for _, op := range ops {
			a.Paths = append(a.Paths, resolve(p.CWD, op.path))
			if rank(op.action) > rank(a.Type) {
				a.Type = op.action
			}
		}
		return a

	case "shell", "local_shell", "bash":
		a.Type = event.ActionRunCommand
		a.Command = in.Command
		// A shell command is how most of a session actually happens, and
		// treating it all as unknowable made whole sessions unexaminable.
		// Recognised forms give real paths; anything unrecognised still
		// reports unknown, because claiming to have read a command we did not
		// would turn an unexamined action into a safe-looking one.
		eff := shell.Parse(in.Command)
		for _, touched := range eff.Writes {
			a.Paths = append(a.Paths, resolve(p.CWD, touched))
		}
		for _, touched := range eff.Deletes {
			a.Paths = append(a.Paths, resolve(p.CWD, touched))
		}
		a.Installs = eff.Installs
		a.PathsUnknown = !eff.Understood
		return a

	case "read_file", "read":
		a.Type = event.ActionReadFile
	default:
		a.Type = event.ActionOther
		a.PathsUnknown = true
		return a
	}

	for _, c := range []string{in.FilePath, in.Path} {
		if c != "" {
			a.Paths = append(a.Paths, resolve(p.CWD, c))
		}
	}
	if len(a.Paths) == 0 {
		a.PathsUnknown = true
	}
	return a
}

// rank orders action types by how hard they are to undo, so a mixed patch is
// reported as its most destructive operation.
func rank(t event.ActionType) int {
	switch t {
	case event.ActionDeleteFile:
		return 3
	case event.ActionWriteFile:
		return 2
	case event.ActionEditFile:
		return 1
	default:
		return 0
	}
}

func resolve(cwd, p string) string {
	if filepath.IsAbs(p) || cwd == "" {
		return filepath.Clean(p)
	}
	return filepath.Join(cwd, p)
}
