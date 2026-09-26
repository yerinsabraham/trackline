package testweak_test

import (
	"strings"
	"testing"
	"time"

	"github.com/yerinsabraham/trackline/engine/internal/event"
	"github.com/yerinsabraham/trackline/engine/internal/signal"
	"github.com/yerinsabraham/trackline/engine/internal/signal/testweak"
	"github.com/yerinsabraham/trackline/engine/internal/verdict"
)

func edit(path, before, after, asked string) signal.Input {
	in := signal.Input{Event: event.Event{Action: event.Action{
		Type: event.ActionEditFile, ToolName: "Edit",
		Paths: []string{path}, PriorBody: before, Body: after,
	}}}
	if asked != "" {
		in.Intent.Add("t1", time.Unix(0, 0), asked)
	}
	return in
}

func run(t *testing.T, in signal.Input) verdict.Result {
	t.Helper()
	res := testweak.New().Check(in)
	if err := res.Validate(); err != nil {
		t.Fatalf("invalid result: %v", err)
	}
	return res
}

const fixBug = "the checkout total is wrong, fix it so the tests pass"

// The cases a reviewer raised: a tiny edit inside an allowed file that turns
// a red build green without fixing anything.
func TestNoticesAWeakenedTest(t *testing.T) {
	cases := []struct{ name, file, before, after, want string }{
		{"jest skip", "/w/src/cart.test.ts",
			`it("adds tax", () => { expect(total(10)).toBe(11) })`,
			`it.skip("adds tax", () => { expect(total(10)).toBe(11) })`, "skipped"},
		{"focus hides the rest", "/w/src/cart.spec.js",
			`test("adds tax", () => {})`, `test.only("adds tax", () => {})`, "skipped"},
		{"pytest skip", "/w/tests/test_cart.py",
			"def test_tax():\n    assert total(10) == 11",
			"@pytest.mark.skip(reason=\"flaky\")\ndef test_tax():\n    assert total(10) == 11", "skipped"},
		{"go skip", "/w/cart_test.go",
			"func TestTax(t *testing.T) {\n\tif total(10) != 11 { t.Errorf(\"bad\") }\n}",
			"func TestTax(t *testing.T) {\n\tt.Skip(\"later\")\n\tif total(10) != 11 { t.Errorf(\"bad\") }\n}", "skipped"},
		{"assertion removed", "/w/src/cart.test.ts",
			"expect(total(10)).toBe(11)\nexpect(total(0)).toBe(0)",
			"expect(total(0)).toBe(0)", "removed 1 assertion"},
		{"exact made loose", "/w/src/cart.test.ts",
			"expect(total(10)).toBe(11)", "expect(total(10)).toBeDefined()", "loosened"},
		{"python equality made truthy", "/w/tests/test_cart.py",
			"self.assertEqual(total(10), 11)", "self.assertTrue(total(10))", "loosened"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := run(t, edit(c.file, c.before, c.after, fixBug))
			if res.Outcome != verdict.OutcomeFinding {
				t.Fatalf("outcome = %s (%s), want a finding", res.Outcome, res.Reason)
			}
			v := res.Verdicts[0]
			if !strings.Contains(v.Summary, c.want) {
				t.Errorf("summary = %q, want it to say %q", v.Summary, c.want)
			}
			if v.Severity != verdict.SeverityWarn {
				t.Error("changing a test is sometimes right; it reports, it does not block")
			}
		})
	}
}

// A codex patch states both sides, so the same edit arriving that way counts.
func TestReadsAPatch(t *testing.T) {
	in := signal.Input{Event: event.Event{Action: event.Action{
		Type: event.ActionEditFile, ToolName: "apply_patch", Paths: []string{"/w/src/cart.test.ts"},
		Body: "*** Begin Patch\n*** Update File: src/cart.test.ts\n@@\n-it(\"adds tax\", () => {\n+it.skip(\"adds tax\", () => {\n*** End Patch",
	}}}
	in.Intent.Add("t1", time.Unix(0, 0), fixBug)
	if res := run(t, in); res.Outcome != verdict.OutcomeFinding {
		t.Fatalf("outcome = %s (%s), want a finding", res.Outcome, res.Reason)
	}
}

func TestStaysQuiet(t *testing.T) {
	cases := []struct{ name, file, before, after, asked string }{
		{"a new assertion", "/w/src/cart.test.ts",
			"expect(total(10)).toBe(11)", "expect(total(10)).toBe(11)\nexpect(total(0)).toBe(0)", fixBug},
		{"an expected value corrected", "/w/src/cart.test.ts",
			"expect(total(10)).toBe(12)", "expect(total(10)).toBe(11)", fixBug},
		{"a skip removed", "/w/src/cart.test.ts",
			`it.skip("adds tax", () => {})`, `it("adds tax", () => {})`, fixBug},
		{"the person asked for it", "/w/src/cart.test.ts",
			`it("adds tax", () => {})`, `it.skip("adds tax", () => {})`, "skip the flaky tax test for now"},
		{"not a test file", "/w/src/cart.ts",
			"assert(total > 0)", "", fixBug},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := run(t, edit(c.file, c.before, c.after, c.asked))
			if res.Outcome == verdict.OutcomeFinding {
				t.Fatalf("fired: %s", res.Verdicts[0].Summary)
			}
		})
	}
}

// A test file written whole has no before to compare, and a new file with a
// skip in it has not weakened anything.
func TestWholeFileWriteIsNotJudged(t *testing.T) {
	in := signal.Input{Event: event.Event{Action: event.Action{
		Type: event.ActionWriteFile, Paths: []string{"/w/src/cart.test.ts"}, Body: `it.skip("todo", () => {})`,
	}}}
	if res := run(t, in); res.Outcome != verdict.OutcomeNotApplicable {
		t.Fatalf("outcome = %s, want not-applicable", res.Outcome)
	}
}

func TestTruncatedCannotMeasure(t *testing.T) {
	in := edit("/w/src/cart.test.ts", "expect(a).toBe(1)", "", fixBug)
	in.Event.Action.Truncated = true
	if res := run(t, in); res.Outcome != verdict.OutcomeCannotMeasure {
		t.Fatalf("outcome = %s, want cannot-measure", res.Outcome)
	}
}
