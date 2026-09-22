// Package e2e proves the Phase 1 pipeline holds together end to end:
// a raw host payload becomes a normalised event, is recorded, is replayed, and
// is judged against intent read from a transcript.
//
// This is the Phase 1 exit criterion as an executable test.
package e2e_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yerinsabraham/trackline/engine/internal/adapter/claudecode"
	"github.com/yerinsabraham/trackline/engine/internal/adapter/codex"
	"github.com/yerinsabraham/trackline/engine/internal/engine"
	"github.com/yerinsabraham/trackline/engine/internal/intent"
	"github.com/yerinsabraham/trackline/engine/internal/session"
	"github.com/yerinsabraham/trackline/engine/internal/signal"
	"github.com/yerinsabraham/trackline/engine/internal/verdict"
)

// stubScope mirrors the scaffolding check in cmd/inspect: it exists to prove
// events and intent reach a signal and a verdict comes back with evidence.
type stubScope struct{}

func (stubScope) Name() string { return "scope-stub" }

func (stubScope) Check(in signal.Input) verdict.Result {
	anchor, ok := in.Intent.Anchor()
	if !ok {
		return verdict.NotApplicable("scope-stub", "nothing substantive asked yet")
	}
	if in.Event.Action.PathsUnknown {
		return verdict.CannotMeasure("scope-stub", in.Event.Action.ToolName+" does not report which files it touches")
	}
	if !in.Event.TouchesFiles() {
		return verdict.NotApplicable("scope-stub", "touches no files")
	}
	for _, p := range in.Event.Action.Paths {
		if !contains(p, "auth") {
			return verdict.Finding("scope-stub", verdict.Verdict{
				Severity: verdict.SeverityWarn,
				Target:   p,
				Summary:  "edited a file the request never mentioned",
				Evidence: []verdict.Evidence{
					{Kind: verdict.EvidenceTurn, Value: anchor.Text, Note: "what was asked"},
					{Kind: verdict.EvidenceFile, Value: p, Note: "what was touched"},
				},
			})
		}
	}
	return verdict.Clean("scope-stub")
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

const transcript = `{"type":"user","turnOrigin":"human","origin":{"kind":"human"},"promptId":"t1","timestamp":"2026-09-22T10:00:00.000Z","message":{"content":"Fix the login bug in the auth module"}}
{"type":"assistant","message":{"content":[{"type":"text","text":"Looking at it"}]}}
{"type":"user","message":{"content":[{"type":"tool_result","content":"ok"}]}}
{"type":"user","turnOrigin":"human","origin":{"kind":"human"},"promptId":"t2","timestamp":"2026-09-22T10:05:00.000Z","message":{"content":"proceed"}}
`

func TestPipeline(t *testing.T) {
	dir := t.TempDir()
	if out := os.Getenv("TRACKLINE_DEMO_DIR"); out != "" {
		dir = out
		os.MkdirAll(dir, 0o755)
	}
	tPath := filepath.Join(dir, "transcript.jsonl")
	rPath := filepath.Join(dir, "session.jsonl")
	os.Remove(rPath)
	if err := os.WriteFile(tPath, []byte(transcript), 0o600); err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	rec := session.Recorder{Path: rPath}

	// Three real-shaped payloads: an in-scope write, a shell command whose
	// files are invisible, and a Codex patch touching an unrelated module.
	claudeWrite := `{"hook_event_name":"PreToolUse","session_id":"s1","prompt_id":"t1","tool_use_id":"u1","tool_name":"Write","cwd":"/work/app","transcript_path":"` + tPath + `","tool_input":{"file_path":"src/auth/login.ts","content":"fix"}}`
	claudeBash := `{"hook_event_name":"PreToolUse","session_id":"s1","prompt_id":"t1","tool_use_id":"u2","tool_name":"Bash","cwd":"/work/app","transcript_path":"` + tPath + `","tool_input":{"command":"rm -rf dist && npm run build"}}`
	codexPatch := `{"hook_event_name":"PreToolUse","session_id":"s1","turn_id":"t1","tool_use_id":"u3","tool_name":"apply_patch","cwd":"/work/app","transcript_path":"` + tPath + `","tool_input":{"command":"*** Begin Patch\n*** Update File: src/payments/charge.ts\n+surcharge\n*** End Patch\n"}}`

	for _, raw := range []string{claudeWrite, claudeBash} {
		ev, err := claudecode.Parse([]byte(raw), now)
		if err != nil {
			t.Fatal(err)
		}
		if err := rec.Append(ev); err != nil {
			t.Fatal(err)
		}
	}
	ev, err := codex.Parse([]byte(codexPatch), now)
	if err != nil {
		t.Fatal(err)
	}
	if err := rec.Append(ev); err != nil {
		t.Fatal(err)
	}

	// Replay from disk, exactly as the inspector does.
	events, err := session.Replay(rPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("replayed %d events, want 3", len(events))
	}

	var in intent.Intent
	if err := (&intent.Reader{Path: events[0].TranscriptPath}).Read(&in); err != nil {
		t.Fatal(err)
	}
	if len(in.Turns) != 2 {
		t.Fatalf("read %d human turns, want 2 (tool results must not count)", len(in.Turns))
	}
	anchor, ok := in.Anchor()
	if !ok || anchor.ID != "t1" {
		t.Fatalf(`anchor = %+v; "proceed" must not become the intent`, anchor)
	}

	e := engine.New(stubScope{})
	outcomes := map[verdict.Outcome]int{}
	var findings []verdict.Verdict
	for _, ev := range events {
		rep := e.Run(ev, in, nil)
		for _, res := range rep.Results {
			outcomes[res.Outcome]++
		}
		findings = append(findings, rep.Findings()...)
	}

	if outcomes[verdict.OutcomeClean] != 1 {
		t.Errorf("clean = %d, want 1 (the in-scope auth write)", outcomes[verdict.OutcomeClean])
	}
	if outcomes[verdict.OutcomeCannotMeasure] != 1 {
		t.Errorf("cannot-measure = %d, want 1 (the shell command hides its files)", outcomes[verdict.OutcomeCannotMeasure])
	}
	if len(findings) != 1 {
		t.Fatalf("findings = %d, want 1 (the Codex patch into payments)", len(findings))
	}

	f := findings[0]
	if len(f.Evidence) != 2 {
		t.Fatalf("a finding must name what it saw: %+v", f)
	}
	if f.Evidence[0].Value != "Fix the login bug in the auth module" {
		t.Errorf("evidence cites %q, not the instruction", f.Evidence[0].Value)
	}
	if filepath.Base(f.Evidence[1].Value) != "charge.ts" {
		t.Errorf("evidence cites %q", f.Evidence[1].Value)
	}
}
