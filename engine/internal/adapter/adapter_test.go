package adapter_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yerinsabraham/trackline/engine/internal/adapter/claudecode"
	"github.com/yerinsabraham/trackline/engine/internal/adapter/codex"
	"github.com/yerinsabraham/trackline/engine/internal/event"
)

func load(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return b
}

var now = time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)

// The payloads below are real captures from live sessions, with local paths
// scrubbed. Testing against invented payloads would only prove the adapter
// matches our idea of the format.

func TestClaudeRealPayload(t *testing.T) {
	ev, err := claudecode.Parse(load(t, "claude-pretool-write.json"), now)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if ev.Host != event.HostClaudeCode {
		t.Errorf("host = %q", ev.Host)
	}
	if ev.Phase != event.PhasePreTool {
		t.Errorf("phase = %q, want pre-tool", ev.Phase)
	}
	if !ev.CanBlock() {
		t.Error("a pre-tool event must be blockable")
	}
	if ev.SessionID == "" || ev.TurnID == "" || ev.TranscriptPath == "" {
		t.Errorf("missing grouping fields: session=%q turn=%q transcript=%q",
			ev.SessionID, ev.TurnID, ev.TranscriptPath)
	}
	if ev.Action.Type != event.ActionWriteFile {
		t.Errorf("action = %q, want write-file", ev.Action.Type)
	}
	if !ev.TouchesFiles() {
		t.Fatal("a Write must report the file it touches")
	}
}

func TestCodexRealPayload(t *testing.T) {
	ev, err := codex.Parse(load(t, "codex-pretool-applypatch.json"), now)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if ev.Host != event.HostCodex {
		t.Errorf("host = %q", ev.Host)
	}
	if ev.TurnID == "" {
		t.Error("turn_id must map onto TurnID, the way prompt_id does on Claude")
	}
	if ev.Action.Type != event.ActionWriteFile {
		t.Errorf("action = %q, want write-file for an Add File patch", ev.Action.Type)
	}
	if len(ev.Action.Paths) != 1 {
		t.Fatalf("paths = %v, want exactly the file the patch adds", ev.Action.Paths)
	}
	if filepath.Base(ev.Action.Paths[0]) != "FORBIDDEN-codex.txt" {
		t.Errorf("path = %q", ev.Action.Paths[0])
	}
}

// The property the whole engine rests on: the same fact, from either host,
// normalises to the same thing. If this breaks, every check downstream has to
// start caring which agent it is watching.
func TestHostsAgreeOnTheSameFact(t *testing.T) {
	claude := []byte(`{
		"hook_event_name":"PreToolUse","session_id":"s","prompt_id":"t",
		"tool_use_id":"u","tool_name":"Write","cwd":"/work/project",
		"transcript_path":"/tmp/t.jsonl",
		"tool_input":{"file_path":"src/app.ts","content":"x"}}`)
	cdx := []byte(`{
		"hook_event_name":"PreToolUse","session_id":"s","turn_id":"t",
		"tool_use_id":"u","tool_name":"apply_patch","cwd":"/work/project",
		"transcript_path":"/tmp/t.jsonl",
		"tool_input":{"command":"*** Begin Patch\n*** Add File: src/app.ts\n+x\n*** End Patch\n"}}`)

	a, err := claudecode.Parse(claude, now)
	if err != nil {
		t.Fatal(err)
	}
	b, err := codex.Parse(cdx, now)
	if err != nil {
		t.Fatal(err)
	}

	if a.Action.Type != b.Action.Type {
		t.Errorf("action type differs: claude=%q codex=%q", a.Action.Type, b.Action.Type)
	}
	if len(a.Action.Paths) != 1 || len(b.Action.Paths) != 1 || a.Action.Paths[0] != b.Action.Paths[0] {
		t.Errorf("paths differ: claude=%v codex=%v", a.Action.Paths, b.Action.Paths)
	}
	if a.SessionID != b.SessionID || a.TurnID != b.TurnID {
		t.Errorf("grouping differs: claude=%s/%s codex=%s/%s",
			a.SessionID, a.TurnID, b.SessionID, b.TurnID)
	}
	if a.Action.PathsUnknown || b.Action.PathsUnknown {
		t.Error("neither host should report paths unknown for a plain file write")
	}
}

