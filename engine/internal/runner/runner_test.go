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

// The rule the whole project keeps re-learning.
func TestShellWritesAreReportedAsUnmeasured(t *testing.T) {
	root := t.TempDir()
	raw := []byte(`{"hook_event_name":"PreToolUse","session_id":"s","prompt_id":"t",
		"tool_name":"Bash","cwd":"` + root + `","tool_input":{"command":"echo X >> .env"}}`)

	d, err := runner.Run(raw, runner.Options{Root: root, Now: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Report.Unmeasured()) == 0 {
		t.Error("a shell write could hit a protected path and must not read as clean")
	}
}

// Measured gap: an agent asked to add a library rewrote package.json whole, and
// dependency-added could not tell an addition from a change because a
// whole-file write carries no prior state. The hook runs before the tool, so
// what is on disk is the prior state.
func TestWholeFileWriteGetsPriorStateFromDisk(t *testing.T) {
	root := t.TempDir()
	manifest := filepath.Join(root, "package.json")
	os.WriteFile(manifest, []byte(`{"dependencies":{"react":"^18.0.0"}}`), 0o600)

	raw := []byte(`{"hook_event_name":"PreToolUse","session_id":"s","prompt_id":"t",
		"tool_name":"Write","cwd":"` + root + `","tool_input":{"file_path":"package.json",
		"content":"{\"dependencies\":{\"react\":\"^18.0.0\",\"lodash\":\"^4.17.21\"}}"}}`)

	d, err := runner.Run(raw, runner.Options{Root: root, Now: time.Now()})
	if err != nil {
		t.Fatal(err)
	}

	var found bool
	for _, v := range d.Report.Findings() {
		if strings.Contains(v.Summary, "lodash") {
			found = true
		}
	}
	if !found {
		var outcomes []string
		for _, r := range d.Report.Results {
			outcomes = append(outcomes, r.Signal+"="+string(r.Outcome))
		}
		t.Errorf("a package added by a whole-file write was not reported: %v", outcomes)
	}
}

// A file that does not exist yet has no prior state and needs none.
func TestNewFileNeedsNoPriorState(t *testing.T) {
	root := t.TempDir()
	raw := []byte(`{"hook_event_name":"PreToolUse","session_id":"s","prompt_id":"t",
		"tool_name":"Write","cwd":"` + root + `","tool_input":{"file_path":"src/new.ts","content":"x"}}`)

	if _, err := runner.Run(raw, runner.Options{Root: root, Now: time.Now()}); err != nil {
		t.Errorf("writing a new file must not fail: %v", err)
	}
}
