package intent_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yerinsabraham/trackline/engine/internal/intent"
)

// The finding this package exists for: the most recent human turn in a real
// measured session was the single word "proceed", which is not a task.
func TestProceedIsNotATask(t *testing.T) {
	var in intent.Intent
	in.Add("t1", time.Now(), "Fix the login bug in the auth module")
	in.Add("t2", time.Now(), "proceed")

	latest, _ := in.Latest()
	if latest.Text != "proceed" {
		t.Fatalf("latest = %q", latest.Text)
	}
	if latest.Substantive {
		t.Error(`"proceed" must not be treated as an instruction`)
	}

	anchor, ok := in.Anchor()
	if !ok {
		t.Fatal("expected an anchor")
	}
	if anchor.Text != "Fix the login bug in the auth module" {
		t.Errorf("anchor = %q; it must stay on the real instruction", anchor.Text)
	}
}

func TestContinuationClassification(t *testing.T) {
	notInstructions := []string{
		"proceed", "Proceed.", "  PROCEED  ", "yes", "ok", "Okay!",
		"continue", "go ahead", "keep going", "do it", "lgtm", "sounds good", "",
	}
	for _, s := range notInstructions {
		if intent.IsSubstantive(s) {
			t.Errorf("IsSubstantive(%q) = true, want false", s)
		}
	}

	instructions := []string{
		"proceed with the refactor but skip the tests",
		"yes, and also update the README",
		"continue from where you left off in the parser",
		"ok now delete the old migration",
		"go", // sentinel: bare "go" is a continuation
	}
	for _, s := range instructions[:4] {
		if !intent.IsSubstantive(s) {
			t.Errorf("IsSubstantive(%q) = false; a continuation word with content after it is an instruction", s)
		}
	}
}

// A long run of "proceed" must not erase what was asked for.
func TestAnchorSurvivesManyContentlessTurns(t *testing.T) {
	var in intent.Intent
	in.Add("t1", time.Now(), "Refactor the payments module, do not touch auth")
	for i := 0; i < 20; i++ {
		in.Add("x", time.Now(), "proceed")
	}
	anchor, ok := in.Anchor()
	if !ok || anchor.ID != "t1" {
		t.Fatalf("anchor = %+v; twenty continuations must not lose the instruction", anchor)
	}
}

func TestNoAnchorBeforeAnythingIsAsked(t *testing.T) {
	var in intent.Intent
	in.Add("t1", time.Now(), "ok")
	if _, ok := in.Anchor(); ok {
		t.Error("a session with no instruction yet must report no anchor, not an empty one")
	}
}

// Intent spans several turns: a task, a correction, a constraint. A check that
// reads only the anchor misses the correction.
func TestRecentReturnsInstructionsOldestFirst(t *testing.T) {
	var in intent.Intent
	in.Add("t1", time.Now(), "Build the export feature")
	in.Add("t2", time.Now(), "proceed")
	in.Add("t3", time.Now(), "Actually put it behind a feature flag")
	in.Add("t4", time.Now(), "yes")

	got := in.Recent(5)
	if len(got) != 2 {
		t.Fatalf("got %d turns, want the 2 substantive ones", len(got))
	}
	if got[0].ID != "t1" || got[1].ID != "t3" {
		t.Errorf("order = %s,%s; want oldest first", got[0].ID, got[1].ID)
	}
}

// Reading a 3.2 MB transcript on every tool call is not available at a 10ms
// budget, so the reader must only consume what was appended.
func TestReaderIsIncremental(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "transcript.jsonl")

	line := func(id, text string) string {
		return `{"type":"user","turnOrigin":"human","origin":{"kind":"human"},` +
			`"promptId":"` + id + `","timestamp":"2026-09-22T10:00:00.000Z",` +
			`"message":{"content":"` + text + `"}}` + "\n"
	}
	if err := os.WriteFile(path, []byte(line("t1", "first task")), 0o600); err != nil {
		t.Fatal(err)
	}

	r := &intent.Reader{Path: path}
	var in intent.Intent
	if err := r.Read(&in); err != nil {
		t.Fatal(err)
	}
	if len(in.Turns) != 1 {
		t.Fatalf("got %d turns, want 1", len(in.Turns))
	}

	// Nothing appended: a second read must add nothing rather than duplicate.
	if err := r.Read(&in); err != nil {
		t.Fatal(err)
	}
	if len(in.Turns) != 1 {
		t.Fatalf("got %d turns after a no-op read; the reader re-read old lines", len(in.Turns))
	}

	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	f.WriteString(line("t2", "second task"))
	f.Close()

	if err := r.Read(&in); err != nil {
		t.Fatal(err)
	}
	if len(in.Turns) != 2 || in.Turns[1].ID != "t2" {
		t.Fatalf("turns = %+v; only the appended line should have been read", in.Turns)
	}
}

// Of 259 `user` entries in a measured session, 229 were tool results.
func TestToolResultsAreNotHumanTurns(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "t.jsonl")
	content := `{"type":"user","turnOrigin":"human","origin":{"kind":"human"},"promptId":"t1","message":{"content":"real instruction"}}
{"type":"user","message":{"content":[{"type":"tool_result","content":"some output"}]}}
{"type":"assistant","message":{"content":[{"type":"text","text":"I will do that"}]}}
{"type":"attachment","message":{"content":"a file"}}
`
	os.WriteFile(path, []byte(content), 0o600)

	r := &intent.Reader{Path: path}
	var in intent.Intent
	if err := r.Read(&in); err != nil {
		t.Fatal(err)
	}
	if len(in.Turns) != 1 {
		t.Fatalf("got %d turns, want only the human one: %+v", len(in.Turns), in.Turns)
	}
	if in.Turns[0].Text != "real instruction" {
		t.Errorf("text = %q", in.Turns[0].Text)
	}
}

func TestRotatedTranscriptIsReadFromTheStart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "t.jsonl")
	line := `{"type":"user","turnOrigin":"human","promptId":"a","message":{"content":"one"}}` + "\n"
	os.WriteFile(path, []byte(line+line+line), 0o600)

	r := &intent.Reader{Path: path}
	var in intent.Intent
	r.Read(&in)

	// Rewritten shorter: the stored offset now points into the middle of an
	// unrelated line.
	os.WriteFile(path, []byte(line), 0o600)
	var after intent.Intent
	if err := r.Read(&after); err != nil {
		t.Fatal(err)
	}
	if len(after.Turns) != 1 {
		t.Errorf("got %d turns; a shrunken transcript must be re-read from the start", len(after.Turns))
	}
}
