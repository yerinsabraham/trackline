// Package cursor normalises a Cursor hook payload into an event.
//
// Field names were taken from payloads captured from a live Cursor CLI session
// (cursor_version 3.21.18), not from documentation, and three of them
// contradicted it:
//
//   - A one-line edit arrives as Write with the whole new file. The agent
//     called StrReplace; Cursor rewrote it before the hook saw it. So the
//     prior content has to come from disk, as it does for Claude's Write.
//   - tool_use_id is not unique. A Read and the Write that followed it shared
//     one, and the ids contain newlines. Nothing may key on it.
//   - cwd is present only on Shell. Everything else is resolved against the
//     first workspace root.
//
// The turn grouping is generation_id, which is what Claude calls prompt_id and
// Codex calls turn_id. conversation_id is the session.
package cursor

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"time"

	"github.com/yerinsabraham/trackline/engine/internal/event"
	"github.com/yerinsabraham/trackline/engine/internal/shell"
)

type payload struct {
	HookEventName  string          `json:"hook_event_name"`
	ConversationID string          `json:"conversation_id"`
	GenerationID   string          `json:"generation_id"`
	ToolUseID      string          `json:"tool_use_id"`
	ToolName       string          `json:"tool_name"`
	CWD            string          `json:"cwd"`
	WorkspaceRoots []string        `json:"workspace_roots"`
	TranscriptPath *string         `json:"transcript_path"`
	ToolInput      json.RawMessage `json:"tool_input"`
}

type toolInput struct {
	FilePath string `json:"file_path"`
	Path     string `json:"path"`
	Command  string `json:"command"`
	CWD      string `json:"cwd"`
	Content  string `json:"content"`
}

// Only preToolUse is installed. Shell commands also fire beforeShellExecution,
// and hooking both would judge every command twice.
var phases = map[string]event.Phase{
	"preToolUse":         event.PhasePreTool,
	"postToolUse":        event.PhasePostTool,
	"beforeSubmitPrompt": event.PhasePrompt,
	"stop":               event.PhaseStop,
}

// bom is stripped before parsing. Cursor on Windows has been reported sending
// one at the start of stdin, and a parser that chokes on it falls through to
// allowing everything.
var bom = []byte{0xEF, 0xBB, 0xBF}

// Parse turns a raw Cursor hook payload into an Event.
func Parse(raw []byte, now time.Time) (event.Event, error) {
	var p payload
	if err := json.Unmarshal(bytes.TrimPrefix(raw, bom), &p); err != nil {
		return event.Event{}, err
	}

	phase, ok := phases[p.HookEventName]
	if !ok {
		phase = event.PhaseUnknown
	}

	cwd := p.CWD
	if cwd == "" && len(p.WorkspaceRoots) > 0 {
		cwd = p.WorkspaceRoots[0]
	}

	transcript := ""
	if p.TranscriptPath != nil {
		transcript = *p.TranscriptPath
	}

	return event.Event{
		ID:             p.ToolUseID,
		SessionID:      p.ConversationID,
		TurnID:         p.GenerationID,
		At:             now,
		Host:           event.HostCursor,
		Phase:          phase,
		CWD:            cwd,
		TranscriptPath: transcript,
		Raw:            append([]byte(nil), raw...),
		Action:         action(p, cwd),
	}, nil
}

func action(p payload, cwd string) event.Action {
	a := event.Action{ToolName: p.ToolName}

	var in toolInput
	if len(p.ToolInput) > 0 {
		if err := json.Unmarshal(p.ToolInput, &in); err != nil {
			a.Type = event.ActionOther
			a.PathsUnknown = true
			return a
		}
	}

	switch p.ToolName {
	case "Shell":
		a.Type = event.ActionRunCommand
		a.Command = in.Command
		// The command runs where the tool says, which is not always the
		// workspace root.
		dir := cwd
		if in.CWD != "" {
			dir = in.CWD
		}
		eff := shell.Parse(in.Command)
		for _, touched := range eff.Writes {
			a.Paths = append(a.Paths, resolve(dir, touched))
		}
		for _, touched := range eff.Deletes {
			a.Paths = append(a.Paths, resolve(dir, touched))
		}
		a.Installs = eff.Installs
		a.PathsUnknown = !eff.Understood
		return a

	case "Write":
		a.Type = event.ActionWriteFile
		a.SetBody(in.Content)
	case "Delete":
		// Not yet seen in a capture. Mapped from the documented tool name, and
		// if the path is not where expected it reports unknown below rather
		// than deleting nothing.
		a.Type = event.ActionDeleteFile
	case "Read":
		a.Type = event.ActionReadFile
	default:
		// MCP:<name>, Grep, Task and anything added later.
		a.Type = event.ActionOther
		a.PathsUnknown = true
		return a
	}

	for _, c := range []string{in.FilePath, in.Path} {
		if c != "" {
			a.Paths = append(a.Paths, resolve(cwd, c))
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
