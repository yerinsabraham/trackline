package diffsize_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/yerinsabraham/trackline/engine/internal/event"
	"github.com/yerinsabraham/trackline/engine/internal/intent"
	"github.com/yerinsabraham/trackline/engine/internal/signal"
	"github.com/yerinsabraham/trackline/engine/internal/signal/diffsize"
	"github.com/yerinsabraham/trackline/engine/internal/verdict"
)

type counter struct {
	files []string
	err   error
}

func (c counter) FilesInTurn(string, string) ([]string, error) { return c.files, c.err }

func ask(s string) intent.Intent {
	var in intent.Intent
	in.Add("t1", time.Now(), s)
	return in
}

func input(task string, prior []string, now string) signal.Input {
	return signal.Input{
		Intent: ask(task),
		Event: event.Event{SessionID: "s", TurnID: "t1", Action: event.Action{
			Type: event.ActionWriteFile, ToolName: "Write", Paths: []string{now},
		}},
	}
}

func run(t *testing.T, c diffsize.Counter, in signal.Input) verdict.Result {
	t.Helper()
	res := diffsize.New(c).Check(in)
	if err := res.Validate(); err != nil {
		t.Fatalf("invalid result: %v", err)
	}
	return res
}

func files(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("/w/src/file%d.ts", i)
	}
	return out
}

// A refactor is exactly the work people hand an agent. Firing on one would make
// the check useless, so the default limit is deliberately generous.
func TestOrdinaryWorkStaysUnderTheLimit(t *testing.T) {
	for _, n := range []int{0, 1, 5, 10, 14} {
		res := run(t, counter{files: files(n)}, input("Refactor the auth module", nil, "/w/src/new.ts"))
		if res.Outcome != verdict.OutcomeClean {
			t.Errorf("%d prior files: outcome = %s, want clean", n, res.Outcome)
		}
	}
}

func TestFiresWhenAChangeSprawls(t *testing.T) {
	res := run(t, counter{files: files(20)}, input("Refactor the auth module", nil, "/w/src/new.ts"))
	if res.Outcome != verdict.OutcomeFinding {
		t.Fatalf("outcome = %s, want a finding", res.Outcome)
	}
	v := res.Verdicts[0]
	if v.Severity != verdict.SeverityWarn {
		t.Error("a heuristic warns, it does not block")
	}
	if !strings.Contains(v.Summary, "21 files") {
		t.Errorf("summary = %q; it should count the current write too", v.Summary)
	}
	if len(v.Evidence) != 3 {
		t.Errorf("evidence should carry the ask, the count and the files: %+v", v.Evidence)
	}
}

// How much change is reasonable depends on how much was asked for.
func TestANarrowRequestGetsLessPatience(t *testing.T) {
	small := []string{
		"Fix the typo in the README",
		"Rename the user variable",
		"Just fix the import order",
		"Make a small change to the header",
	}
	for _, task := range small {
		t.Run(task, func(t *testing.T) {
			// Six files is fine for a refactor and not for a typo.
			res := run(t, counter{files: files(6)}, input(task, nil, "/w/src/new.ts"))
			if res.Outcome != verdict.OutcomeFinding {
				t.Errorf("outcome = %s; %q should not touch seven files", res.Outcome, task)
			}
			if !strings.Contains(res.Verdicts[0].Summary, "small change") {
				t.Errorf("summary should say why the limit was lower: %q", res.Verdicts[0].Summary)
			}
		})
	}

	// The same seven files under a broad request are fine.
	res := run(t, counter{files: files(6)}, input("Migrate the auth system to the new API", nil, "/w/src/new.ts"))
	if res.Outcome != verdict.OutcomeClean {
		t.Errorf("outcome = %s; a broad request earns a broad change", res.Outcome)
	}
}

func TestRewritingTheSameFileDoesNotInflateTheCount(t *testing.T) {
	prior := files(4)
	res := run(t, counter{files: prior}, input("Fix the typo", nil, prior[0]))
	if res.Outcome != verdict.OutcomeClean {
		t.Errorf("outcome = %s; editing an already-counted file adds no new file", res.Outcome)
	}
}

// Unmeasurable is not clean, in both directions.
func TestUnmeasurableCases(t *testing.T) {
	t.Run("no memory available", func(t *testing.T) {
		if res := run(t, nil, input("Fix the typo", nil, "/w/a.ts")); res.Outcome != verdict.OutcomeCannotMeasure {
			t.Errorf("outcome = %s; without a record of earlier writes the size is unknown", res.Outcome)
		}
	})
	t.Run("recording unreadable", func(t *testing.T) {
		res := run(t, counter{err: errors.New("corrupt")}, input("Fix the typo", nil, "/w/a.ts"))
		if res.Outcome != verdict.OutcomeCannotMeasure {
			t.Errorf("outcome = %s", res.Outcome)
		}
	})
	t.Run("files not visible", func(t *testing.T) {
		in := input("Fix the typo", nil, "/w/a.ts")
		in.Event.Action.PathsUnknown = true
		in.Event.Action.Paths = nil
		if res := run(t, counter{files: files(2)}, in); res.Outcome != verdict.OutcomeCannotMeasure {
			t.Errorf("outcome = %s; an invisible write cannot be counted", res.Outcome)
		}
	})
	t.Run("host did not group by turn", func(t *testing.T) {
		in := input("Fix the typo", nil, "/w/a.ts")
		in.Event.TurnID = ""
		if res := run(t, counter{}, in); res.Outcome != verdict.OutcomeNotApplicable {
			t.Errorf("outcome = %s; without a turn there is nothing to total", res.Outcome)
		}
	})
}
