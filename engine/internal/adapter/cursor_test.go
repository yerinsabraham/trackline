package adapter_test

import (
	"path/filepath"
	"testing"

	"github.com/yerinsabraham/trackline/engine/internal/adapter/claudecode"
	"github.com/yerinsabraham/trackline/engine/internal/adapter/cursor"
	"github.com/yerinsabraham/trackline/engine/internal/event"
)

// Captured from a live Cursor CLI session, cursor_version 3.21.18, with local
// paths and the account email scrubbed.

func TestCursorRealWrite(t *testing.T) {
	ev, err := cursor.Parse(load(t, "cursor-pretool-write.json"), now)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if ev.Host != event.HostCursor {
		t.Errorf("host = %q", ev.Host)
	}
	if ev.Phase != event.PhasePreTool || !ev.CanBlock() {
		t.Errorf("phase = %q; preToolUse must be a blockable pre-tool event", ev.Phase)
	}
	if ev.SessionID == "" || ev.TurnID == "" || ev.TranscriptPath == "" {
		t.Errorf("missing grouping fields: session=%q turn=%q transcript=%q",
			ev.SessionID, ev.TurnID, ev.TranscriptPath)
	}
	// The agent called StrReplace. Cursor handed the hook a Write of the whole
	// file, so this is a write with the full new body, and the prior content
	// has to come from disk.
	if ev.Action.Type != event.ActionWriteFile {
		t.Errorf("action = %q, want write-file", ev.Action.Type)
	}
	if len(ev.Action.Paths) != 1 || ev.Action.Paths[0] != "/work/lab/greet.js" {
		t.Errorf("paths = %v", ev.Action.Paths)
	}
	if ev.Action.BodySize == 0 {
		t.Error("the written content must be carried as the body")
	}
	// cwd is absent on a Write; the workspace root stands in.
	if ev.CWD != "/work/lab" {
		t.Errorf("cwd = %q, want the first workspace root", ev.CWD)
	}
}

func TestCursorRealShell(t *testing.T) {
	ev, err := cursor.Parse(load(t, "cursor-pretool-shell.json"), now)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Action.Type != event.ActionRunCommand {
		t.Errorf("action = %q, want run-command", ev.Action.Type)
	}
	if ev.Action.Command == "" {
		t.Error("the command must be carried")
	}
	// node -e is not a form the shell parser claims to understand.
	if !ev.Action.PathsUnknown {
		t.Error("an unrecognised command must report its paths as unknown")
	}
}

func TestCursorRealRead(t *testing.T) {
	ev, err := cursor.Parse(load(t, "cursor-pretool-read.json"), now)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Action.Type != event.ActionReadFile {
		t.Errorf("action = %q, want read-file", ev.Action.Type)
	}
	if len(ev.Action.Paths) != 1 || filepath.Base(ev.Action.Paths[0]) != "farewell.js" {
		t.Errorf("paths = %v", ev.Action.Paths)
	}
}

func TestCursorMCPToolIsUnknownNotNothing(t *testing.T) {
	ev, err := cursor.Parse(load(t, "cursor-pretool-mcp.json"), now)
	if err != nil {
		t.Fatal(err)
	}
	if !ev.Action.PathsUnknown || ev.TouchesFiles() {
		t.Error("an MCP tool may touch anything, and must say it cannot tell")
	}
}

func TestCursorAgreesWithClaudeOnTheSameFact(t *testing.T) {
	claude := []byte(`{
		"hook_event_name":"PreToolUse","session_id":"s","prompt_id":"t",
		"tool_use_id":"u","tool_name":"Write","cwd":"/work/project",
		"transcript_path":"/tmp/t.jsonl",
		"tool_input":{"file_path":"src/app.ts","content":"x"}}`)
	cur := []byte(`{
		"hook_event_name":"preToolUse","conversation_id":"s","generation_id":"t",
		"tool_use_id":"u","tool_name":"Write","workspace_roots":["/work/project"],
		"transcript_path":"/tmp/t.jsonl","cursor_version":"3.21.18",
		"tool_input":{"file_path":"src/app.ts","content":"x"}}`)

	a, err := claudecode.Parse(claude, now)
	if err != nil {
		t.Fatal(err)
	}
	b, err := cursor.Parse(cur, now)
	if err != nil {
		t.Fatal(err)
	}
	if a.Action.Type != b.Action.Type {
		t.Errorf("action type differs: claude=%q cursor=%q", a.Action.Type, b.Action.Type)
	}
	if len(a.Action.Paths) != 1 || len(b.Action.Paths) != 1 || a.Action.Paths[0] != b.Action.Paths[0] {
		t.Errorf("paths differ: claude=%v cursor=%v", a.Action.Paths, b.Action.Paths)
	}
	if a.SessionID != b.SessionID || a.TurnID != b.TurnID {
		t.Errorf("grouping differs: claude=%s/%s cursor=%s/%s",
			a.SessionID, a.TurnID, b.SessionID, b.TurnID)
	}
	if a.Action.BodySize != b.Action.BodySize {
		t.Errorf("body size differs: claude=%d cursor=%d", a.Action.BodySize, b.Action.BodySize)
	}
}

func TestCursorUnknownIsNeverReportedAsNothing(t *testing.T) {
	cases := map[string]string{
		"unrecognised tool": `{"hook_event_name":"preToolUse","tool_name":"Grep",
			"workspace_roots":["/w"],"tool_input":{"pattern":"x"}}`,
		"write with no path": `{"hook_event_name":"preToolUse","tool_name":"Write",
			"workspace_roots":["/w"],"tool_input":{"content":"x"}}`,
		"malformed tool input": `{"hook_event_name":"preToolUse","tool_name":"Write",
			"workspace_roots":["/w"],"tool_input":"not-an-object"}`,
		"unrecognised command": `{"hook_event_name":"preToolUse","tool_name":"Shell",
			"workspace_roots":["/w"],"tool_input":{"command":"./deploy.sh --all"}}`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			ev, err := cursor.Parse([]byte(raw), now)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if !ev.Action.PathsUnknown || ev.TouchesFiles() {
				t.Error("this action's files are not visible and must be reported as unknown")
			}
		})
	}
}

// Reported from Cursor on Windows. A parser that fails on the byte-order mark
// falls through to allowing everything, silently.
func TestCursorByteOrderMarkIsTolerated(t *testing.T) {
	raw := append([]byte{0xEF, 0xBB, 0xBF}, load(t, "cursor-pretool-write.json")...)
	ev, err := cursor.Parse(raw, now)
	if err != nil {
		t.Fatalf("a leading BOM must not make the payload unreadable: %v", err)
	}
	if ev.Action.Type != event.ActionWriteFile {
		t.Errorf("action = %q", ev.Action.Type)
	}
}

// A shell command runs where its tool input says, which can differ from the
// workspace root.
func TestCursorShellResolvesAgainstItsOwnDirectory(t *testing.T) {
	raw := []byte(`{"hook_event_name":"preToolUse","tool_name":"Shell",
		"workspace_roots":["/w"],"tool_input":{"command":"echo x > out.txt","cwd":"/w/sub"}}`)
	ev, err := cursor.Parse(raw, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(ev.Action.Paths) != 1 || ev.Action.Paths[0] != "/w/sub/out.txt" {
		t.Errorf("paths = %v, want the redirect resolved against the command's cwd", ev.Action.Paths)
	}
}
