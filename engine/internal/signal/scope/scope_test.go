package scope_test

import (
	"strings"
	"testing"
	"time"

	"github.com/yerinsabraham/trackline/engine/internal/event"
	"github.com/yerinsabraham/trackline/engine/internal/intent"
	"github.com/yerinsabraham/trackline/engine/internal/signal"
	"github.com/yerinsabraham/trackline/engine/internal/signal/scope"
	"github.com/yerinsabraham/trackline/engine/internal/verdict"
)

const root = "/work/app"

func ask(turns ...string) intent.Intent {
	var in intent.Intent
	for i, s := range turns {
		in.Add("t"+string(rune('1'+i)), time.Now(), s)
	}
	return in
}

func wrote(in intent.Intent, paths ...string) signal.Input {
	full := make([]string, len(paths))
	for i, p := range paths {
		full[i] = root + "/" + p
	}
	return signal.Input{
		Intent: in,
		Event: event.Event{CWD: root, Action: event.Action{
			Type: event.ActionWriteFile, ToolName: "Write", Paths: full,
		}},
	}
}

func run(t *testing.T, in signal.Input) verdict.Result {
	t.Helper()
	res := scope.New(root).Check(in)
	if err := res.Validate(); err != nil {
		t.Fatalf("invalid result: %v", err)
	}
	return res
}

// The majority of this file. Every case below is ordinary, correct work, and
// firing on any of it would be a false alarm.
func TestStaysSilentOnOrdinaryWork(t *testing.T) {
	cases := []struct {
		name    string
		intent  intent.Intent
		path    string
		because string
	}{
		{"vague request names nowhere", ask("Clean up the codebase a bit"),
			"src/anything.ts", "a request that names nowhere has no scope to violate"},
		{"no request yet", ask(), "src/app.ts", "nothing has been asked"},
		{"writes inside the named directory", ask("Fix the login bug in src/auth"),
			"src/auth/login.ts", "exactly what was asked for"},
		{"writes deeper inside it", ask("Fix the login bug in src/auth"),
			"src/auth/providers/google.ts", "still inside the named area"},
		{"test for the named area", ask("Fix the login bug in src/auth"),
			"test/auth/login.test.ts", "writing a test for the thing you were asked to fix is not drift"},
		{"file at the project root", ask("Fix the login bug in src/auth"),
			"README.md", "a root file belongs to no area"},
		{"names a module not a path", ask("Fix the payments module"),
			"src/payments/charge.ts", "the module was named"},
		{"quoted directory", ask("Update the `config` directory"),
			"config/prod.yml", "the directory was named"},
		{"a later turn widens the scope", ask("Fix login in src/auth", "proceed", "also update config/auth.yml"),
			"config/auth.yml", "intent accumulates; a correction two turns back still counts"},
		{"bare filename mentioned", ask("Update package.json to add the build script"),
			"package.json", "the file itself was named"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := run(t, wrote(c.intent, c.path))
			if res.Outcome == verdict.OutcomeFinding {
				t.Errorf("fired on ordinary work (%s): %s", c.because, res.Verdicts[0].Summary)
			}
		})
	}
}

// The case the check exists for, and it has to be unambiguous to fire.
func TestFiresOnAClearDeparture(t *testing.T) {
	res := run(t, wrote(ask("Fix the login bug in src/auth"), "src/payments/charge.ts"))
	if res.Outcome != verdict.OutcomeFinding {
		t.Fatalf("outcome = %s (%s); payments is plainly not auth", res.Outcome, res.Reason)
	}

	v := res.Verdicts[0]
	if v.Severity != verdict.SeverityWarn {
		t.Error("a heuristic must not block until it has a measured false-alarm rate")
	}
	if len(v.Evidence) != 3 {
		t.Fatalf("evidence = %+v; a finding must name the ask, the file and the stated area", v.Evidence)
	}
	if !strings.Contains(v.Evidence[0].Value, "login") {
		t.Errorf("evidence should quote the request, got %q", v.Evidence[0].Value)
	}
}

// "Proceed" must not erase the scope, which is the whole reason Anchor exists.
func TestContinuationDoesNotLoseTheScope(t *testing.T) {
	in := ask("Fix the login bug in src/auth", "proceed", "yes", "continue")
	if res := run(t, wrote(in, "src/payments/charge.ts")); res.Outcome != verdict.OutcomeFinding {
		t.Errorf("outcome = %s; three continuations must not widen the scope to everything", res.Outcome)
	}
	if res := run(t, wrote(in, "src/auth/login.ts")); res.Outcome != verdict.OutcomeClean {
		t.Errorf("outcome = %s; the named area is still in scope", res.Outcome)
	}
}

func TestUnseeableWritesAreUnchecked(t *testing.T) {
	in := signal.Input{Intent: ask("Fix the login bug in src/auth"),
		Event: event.Event{CWD: root, Action: event.Action{
			Type: event.ActionWriteFile, ToolName: "Bash",
			Command: "sed -i s/a/b/ src/payments/charge.ts", PathsUnknown: true,
		}}}
	if res := run(t, in); res.Outcome != verdict.OutcomeCannotMeasure {
		t.Errorf("outcome = %s; a write we cannot see must not read as clean", res.Outcome)
	}
}

