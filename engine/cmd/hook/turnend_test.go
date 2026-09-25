package main

import (
	"encoding/json"
	"os"
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
		"claude-stop": {event.HostClaudeCode, "8ae3ba4e-dbd9-4f00-8bbc-8a035fd7ba98", "9420d9d8-3508-4c2b-8a0a-4b0dddaa55ca", "/work/lab", "The capital of France is Paris."},
		"codex-stop":  {event.HostCodex, "01a0d85c-41ac-77d0-890e-4ee49a2963d1", "01a0d85c-42d3-76f0-986c-709d511b56b2", "/work/lab", "The capital of France is Paris."},
	} {
		raw := caps[name]
		got, ok := parseTurnEnd(raw, runner.Detect(raw))
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
		want := turnEnd{event.HostCursor, "c1", "g1", "/work/lab", "Done."}
		if !ok || got != want {
			t.Errorf("got %+v, want %+v", got, want)
		}
	}
}
