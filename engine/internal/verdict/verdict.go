// Package verdict is what a check produces.
//
// Two rules are enforced here rather than left to the checks that implement
// them, because both have already been learned the expensive way on the CI side
// of this project.
//
//  1. A verdict that would interrupt someone must carry evidence. "The agent
//     seems off track" is useless; "the task said auth, this edits
//     payments/charge.ts" is actionable. Validate refuses the first.
//
//  2. A check that could not run reports that it could not run. It never
//     reports clean. The CI gate shipped for months reporting a safety metric
//     of 0.000 that was never computed, because nothing in the type system
//     distinguished "measured zero" from "no measurement". Outcome makes that
//     distinction impossible to lose.
package verdict

import (
	"errors"
	"fmt"
)

// Severity is how loudly a finding should be treated.
type Severity string

const (
	// SeverityInfo is recorded and never shown unless asked for.
	SeverityInfo Severity = "info"
	// SeverityWarn is surfaced to the human but never interrupts.
	SeverityWarn Severity = "warn"
	// SeverityBlock is a finding serious enough to justify stopping the action,
	// if the configured mode allows stopping at all.
	SeverityBlock Severity = "block"
)

// Interrupts reports whether a severity is allowed to reach the user unasked.
func (s Severity) Interrupts() bool { return s == SeverityWarn || s == SeverityBlock }

// EvidenceKind says what sort of thing is being pointed at, so a renderer can
// present it well and a reader can tell a file from a rule at a glance.
type EvidenceKind string

const (
	EvidenceFile    EvidenceKind = "file"
	EvidenceRule    EvidenceKind = "rule"
	EvidenceTurn    EvidenceKind = "turn"
	EvidenceCommand EvidenceKind = "command"
	EvidenceCount   EvidenceKind = "count"
)

// Evidence is one checkable fact behind a verdict.
type Evidence struct {
	Kind  EvidenceKind `json:"kind"`
	Value string       `json:"value"`
	// Note explains why this fact matters, in one short line.
	Note string `json:"note,omitempty"`
}

// Outcome is whether a check produced an answer at all.
type Outcome string

const (
	// OutcomeClean means the check ran and found nothing.
	OutcomeClean Outcome = "clean"
	// OutcomeFinding means the check ran and found something.
	OutcomeFinding Outcome = "finding"
	// OutcomeNotApplicable means there was nothing of this kind to check. No
	// scope was stated, no rules file exists. Harmless and silent.
	OutcomeNotApplicable Outcome = "not-applicable"
	// OutcomeCannotMeasure means the check should have run and could not: the
	// data it needs was missing, unreadable or redacted.
	//
	// This is never clean. A check that silently could not see anything looks
	// exactly like a check that saw nothing wrong, and only one of those is
	// good news.
	OutcomeCannotMeasure Outcome = "cannot-measure"
)

// Result is one check's answer for one event.
type Result struct {
	Signal  string  `json:"signal"`
	Outcome Outcome `json:"outcome"`

	// Reason is required when Outcome is NotApplicable or CannotMeasure, and
	// says which in plain words.
	Reason string `json:"reason,omitempty"`

	// Verdicts is non-empty only when Outcome is Finding.
	Verdicts []Verdict `json:"verdicts,omitempty"`
}

// Verdict is one finding.
type Verdict struct {
	Signal   string     `json:"signal"`
	Severity Severity   `json:"severity"`
	Summary  string     `json:"summary"`
	Evidence []Evidence `json:"evidence"`

	// Suggestion is what the agent or human should do instead. Phase 0 found
	// that a block naming a concrete alternative was corrected 11 times out of
	// 11; whether a bare refusal works as well is untested. Until that is
	// known, a suggestion is treated as load-bearing.
	Suggestion string `json:"suggestion,omitempty"`
}

var (
	ErrNoEvidence = errors.New("verdict has no evidence")
	ErrNoSummary  = errors.New("verdict has no summary")
	ErrNoReason   = errors.New("outcome requires a reason")
)

// Validate refuses a verdict that could not be acted on.
func (v Verdict) Validate() error {
	if v.Summary == "" {
		return fmt.Errorf("%s: %w", v.Signal, ErrNoSummary)
	}
	if v.Severity.Interrupts() && len(v.Evidence) == 0 {
		return fmt.Errorf("%s: %w: a %s verdict must name what it saw", v.Signal, ErrNoEvidence, v.Severity)
	}
	return nil
}

// Validate refuses a result that is internally inconsistent.
func (r Result) Validate() error {
	switch r.Outcome {
	case OutcomeNotApplicable, OutcomeCannotMeasure:
		if r.Reason == "" {
			return fmt.Errorf("%s: %w (%s)", r.Signal, ErrNoReason, r.Outcome)
		}
		if len(r.Verdicts) > 0 {
			return fmt.Errorf("%s: outcome %s cannot carry verdicts", r.Signal, r.Outcome)
		}
	case OutcomeClean:
		if len(r.Verdicts) > 0 {
			return fmt.Errorf("%s: a clean outcome cannot carry verdicts", r.Signal)
		}
	case OutcomeFinding:
		if len(r.Verdicts) == 0 {
			return fmt.Errorf("%s: a finding outcome must carry at least one verdict", r.Signal)
		}
	default:
		return fmt.Errorf("%s: unknown outcome %q", r.Signal, r.Outcome)
	}
	for _, v := range r.Verdicts {
		if err := v.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// Clean is the answer from a check that ran and found nothing.
func Clean(signal string) Result {
	return Result{Signal: signal, Outcome: OutcomeClean}
}

// NotApplicable is the answer when there was nothing of this kind to check.
func NotApplicable(signal, reason string) Result {
	return Result{Signal: signal, Outcome: OutcomeNotApplicable, Reason: reason}
}

// CannotMeasure is the answer when the check should have run and could not.
func CannotMeasure(signal, reason string) Result {
	return Result{Signal: signal, Outcome: OutcomeCannotMeasure, Reason: reason}
}

// Finding is the answer when the check ran and found something.
func Finding(signal string, vs ...Verdict) Result {
	for i := range vs {
		if vs[i].Signal == "" {
			vs[i].Signal = signal
		}
	}
	return Result{Signal: signal, Outcome: OutcomeFinding, Verdicts: vs}
}