// The dangerous reading is silence. If the adapter cannot see which files an
// action touches, it must say so, because a scope check handed an empty path
// list will pass the action quietly. These tests exist to make that impossible
// to regress.
func TestUnknownIsNeverReportedAsNothing(t *testing.T) {
	cases := []struct {
		name string
		host string
		raw  string
	}{
		{"claude shell command", "claude",
			`{"hook_event_name":"PreToolUse","tool_name":"Bash","cwd":"/w",
			  "tool_input":{"command":"rm -rf build && cp -r a b"}}`},
		{"claude unrecognised tool", "claude",
			`{"hook_event_name":"PreToolUse","tool_name":"SomeFutureTool","cwd":"/w",
			  "tool_input":{"whatever":"x"}}`},
		{"claude malformed tool input", "claude",
			`{"hook_event_name":"PreToolUse","tool_name":"Write","cwd":"/w",
			  "tool_input":"not-an-object"}`},
		{"codex shell command", "codex",
			`{"hook_event_name":"PreToolUse","tool_name":"shell","cwd":"/w",
			  "tool_input":{"command":"rm -rf build"}}`},
		{"codex unparseable patch", "codex",
			`{"hook_event_name":"PreToolUse","tool_name":"apply_patch","cwd":"/w",
			  "tool_input":{"command":"this is not a patch"}}`},
		{"codex patch with no file operations", "codex",
			`{"hook_event_name":"PreToolUse","tool_name":"apply_patch","cwd":"/w",
			  "tool_input":{"command":"*** Begin Patch\n*** End Patch\n"}}`},
		{"codex unrecognised tool", "codex",
			`{"hook_event_name":"PreToolUse","tool_name":"mcp__thing__do","cwd":"/w",
			  "tool_input":{"a":1}}`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var ev event.Event
			var err error
			if c.host == "claude" {
				ev, err = claudecode.Parse([]byte(c.raw), now)
			} else {
				ev, err = codex.Parse([]byte(c.raw), now)
			}
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if !ev.Action.PathsUnknown {
				t.Errorf("PathsUnknown = false; this action's files are not visible and must be reported as unknown")
			}
			if ev.TouchesFiles() {
				t.Error("TouchesFiles must be false when paths are unknown, so callers are forced to check")
			}
		})
	}
}

func TestPatchReportsMostDestructiveOperation(t *testing.T) {
	// A patch that edits one file and deletes another is a delete as far as any
	// policy decision is concerned.
	raw := []byte(`{"hook_event_name":"PreToolUse","tool_name":"apply_patch","cwd":"/w",
		"tool_input":{"command":"*** Begin Patch\n*** Update File: a.txt\n+x\n*** Delete File: b.txt\n*** End Patch\n"}}`)
	ev, err := codex.Parse(raw, now)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Action.Type != event.ActionDeleteFile {
		t.Errorf("action = %q, want delete-file: a mixed patch is judged by its most destructive operation", ev.Action.Type)
	}
	if len(ev.Action.Paths) != 2 {
		t.Errorf("paths = %v, want both files", ev.Action.Paths)
	}
}

func TestRelativePathsResolveAgainstCWD(t *testing.T) {
	raw := []byte(`{"hook_event_name":"PreToolUse","tool_name":"Write","cwd":"/work/project",
		"tool_input":{"file_path":"src/app.ts"}}`)
	ev, err := claudecode.Parse(raw, now)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Action.Paths[0] != "/work/project/src/app.ts" {
		t.Errorf("path = %q, want it resolved against cwd", ev.Action.Paths[0])
	}
}

func TestMalformedPayloadErrorsRatherThanGuessing(t *testing.T) {
	if _, err := claudecode.Parse([]byte(`{not json`), now); err == nil {
		t.Error("a malformed payload must error, not produce a half-filled event")
	}
	if _, err := codex.Parse([]byte(`{not json`), now); err == nil {
		t.Error("a malformed payload must error, not produce a half-filled event")
	}
}
