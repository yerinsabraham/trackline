package runner_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yerinsabraham/trackline/engine/internal/config"
	"github.com/yerinsabraham/trackline/engine/internal/runner"
	"github.com/yerinsabraham/trackline/engine/internal/verdict"
)

func TestDetectsHostFromPayloadShape(t *testing.T) {
	cases := []struct{ raw, want string }{
		{`{"prompt_id":"p","tool_name":"Write"}`, runner.HostClaude},
		{`{"turn_id":"t","tool_name":"apply_patch"}`, runner.HostCodex},
		{`{"model":"gpt-5","tool_name":"shell"}`, runner.HostCodex},
		{`{"tool_name":"Write"}`, runner.HostClaude},
		{`{"cursor_version":"3.21.18","generation_id":"g","tool_name":"Write"}`, runner.HostCursor},
		// Cursor also runs Claude-format hooks. Its payload still says Cursor.
		{`{"cursor_version":"3.21.18","prompt_id":"p","tool_name":"Write"}`, runner.HostCursor},
		{"\xEF\xBB\xBF" + `{"cursor_version":"3.21.18","tool_name":"Write"}`, runner.HostCursor},
		{`not json`, runner.HostClaude},
	}
	for _, c := range cases {
		if got := runner.Detect([]byte(c.raw)); got != c.want {
			t.Errorf("Detect(%s) = %q, want %q", c.raw, got, c.want)
		}
	}
}

func payload(root, file string) []byte {
	return []byte(`{"hook_event_name":"PreToolUse","session_id":"s","prompt_id":"t",
		"tool_name":"Write","cwd":"` + root + `","tool_input":{"file_path":"` + file + `"}}`)
}

