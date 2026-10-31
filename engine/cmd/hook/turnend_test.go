package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/yerinsabraham/trackline/engine/internal/event"
	"github.com/yerinsabraham/trackline/engine/internal/runner"
)

// Captured from Claude Code and Codex running for real, 2026-09-25, with a
// hook that saved its input. Paths shortened; nothing else changed.
func TestTurnEndsAreRecognisedAsTheHostsSendThem(t *testing.T) {
	b, err := os.ReadFile("testdata/turn-ends.json")
	if err != nil {
		t.Fatal(err)
	}
	var caps map[string]json.RawMessage
	json.Unmarshal(b, &caps)

	for name, want := range map[string]turnEnd{
		"claude-stop": {event.HostClaudeCode, "8ae3ba4e-dbd9-4f00-8bbc-8a035fd7ba98", "9420d9d8-3508-4c2b-8a0a-4b0dddaa55ca", "/work/lab", "The capital of France is Paris.", ""},
		"codex-stop":  {event.HostCodex, "01a0d85c-41ac-77d0-890e-4ee49a2963d1", "01a0d85c-42d3-76f0-986c-709d511b56b2", "/work/lab", "The capital of France is Paris.", ""},
	} {
		raw := caps[name]
		got, ok := parseTurnEnd(raw, runner.Detect(raw))
		// Claude Code says where its transcript is; Codex's Stop may not, and
		// codexTranscript finds it.
		if got.transcript == "" && name == "claude-stop" {
			t.Errorf("%s: no transcript, so a turn answered in words loses what was asked", name)
		}
		got.transcript = ""
		if !ok || got != want {
			t.Errorf("%s: got %+v %v, want %+v", name, got, ok, want)
		}
	}
	if _, ok := parseTurnEnd(caps["codex-pre"], runner.HostCodex); ok {
		t.Error("an action was taken for the end of a turn")
	}
}

func TestCursorReplyArrivesEitherWay(t *testing.T) {
	for _, raw := range []string{
		`{"hook_event_name":"afterAgentResponse","conversation_id":"c1","generation_id":"g1","workspace_roots":["/work/lab"],"cursor_version":"1.7","text":"Done."}`,
		`{"hook_event_name":"Stop","conversation_id":"c1","generation_id":"g1","cwd":"/work/lab","cursor_version":"1.7","last_assistant_message":"Done."}`,
	} {
		got, ok := parseTurnEnd([]byte(raw), runner.Detect([]byte(raw)))
		want := turnEnd{event.HostCursor, "c1", "g1", "/work/lab", "Done.", ""}
		if !ok || got != want {
			t.Errorf("got %+v, want %+v", got, want)
		}
	}
}

// The real shapes, one of each: a request reaches the phone only through these.
func TestLastRequestIsReadFromTheTranscript(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.jsonl")
	lines := `{"type":"user","turnOrigin":"human","message":{"role":"user","content":"first thing"},"promptId":"a"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"ok"}]}}
{"type":"user","turnOrigin":"human","message":{"role":"user","content":"what does this do?"},"promptId":"c"}
`
	if err := os.WriteFile(path, []byte(lines), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := lastRequest(path); got != "what does this do?" {
		t.Fatalf("got %q", got)
	}
	if got := lastRequest(""); got != "" {
		t.Fatalf("no transcript gave %q", got)
	}
	if got := lastRequest(path + ".missing"); got != "" {
		t.Fatalf("a missing transcript gave %q", got)
	}
}

func TestCodexTranscriptIsFoundFromItsSession(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	dir := filepath.Join(home, "sessions", "2026", "10", "31")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "rollout-2026-10-31T10-00-00-abc-123.jsonl")
	if err := os.WriteFile(want, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := codexTranscript("abc-123"); got != want {
		t.Fatalf("got %q", got)
	}
	for _, id := range []string{"", "*", "other", "../abc-123"} {
		if got := codexTranscript(id); got != "" {
			t.Errorf("%q found %q", id, got)
		}
	}
}
