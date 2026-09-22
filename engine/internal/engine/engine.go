// Package engine runs signals over events and collects the answers.
//
// It is deliberately dull. All the judgement lives in the signals; this decides
// nothing except the order things happen in and what to do when a signal
// misbehaves.
package engine

import (
	"fmt"

	"github.com/yerinsabraham/trackline/engine/internal/event"
	"github.com/yerinsabraham/trackline/engine/internal/intent"
	"github.com/yerinsabraham/trackline/engine/internal/signal"
	"github.com/yerinsabraham/trackline/engine/internal/verdict"
)

// Engine holds the configured signals.
type Engine struct {
	signals []signal.Signal
}

// New builds an engine from a set of signals.
func New(signals ...signal.Signal) *Engine {
	return &Engine{signals: signals}
}

// Report is everything the engine concluded about one event.
type Report struct {
	Event   event.Event      `json:"event"`
	Results []verdict.Result `json:"results"`
}

// Findings returns just the verdicts, across all signals.
func (r Report) Findings() []verdict.Verdict {
	var out []verdict.Verdict
	for _, res := range r.Results {
		out = append(out, res.Verdicts...)
	}
	return out
}

// Blocked reports whether any finding is severe enough to justify stopping the
// action. Whether it is actually stopped is a mode decision made elsewhere;
// this only says the grounds exist.
func (r Report) Blocked() bool {
	for _, v := range r.Findings() {
		if v.Severity == verdict.SeverityBlock {
			return true
		}
	}
	return false
}

// Unmeasured returns the signals that should have run and could not. These are
// reported rather than dropped: a check that silently could not see anything
// looks exactly like one that saw nothing wrong.
func (r Report) Unmeasured() []verdict.Result {
	var out []verdict.Result
	for _, res := range r.Results {
		if res.Outcome == verdict.OutcomeCannotMeasure {
			out = append(out, res)
		}
	}
	return out
}

// Run puts one event through every signal.
//
// A signal that panics is converted into a CannotMeasure result rather than
// taking the process down. On Codex a crashed hook is a silently disabled hook,
// so one bad signal must not switch off all the others.
func (e *Engine) Run(ev event.Event, in intent.Intent, rules []signal.Rule, intentUnavailable ...string) Report {
	rep := Report{Event: ev}
	input := signal.Input{Event: ev, Intent: in, Rules: rules}
	if len(intentUnavailable) > 0 {
		input.IntentUnavailable = intentUnavailable[0]
	}

	for _, s := range e.signals {
		rep.Results = append(rep.Results, runOne(s, input))
	}
	return rep
}

func runOne(s signal.Signal, in signal.Input) (res verdict.Result) {
	// The recover has to be installed before anything touches s, including
	// Name(). A nil entry in the signal list, or a Name() that panics, would
	// otherwise take the process down before the guard existed — and on Codex a
	// crashed hook is read as permission to proceed, so the whole safety path
	// would silently switch off.
	name := "unknown"
	defer func() {
		if r := recover(); r != nil {
			res = verdict.CannotMeasure(name, fmt.Sprintf("the check panicked: %v", r))
		}
	}()

	if s == nil {
		return verdict.CannotMeasure(name, "a nil signal was registered")
	}
	name = s.Name()
	if name == "" {
		return verdict.CannotMeasure("unnamed", "the check has no name, so its findings could not be attributed")
	}

	res = s.Check(in)
	if res.Signal == "" {
		res.Signal = name
	}

	// A signal that returns something inconsistent is a bug in that signal, and
	// the honest report of a buggy check is that it did not measure anything.
	// Passing its output through would let a malformed verdict reach a user.
	if err := res.Validate(); err != nil {
		return verdict.CannotMeasure(name, fmt.Sprintf("the check returned an invalid result: %v", err))
	}
	return res
}