// Phase 2 ships warn-only. Grounds to block are not permission to block.
func TestWarnModeNeverBlocks(t *testing.T) {
	root := t.TempDir()
	d, err := runner.Run(payload(root, ".env"), runner.Options{Root: root, Now: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if !d.Report.Blocked() {
		t.Fatal("writing to .env should establish grounds to block")
	}
	if d.Block {
		t.Error("warn mode must never actually block, however serious the finding")
	}
	if d.Mode != config.ModeWarn {
		t.Errorf("mode = %q, want warn by default", d.Mode)
	}
}

// A session started from the phone blocks on a secret even where the project
// only warns: there is nobody at the keyboard to read a warning.
func TestRemoteSessionBlocksWhereTheProjectOnlyWarns(t *testing.T) {
	root := t.TempDir()
	t.Setenv(config.RemoteEnv, "job_1")
	d, err := runner.Run(payload(root, ".env"), runner.Options{Root: root, Now: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if !d.Block {
		t.Fatal("a remote session wrote to .env without being stopped")
	}
}

func TestAutoModeBlocksWithAnActionableMessage(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, ".trackline.json"),
		[]byte(`{"modes":{"off-limits":"auto"}}`), 0o600)

	d, err := runner.Run(payload(root, ".env"), runner.Options{Root: root, Now: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if !d.Block {
		t.Fatal("auto mode must block a protected write")
	}
	// Phase 0 measured 11 corrections out of 11 when the message named a
	// concrete alternative. A bare refusal is untested, so the suggestion is
	// treated as load-bearing.
	if !strings.Contains(d.Message, "do this instead") {
		t.Errorf("block message must say what to do instead:\n%s", d.Message)
	}
	if !strings.Contains(d.Message, ".env") {
		t.Errorf("block message must name the file:\n%s", d.Message)
	}
}

func TestOrdinaryFilesArePassedThroughInEveryMode(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, ".trackline.json"),
		[]byte(`{"mode":"auto"}`), 0o600)

	d, err := runner.Run(payload(root, "src/app.ts"), runner.Options{Root: root, Now: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if d.Block {
		t.Error("ordinary work must never be blocked, even in auto mode")
	}
	if len(d.Report.Findings()) != 0 {
		t.Errorf("no findings expected, got %+v", d.Report.Findings())
	}
}

// A broken config must be said out loud and must not stop the user working.
func TestBrokenConfigIsReportedButNeverBlocks(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, ".trackline.json"), []byte(`{"mode":`), 0o600)

	d, err := runner.Run(payload(root, "src/app.ts"), runner.Options{Root: root, Now: time.Now()})
	if err != nil {
		t.Fatalf("a broken config must not fail the pass: %v", err)
	}
	if d.Block {
		t.Error("a broken config must never block the user's work")
	}
	var saw bool
	for _, r := range d.Report.Results {
		if r.Signal == "config" && r.Outcome == verdict.OutcomeCannotMeasure {
			saw = true
		}
	}
	if !saw {
		t.Error("a broken config should be surfaced as cannot-measure, not swallowed")
	}
}

func TestMalformedPayloadErrorsRatherThanGuessing(t *testing.T) {
	if _, err := runner.Run([]byte(`{not json`), runner.Options{Now: time.Now()}); err == nil {
		t.Error("a malformed payload must error")
	}
}

// A shell redirect into a protected path used to be invisible. It is not any
// more, and this is the case that made the shell parser worth building.
func TestShellRedirectIntoAProtectedPathIsCaught(t *testing.T) {
	root := t.TempDir()
	raw := []byte(`{"hook_event_name":"PreToolUse","session_id":"s","prompt_id":"t",
		"tool_name":"Bash","cwd":"` + root + `","tool_input":{"command":"echo TOKEN=x >> .env"}}`)

	d, err := runner.Run(raw, runner.Options{Root: root, Now: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, v := range d.Report.Findings() {
		if strings.Contains(v.Summary, ".env") {
			found = true
		}
	}
	if !found {
		t.Errorf("a redirect into .env was not caught: %+v", d.Report.Results)
	}
}

// A command nobody can read must still be reported as unchecked.
func TestUnreadableCommandsAreStillUnmeasured(t *testing.T) {
	root := t.TempDir()
	raw := []byte(`{"hook_event_name":"PreToolUse","session_id":"s","prompt_id":"t",
		"tool_name":"Bash","cwd":"` + root + `","tool_input":{"command":"./scripts/deploy.sh"}}`)

	d, err := runner.Run(raw, runner.Options{Root: root, Now: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Report.Unmeasured()) == 0 {
		t.Error("an unrecognised command could do anything and must not read as clean")
	}
}

// The same protected write, arriving from Cursor, reaches the same decision
// through the unchanged core.
func TestCursorPayloadIsJudgedLikeAnyOther(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, ".trackline.json"),
		[]byte(`{"modes":{"off-limits":"auto"}}`), 0o600)

	raw := []byte(`{"hook_event_name":"preToolUse","conversation_id":"c","generation_id":"g",
		"tool_use_id":"u","tool_name":"Write","cursor_version":"3.21.18",
		"workspace_roots":["` + root + `"],"transcript_path":null,
		"tool_input":{"file_path":"` + filepath.Join(root, ".env") + `","content":"SECRET=1"}}`)

	d, err := runner.Run(raw, runner.Options{Now: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if !d.Block {
		t.Fatal("a protected write from Cursor must block in auto mode, as from any host")
	}
	if !strings.Contains(d.Message, ".env") {
		t.Errorf("block message must name the file:\n%s", d.Message)
	}
}

// Cursor's transcript has no turn ids, so review could not pair an action with
// its request. The request is now recorded with the action as the hook read it.
func TestRecordedActionCarriesItsRequest(t *testing.T) {
	root := t.TempDir()
	transcript := filepath.Join(root, "t.jsonl")
	os.WriteFile(transcript, []byte(
		`{"role":"user","message":{"content":[{"type":"text","text":"<user_query>\nRename tmp to total\n</user_query>"}]}}`+"\n"), 0o600)
	raw := []byte(`{"hook_event_name":"preToolUse","conversation_id":"c","generation_id":"g",
		"tool_name":"Write","cursor_version":"3.21.18","workspace_roots":["` + root + `"],
		"transcript_path":"` + transcript + `",
		"tool_input":{"file_path":"` + filepath.Join(root, "src", "sum.js") + `","content":"x"}}`)
	if _, err := runner.Run(raw, runner.Options{Now: time.Now()}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(root, ".trackline", "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"request":"Rename tmp to total"`) {
		t.Errorf("the recorded event must carry the request it was made under:\n%s", b)
	}
}

// Surfaced as cannot-measure on every action, never as a block: a broken
// rules file is worth saying out loud and never worth stopping work over.
func TestUnreadableRulesAreCannotMeasure(t *testing.T) {
	root := t.TempDir()
	os.Mkdir(filepath.Join(root, "AGENTS.md"), 0o755)
	d, err := runner.Run(payload(root, "src/app.ts"), runner.Options{Root: root, Now: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, r := range d.Report.Results {
		if r.Signal == "rules" && r.Outcome == verdict.OutcomeCannotMeasure && strings.Contains(r.Reason, "AGENTS.md") {
			found = true
		}
	}
	if !found {
		t.Errorf("results = %+v; an unreadable rules file must be reported", d.Report.Results)
	}
	if d.Block {
		t.Error("an unreadable rules file must never block")
	}
}