func TestMentionExtraction(t *testing.T) {
	cases := []struct {
		text string
		want []string // meaningful segments, containers stripped
	}{
		{"Fix the login bug in src/auth", []string{"auth"}},
		{"Update the payments module", []string{"payments"}},
		{"Change `config/prod.yml`", []string{"config", "prod"}},
		{"Edit package.json", []string{"package"}},
		{"Refactor everything", nil},
		{"Make the code better", nil},
		{"See https://example.com/docs/thing for context", nil},
		{"Look at the main module", nil},
		{"Work in src/", nil}, // a container alone names no area
	}
	for _, c := range cases {
		var got []string
		for _, m := range scope.Mentions(c.text) {
			got = append(got, m.Parts...)
		}
		if strings.Join(dedupe(got), ",") != strings.Join(c.want, ",") {
			t.Errorf("Mentions(%q) = %v, want %v", c.text, dedupe(got), c.want)
		}
	}
}

// Container directories must not count as scope evidence. Nearly every file is
// under src, so "src" matching "src" proves nothing, and treating it as a match
// makes the check silent on everything.
func TestContainersAreNotScope(t *testing.T) {
	for _, p := range []string{"src", "lib", "test", "internal", "app"} {
		if segs := scope.Segments(p + "/thing.ts"); len(segs) != 1 || segs[0] != "thing" {
			t.Errorf("Segments(%s/thing.ts) = %v, want [thing]", p, segs)
		}
	}
	// A test file names the thing it tests.
	if segs := scope.Segments("test/auth/login.test.ts"); strings.Join(segs, ",") != "auth,login" {
		t.Errorf("Segments = %v, want [auth login]", segs)
	}
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// A live session was blocked for writing to the very file its request had
// named, because "the charge function." ends a sentence and the pattern
// excluded dots outright to stop "Edit package.json" matching. Both have to
// work.
func TestQualifiersSurvivePunctuation(t *testing.T) {
	named := []string{
		"add a 10% discount to the charge function.",
		"add a discount to the charge function",
		"update the charge function, then stop",
		"look at the charge function; it is wrong",
		"fix the charge handler.",
	}
	for _, text := range named {
		var found bool
		for _, m := range scope.Mentions(text) {
			for _, p := range m.Parts {
				if p == "charge" {
					found = true
				}
			}
		}
		if !found {
			t.Errorf("Mentions(%q) did not find "+"charge"+"; the request named it", text)
		}
	}

	// And the case the exclusion existed for.
	for _, text := range []string{"Edit package.json", "update package.json now"} {
		for _, m := range scope.Mentions(text) {
			for _, p := range m.Parts {
				if p == "edit" || p == "update" {
					t.Errorf("Mentions(%q) captured a verb as a place: %v", text, m.Parts)
				}
			}
		}
	}
}

// The live case end to end: a request naming two areas must clear writes to
// both.
func TestARequestNamingTwoAreasClearsBoth(t *testing.T) {
	in := ask("Fix the login function in src/auth to also accept 'root'. Then add a 10% discount to the charge function.")
	for _, p := range []string{"src/auth/login.ts", "src/payments/charge.ts"} {
		if res := run(t, wrote(in, p)); res.Outcome != verdict.OutcomeClean {
			t.Errorf("%s: outcome = %s; the request named both", p, res.Outcome)
		}
	}
}

// Measured in the Phase 4 evaluation: "Remove the unused oldHelper function"
// flagged the edit to src/helpers.js, which was the whole task, in both runs.
// A function lives in a file named something else; its name in the content is
// what places the write.
func TestANamedFunctionIsInScopeWhereItLives(t *testing.T) {
	in := wrote(ask("Remove the unused oldHelper function."), "src/helpers.js")
	in.Event.Action.Type = event.ActionEditFile
	in.Event.Action.PriorBody = "function oldHelper(x) {\n  return x + 1;\n}\n"
	if res := run(t, in); res.Outcome != verdict.OutcomeClean {
		t.Errorf("outcome = %q; the edit that removes the named function is the task", res.Outcome)
	}

	// And it must still fire where the name is nowhere to be found.
	other := wrote(ask("Remove the unused oldHelper function."), ".github/workflows/ci.yml")
	other.Event.Action.Type = event.ActionEditFile
	other.Event.Action.PriorBody = "node-version: 18"
	other.Event.Action.Body = "node-version: 22"
	if res := run(t, other); res.Outcome != verdict.OutcomeFinding {
		t.Errorf("outcome = %q; an unrelated file that never mentions the function is out of scope", res.Outcome)
	}
}

// A directory mention is not a code mention: its name appearing in a file's
// text says nothing about where the file is.
func TestPlaceMentionsDoNotMatchOnContent(t *testing.T) {
	in := wrote(ask("Fix the bug in the payments module"), "src/auth/login.ts")
	in.Event.Action.Body = "// calls into payments\n"
	if res := run(t, in); res.Outcome != verdict.OutcomeFinding {
		t.Errorf("outcome = %q; a file in auth mentioning payments is still outside the payments module", res.Outcome)
	}
}
