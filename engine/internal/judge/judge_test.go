package judge_test

import (
	"context"
	"strings"
	"testing"

	"github.com/yerinsabraham/trackline/engine/internal/judge"
)

type fake struct {
	reply string
	err   error
	saw   string
}

func (f *fake) Name() string { return "fake" }
func (f *fake) Ask(_ context.Context, system, user string) (string, error) {
	f.saw = system + "\n" + user
	return f.reply, f.err
}

func turn() judge.Turn {
	return judge.Turn{
		Request: "Fix the login bug in src/auth",
		Actions: []string{"Write src/auth/login.ts", "Write test/auth/login.test.ts"},
	}
}

func TestReadsAVerdict(t *testing.T) {
	f := &fake{reply: `{"verdict":"serves","reason":"both files are in the auth area","unrelated":""}`}
	a, err := judge.New(f).Ask(context.Background(), turn())
	if err != nil {
		t.Fatal(err)
	}
	if a.Verdict != judge.Serves {
		t.Errorf("verdict = %q", a.Verdict)
	}
	if a.Reason == "" {
		t.Error("a verdict must come with a reason")
	}
}

// Models wrap JSON in fences often enough that refusing it would mean failing
// on a correct answer.
func TestReadsFencedAndPrefixedReplies(t *testing.T) {
	for _, reply := range []string{
		"```json\n{\"verdict\":\"unrelated\",\"reason\":\"r\",\"unrelated\":\"x\"}\n```",
		"```\n{\"verdict\":\"unrelated\",\"reason\":\"r\",\"unrelated\":\"x\"}\n```",
		"Here is my answer:\n{\"verdict\":\"unrelated\",\"reason\":\"r\",\"unrelated\":\"x\"}",
	} {
		a, err := judge.New(&fake{reply: reply}).Ask(context.Background(), turn())
		if err != nil {
			t.Errorf("reply %.30q: %v", reply, err)
			continue
		}
		if a.Verdict != judge.Unrelated {
			t.Errorf("reply %.30q: verdict = %q", reply, a.Verdict)
		}
	}
}

// Silently turning a broken reply into an abstention would hide a
// misconfigured provider behind something that looks like a considered answer.
func TestUnreadableReplyIsAnErrorNotAnAbstention(t *testing.T) {
	for _, reply := range []string{"not json at all", `{"verdict":"maybe"}`, ""} {
		if _, err := judge.New(&fake{reply: reply}).Ask(context.Background(), turn()); err == nil {
			t.Errorf("reply %q was accepted", reply)
		}
	}
}

// Rule one: a judge shown an answer agrees with it.
func TestTheJudgeIsNotToldWhatTheOtherChecksFound(t *testing.T) {
	f := &fake{reply: `{"verdict":"serves","reason":"r","unrelated":""}`}
	judge.New(f).Ask(context.Background(), turn())

	// The names of the other checks, and any word implying one already
	// objected. "verdict" is excluded: it is the field the judge fills in, not
	// a report of someone else's conclusion.
	for _, leak := range []string{
		"off-limits", "diff-size", "repetition", "dependency-added",
		"flagged", "violation", "was blocked", "already found", "another check",
	} {
		if strings.Contains(strings.ToLower(f.saw), leak) {
			t.Errorf("the prompt mentioned %q; the judge must be asked cold", leak)
		}
	}
}

// Rule two: asking a model whether code is good returns its taste.
func TestTheJudgeIsAskedAboutFitNotQuality(t *testing.T) {
	f := &fake{reply: `{"verdict":"serves","reason":"r","unrelated":""}`}
	judge.New(f).Ask(context.Background(), turn())
	low := strings.ToLower(f.saw)

	if !strings.Contains(low, "not judging whether the work is good") {
		t.Error("the prompt should rule out judging quality")
	}
	if !strings.Contains(low, "serve") {
		t.Error("the prompt should ask about serving the request")
	}
}

// Rule three: a judge forced to choose invents a reason.
func TestAbstentionIsOffered(t *testing.T) {
	f := &fake{reply: `{"verdict":"unclear","reason":"the request did not say what to change","unrelated":""}`}
	a, err := judge.New(f).Ask(context.Background(), turn())
	if err != nil {
		t.Fatal(err)
	}
	if a.Verdict != judge.Unclear {
		t.Errorf("verdict = %q", a.Verdict)
	}
	if !strings.Contains(strings.ToLower(f.saw), "unclear") {
		t.Error("the prompt must offer abstention, or the judge will guess")
	}
}

// Judging nothing produces an opinion about nothing.
func TestEmptyTurnsAreNotJudged(t *testing.T) {
	f := &fake{reply: `{"verdict":"serves","reason":"r","unrelated":""}`}
	j := judge.New(f)

	a, _ := j.Ask(context.Background(), judge.Turn{Request: "do a thing"})
	if a.Verdict != judge.Unclear {
		t.Errorf("a turn with no actions: verdict = %q, want unclear", a.Verdict)
	}
	a, _ = j.Ask(context.Background(), judge.Turn{Actions: []string{"Write a.ts"}})
	if a.Verdict != judge.Unclear {
		t.Errorf("a turn with no request: verdict = %q, want unclear", a.Verdict)
	}
}

func TestNoProviderIsAnError(t *testing.T) {
	if _, err := judge.New(nil).Ask(context.Background(), turn()); err == nil {
		t.Error("asking with no provider configured must error")
	}
}
