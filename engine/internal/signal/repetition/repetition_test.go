package repetition_test

import (
	"strings"
	"testing"

	"github.com/yerinsabraham/trackline/engine/internal/event"
	"github.com/yerinsabraham/trackline/engine/internal/signal"
	"github.com/yerinsabraham/trackline/engine/internal/signal/repetition"
	"github.com/yerinsabraham/trackline/engine/internal/verdict"
)

type history []event.Event

func (h history) ActionsInTurn(string, string) ([]event.Event, error) { return h, nil }

func write(path string) event.Event {
	return event.Event{SessionID: "s", TurnID: "t1", Action: event.Action{
		Type: event.ActionEditFile, ToolName: "Edit", Paths: []string{path},
	}}
}

func cmd(c string) event.Event {
	return event.Event{SessionID: "s", TurnID: "t1", Action: event.Action{
		Type: event.ActionRunCommand, ToolName: "Bash", Command: c, PathsUnknown: true,
	}}
}

func run(t *testing.T, h repetition.History, ev event.Event) verdict.Result {
	t.Helper()
	res := repetition.New(h).Check(signal.Input{Event: ev})
	if err := res.Validate(); err != nil {
		t.Fatalf("invalid result: %v", err)
	}
	return res
}

func repeat(ev event.Event, n int) history {
	h := make(history, n)
	for i := range h {
		h[i] = ev
	}
	return h
}

// The ordinary rhythm of real work is write, test, fix, test. Firing on that
// would make the check unusable.
func TestNormalIterationIsSilent(t *testing.T) {
	ev := write("/w/src/a.ts")
	for _, n := range []int{0, 1, 2} {
		if res := run(t, repeat(ev, n), ev); res.Outcome != verdict.OutcomeClean {
			t.Errorf("%d prior attempts: outcome = %s, want clean", n, res.Outcome)
		}
	}
}

func TestFiresOnAStuckLoop(t *testing.T) {
	ev := write("/w/src/a.ts")
	res := run(t, repeat(ev, 3), ev) // three before, this is the fourth
	if res.Outcome != verdict.OutcomeFinding {
		t.Fatalf("outcome = %s, want a finding at the fourth attempt", res.Outcome)
	}
	v := res.Verdicts[0]
	if !strings.Contains(v.Summary, "4 times") {
		t.Errorf("summary = %q, should count this attempt too", v.Summary)
	}
	if v.Severity != verdict.SeverityWarn {
		t.Error("repetition is a proxy for being stuck, not proof; it warns")
	}
	if v.Suggestion == "" {
		t.Error("a stuck agent should be told to stop and explain rather than retry")
	}
}

// Working across several files is progress, not a loop.
func TestDifferentActionsDoNotAccumulate(t *testing.T) {
	h := history{write("/w/a.ts"), write("/w/b.ts"), write("/w/c.ts"), write("/w/d.ts")}
	if res := run(t, h, write("/w/e.ts")); res.Outcome != verdict.OutcomeClean {
		t.Errorf("outcome = %s; five different files is work, not repetition", res.Outcome)
	}
}

func TestCommandsAreComparedByText(t *testing.T) {
	c := cmd("go test ./...")
	if res := run(t, repeat(c, 3), c); res.Outcome != verdict.OutcomeFinding {
		t.Errorf("outcome = %s; the same command four times is a loop", res.Outcome)
	}
	// Whitespace should not make two identical commands look different.
	if res := run(t, repeat(cmd("go   test  ./..."), 3), c); res.Outcome != verdict.OutcomeFinding {
		t.Errorf("outcome = %s; spacing must not defeat the comparison", res.Outcome)
	}
	// Genuinely different commands are not repeats.
	h := history{cmd("go build ./..."), cmd("go vet ./..."), cmd("gofmt -l .")}
	if res := run(t, h, c); res.Outcome != verdict.OutcomeClean {
		t.Errorf("outcome = %s; different commands are not a loop", res.Outcome)
	}
}

// An action that cannot be told apart from another cannot be counted, and
// saying nothing would look like saying it is fine.
func TestIndistinguishableActionsCannotBeMeasured(t *testing.T) {
	for _, ev := range []event.Event{
		{SessionID: "s", TurnID: "t1", Action: event.Action{
			Type: event.ActionRunCommand, ToolName: "Bash", PathsUnknown: true}},
		{SessionID: "s", TurnID: "t1", Action: event.Action{
			Type: event.ActionWriteFile, ToolName: "Write", PathsUnknown: true}},
		{SessionID: "s", TurnID: "t1", Action: event.Action{
			Type: event.ActionCallTool, ToolNameUnknown: true, PathsUnknown: true}},
	} {
		if res := run(t, history{}, ev); res.Outcome != verdict.OutcomeCannotMeasure {
			t.Errorf("%s: outcome = %s, want cannot-measure", ev.Action.Type, res.Outcome)
		}
	}
}

func TestNoHistoryMeansNoMeasurement(t *testing.T) {
	if res := run(t, nil, write("/w/a.ts")); res.Outcome != verdict.OutcomeCannotMeasure {
		t.Errorf("outcome = %s; without a record, repeats cannot be counted", res.Outcome)
	}
}

// Measured on real usage: someone building a markdown file section by section
// was told they might be stuck. Four edits to one file, each writing something
// different, is work.
func TestGrowingADocumentIsNotALoop(t *testing.T) {
	edit := func(body string) event.Event {
		return event.Event{SessionID: "s", TurnID: "t1", Action: event.Action{
			Type: event.ActionEditFile, ToolName: "Edit",
			Paths: []string{"/w/CANDIDATE.md"}, Body: body,
		}}
	}
	h := history{
		edit("## Summary\nshort"),
		edit("## Summary\nshort\n\n## Experience\nquite a lot more text here than before"),
		edit("## Summary\nshort\n\n## Experience\nquite a lot more text here than before\n\n## Skills\nand more again, growing steadily each time"),
	}
	now := edit("## Summary\nshort\n\n## Experience\nquite a lot more text here than before\n\n## Skills\nand more again, growing steadily each time\n\n## Contact\nplus a final section that makes it longer still")

	if res := run(t, h, now); res.Outcome != verdict.OutcomeClean {
		t.Errorf("outcome = %s; a document being written is progress, not a loop", res.Outcome)
	}
}

// The case it must still catch: the same fix attempted over and over.
func TestRetryingTheSameChangeIsALoop(t *testing.T) {
	attempt := func(body string) event.Event {
		return event.Event{SessionID: "s", TurnID: "t1", Action: event.Action{
			Type: event.ActionEditFile, ToolName: "Edit",
			Paths: []string{"/w/src/parser.ts"}, Body: body,
		}}
	}
	h := history{
		attempt("return parse(input, {strict: true})"),
		attempt("return parse(input, {strict: false})"),
		attempt("return parse(input, {strict: true, safe: 1})"),
	}
	now := attempt("return parse(input, {strict: false, safe: 0})")

	if res := run(t, h, now); res.Outcome != verdict.OutcomeFinding {
		t.Errorf("outcome = %s; four near-identical attempts at one line is a loop", res.Outcome)
	}
}

// Commands carry no body, and the same command four times is the clearest loop
// there is.
func TestIdenticalCommandsStillLoop(t *testing.T) {
	c := cmd("npm test -- auth")
	if res := run(t, repeat(c, 3), c); res.Outcome != verdict.OutcomeFinding {
		t.Errorf("outcome = %s; the same command four times is a loop", res.Outcome)
	}
}
