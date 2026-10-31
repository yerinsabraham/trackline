package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yerinsabraham/trackline/engine/internal/config"
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

func TestTheAgentsOwnTitleIsFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "t.jsonl")
	lines := `{"type":"user","message":{"content":"title this"}}
{"type":"ai-title","aiTitle":"First name","sessionId":"s"}
{"type":"ai-title","aiTitle":"Better name","sessionId":"s"}
`
	if err := os.WriteFile(path, []byte(lines), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := agentTitle(turnEnd{host: event.HostClaudeCode, transcript: path}); got != "Better name" {
		t.Fatalf("claude: %q", got)
	}
	t.Setenv("CODEX_HOME", dir)
	index := `{"id":"a","thread_name":"Not this one"}
{"id":"b","thread_name":"Old name"}
{"id":"b","thread_name":"Guardian review"}
`
	if err := os.WriteFile(filepath.Join(dir, "session_index.jsonl"), []byte(index), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := agentTitle(turnEnd{host: event.HostCodex, session: "b"}); got != "Guardian review" {
		t.Fatalf("codex: %q", got)
	}
	if got := agentTitle(turnEnd{host: event.HostCursor, session: "b"}); got != "" {
		t.Fatalf("cursor: %q", got)
	}
}

// Measured on real transcripts: 559 MB took 30 seconds to read whole, against a
// 15 second hook limit, and the reply went with it.
func TestALongTranscriptCostsNoMoreThanAShortOne(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(256 << 20); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Seek(0, 2); err != nil {
		t.Fatal(err)
	}
	f.WriteString("\n" + `{"type":"ai-title","aiTitle":"Named late"}` + "\n")
	f.WriteString(`{"type":"user","turnOrigin":"human","message":{"role":"user","content":"the last thing"},"promptId":"z"}` + "\n")
	f.Close()

	start := time.Now()
	req, title := lastRequest(path), agentTitle(turnEnd{host: event.HostClaudeCode, transcript: path})
	if took := time.Since(start); took > time.Second {
		t.Fatalf("took %s", took)
	}
	if req != "the last thing" || title != "Named late" {
		t.Fatalf("req %q, title %q", req, title)
	}
}

func TestTheModelIsTheLatestOneTheTranscriptNames(t *testing.T) {
	tail := []byte(`{"message":{"model":"claude-opus-5-5"}}
{"payload":{"model":"gpt-6-astra"}}
{"message":{"model":"<synthetic>"}}
{"text":"the user wrote \"model\":\"made-up\" here"}
`)
	if got := lastModel(tail); got != "gpt-6-astra" {
		t.Fatalf("model = %q", got)
	}
	if lastModel(nil) != "" {
		t.Fatal("no transcript must mean no model")
	}
}

func TestAClaudeChatsNameIsFoundInTheList(t *testing.T) {
	list := []byte(`[{"sessionId":"a","name":"app-3b"},{"sessionId":"b","name":"app-c8"}]`)
	if got := claudeChatName(list, "b"); got != "app-c8" {
		t.Fatalf("name = %q", got)
	}
	if claudeChatName(list, "headless") != "" || claudeChatName([]byte("no"), "b") != "" {
		t.Fatal("an unlisted session has no name")
	}
}

func TestAKnownNameIsNotAskedForAgain(t *testing.T) {
	dir := t.TempDir()
	bin := t.TempDir()
	calls := filepath.Join(bin, "calls")
	script := "#!/bin/sh\necho x >> " + calls + "\nprintf '[{\"sessionId\":\"s1\",\"name\":\"app-3b\"}]'\n"
	os.WriteFile(filepath.Join(bin, "claude"), []byte(script), 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	end := turnEnd{host: event.HostClaudeCode, session: "s1"}
	for range 2 {
		if got := agentName(end, dir); got != "app-3b" {
			t.Fatalf("name = %q", got)
		}
	}
	if b, _ := os.ReadFile(calls); len(b) != 2 {
		t.Fatalf("asked %d times", len(b)/2)
	}
}

func TestAPhoneRunDoesNotAskForAName(t *testing.T) {
	t.Setenv(config.RemoteEnv, "job_1")
	if got := agentName(turnEnd{host: event.HostClaudeCode, session: "s1"}, t.TempDir()); got != "" {
		t.Fatalf("name = %q", got)
	}
}
