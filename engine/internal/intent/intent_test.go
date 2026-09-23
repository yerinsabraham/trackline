package intent_test

import (
	"os"
	"path/filepath"
	"strings"
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

// A headless session marks the human turn turnOrigin:"sdk" and carries no
// origin object. Missing it meant every scripted session read as having no
// request in it, and the checks that depend on intent reported not-applicable
// when they were actually blind.
func TestHeadlessSessionsAreHumanTurns(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "t.jsonl")
	os.WriteFile(path, []byte(
		`{"type":"user","turnOrigin":"sdk","userType":"external","promptId":"t1","message":{"content":"Fix the login bug in src/auth"}}`+"\n"+
			`{"type":"assistant","message":{"content":[{"type":"text","text":"ok"}]}}`+"\n"+
			`{"type":"user","message":{"content":[{"type":"tool_result","content":"done"}]}}`+"\n"), 0o600)

	r := &intent.Reader{Path: path}
	var in intent.Intent
	if err := r.Read(&in); err != nil {
		t.Fatal(err)
	}
	if len(in.Turns) != 1 {
		t.Fatalf("got %d turns, want the sdk prompt: %+v", len(in.Turns), in.Turns)
	}
	if in.Turns[0].Text != "Fix the login bug in src/auth" {
		t.Errorf("text = %q", in.Turns[0].Text)
	}
}

// Separating "nothing asked yet" from "we did not recognise this format".
func TestReaderReportsWhetherItSawAConversation(t *testing.T) {
	dir := t.TempDir()

	empty := filepath.Join(dir, "empty.jsonl")
	os.WriteFile(empty, []byte(""), 0o600)
	r := &intent.Reader{Path: empty}
	var in intent.Intent
	r.Read(&in)
	if entries, _ := r.ReadAnything(); entries != 0 {
		t.Errorf("entries = %d, want 0 for an empty transcript", entries)
	}

	unknown := filepath.Join(dir, "unknown.jsonl")
	os.WriteFile(unknown, []byte(
		`{"type":"user","turnOrigin":"something-new","message":{"content":"a request"}}`+"\n"+
			`{"type":"assistant","message":{"content":[{"type":"text","text":"a reply"}]}}`+"\n"), 0o600)
	r2 := &intent.Reader{Path: unknown}
	var in2 intent.Intent
	r2.Read(&in2)
	entries, assistant := r2.ReadAnything()
	if len(in2.Turns) != 0 {
		t.Fatal("this test needs an unrecognised origin")
	}
	if entries == 0 || assistant == 0 {
		t.Errorf("entries=%d assistant=%d; a conversation was read but no request recognised, and the caller must be able to tell", entries, assistant)
	}
}

// Cursor's transcript: {role, message} lines, the request wrapped in
// <user_query>. Read from a scrubbed capture, not an invented line.
func TestCursorTranscriptYieldsTheRequest(t *testing.T) {
	r := &intent.Reader{Path: filepath.Join("testdata", "cursor-transcript.jsonl")}
	var in intent.Intent
	if err := r.Read(&in); err != nil {
		t.Fatal(err)
	}
	if len(in.Turns) != 1 {
		t.Fatalf("got %d turns, want the one request: %+v", len(in.Turns), in.Turns)
	}
	got := in.Turns[0].Text
	if !strings.HasPrefix(got, "Read greet.js.") {
		t.Errorf("text = %q; want the request without Cursor's wrapper", got)
	}
	if strings.Contains(got, "<timestamp>") || strings.Contains(got, "user_query") {
		t.Errorf("wrapper leaked into the request: %q", got)
	}
	if _, assistant := r.ReadAnything(); assistant != 1 {
		t.Errorf("assistant turns = %d, want 1", assistant)
	}
}

// A user line Cursor wrote without a <user_query> is not a request anyone
// made, and must not become the anchor that scope is judged against.
func TestCursorLinesWithoutAQueryAreNotRequests(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "t.jsonl")
	os.WriteFile(path, []byte(
		`{"role":"user","message":{"content":[{"type":"text","text":"<system_reminder>x</system_reminder>"}]}}`+"\n"), 0o600)
	r := &intent.Reader{Path: path}
	var in intent.Intent
	r.Read(&in)
	if len(in.Turns) != 0 {
		t.Errorf("got %+v; an injected line is not a request", in.Turns)
	}
}
