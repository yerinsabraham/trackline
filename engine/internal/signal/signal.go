// Package signal defines the one interface every check implements.
//
// Everything that inspects an action is a Signal: the deterministic checks that
// count files and diff sizes, and later the single model-backed check that asks
// whether an action served the stated goal. Putting them behind one interface
// is what stops the model-backed one spreading: it is one implementation among
// several, not a separate path through the engine.
//
// No signals live here. Phase 1 builds the socket; Phase 2 builds the checks.
package signal

import (
	"github.com/yerinsabraham/trackline/engine/internal/event"
	"github.com/yerinsabraham/trackline/engine/internal/intent"
	"github.com/yerinsabraham/trackline/engine/internal/verdict"
)

// Input is everything a check is allowed to see.
//
// Deliberately narrow. A signal that needs the filesystem, the network or the
// clock has to take it as a dependency rather than reach for it, so that
// replaying a recorded session gives the same answer every time.
type Input struct {
	// Event is the action under consideration.
	Event event.Event

	// Intent is every human turn so far. Use Intent.Anchor rather than the
	// latest turn: the latest is often "proceed".
	Intent intent.Intent

	// IntentUnavailable is non-empty when intent could not be read, and says
	// why in words fit to show a user.
	//
	// An empty Intent means two very different things, and a check must not
	// treat them alike: the user has not asked for anything yet, or we read a
	// whole conversation and failed to recognise any of it. The second happened
	// for real — headless sessions mark the human turn differently, so every
	// scripted session looked like it contained no request — and the checks
	// that depend on intent reported not-applicable while actually being blind.
	//
	// When this is set, a check needing intent must return CannotMeasure.
	IntentUnavailable string

	// Rules are the project's stated constraints, from CLAUDE.md, AGENTS.md or
	// config. Empty is a real state and means no rules were found, which is
	// different from no rules being violated.
	Rules []Rule
}

// Rule is one stated constraint, from a project file or configuration.
type Rule struct {
	// ID is stable so a verdict can point at the rule it fired on.
	ID string `json:"id"`
	// Text is the rule as written, for evidence.
	Text string `json:"text"`
	// Source says where it came from, e.g. "CLAUDE.md:12".
	Source string `json:"source,omitempty"`
	// Truncated marks a rule read from content that was cut short. A rule
	// missing its final clause may read as permissive when it is not, so a
	// check must degrade rather than trust it.
	Truncated bool `json:"truncated,omitempty"`
}

// Signal is one check.
//
// Check must not panic and must not block. On Codex a hook that crashes or
// times out is treated as permission to proceed, so a panicking signal is a
// silently disabled one.
type Signal interface {
	// Name is stable and appears in every verdict this signal produces.
	Name() string

	// Check answers for exactly one event. Returning CannotMeasure is a
	// legitimate answer and is always preferable to guessing.
	Check(Input) verdict.Result
}
