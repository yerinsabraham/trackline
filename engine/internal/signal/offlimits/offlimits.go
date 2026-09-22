// Package offlimits refuses writes to files the project has put out of bounds.
//
// This is the most objective check there is. It does not infer intent, guess
// scope, or consult a model: either the path matches a pattern the user
// declared, or it does not. When it fires, the evidence is the pattern and the
// path, and there is nothing to argue about except whether the pattern was
// right.
//
// It is deliberately the first check built. A tool that interrupts wrongly gets
// muted on the first day, so the checks that cannot be wrong come first and
// earn the right to the ones that can.
package offlimits

import (
	"fmt"
	"strings"

	"github.com/yerinsabraham/trackline/engine/internal/event"
	"github.com/yerinsabraham/trackline/engine/internal/signal"
	"github.com/yerinsabraham/trackline/engine/internal/verdict"
)

// Name is stable and appears on every verdict this check produces.
const Name = "off-limits"

// Signal checks writes against a list of protected patterns.
type Signal struct {
	// Patterns come from configuration, which ships defaults covering secrets.
	// A pattern beginning with "!" is an exception: a path matching it is never
	// protected, whatever else matches.
	Patterns []string
}

// New builds the check.
func New(patterns []string) Signal { return Signal{Patterns: patterns} }

// exempt reports whether a path matches an exception pattern.
//
// Exceptions exist because the obvious protective pattern over-reaches.
// Protecting ".env.*" also catches ".env.example", which is a committed
// template that people edit all day, and firing on it would be a false alarm on
// ordinary work. The narrower pattern is not available: ".env.*" is exactly
// what protects ".env.production".
func (s Signal) exempt(path string) (string, bool) {
	for _, p := range s.Patterns {
		if !strings.HasPrefix(p, "!") {
			continue
		}
		if Match(strings.TrimPrefix(p, "!"), path) {
			return p, true
		}
	}
	return "", false
}

func (Signal) Name() string { return Name }

// Check answers for one event.
func (s Signal) Check(in signal.Input) verdict.Result {
	if len(s.Patterns) == 0 {
		return verdict.NotApplicable(Name, "no protected paths are configured")
	}

	if !couldWrite(in.Event.Action.Type) {
		return verdict.NotApplicable(Name, "this action cannot write to a file")
	}

	// A shell command, or any action whose files we cannot see, might well
	// write to a protected path: `echo TOKEN=x >> .env` is a write by any
	// reasonable reading. Reporting "not applicable" here would be the exact
	// false-clean this project keeps finding in itself, so it is reported as
	// unchecked instead.
	if in.Event.Action.PathsUnknown {
		return verdict.CannotMeasure(Name, fmt.Sprintf(
			"%s does not report which files it touches, so protected paths could not be checked",
			describeTool(in.Event)))
	}
	if len(in.Event.Action.Paths) == 0 {
		return verdict.NotApplicable(Name, "this action touches no files")
	}

	var vs []verdict.Verdict
	for _, path := range in.Event.Action.Paths {
		if _, ok := s.exempt(path); ok {
			continue
		}
		for _, pattern := range s.Patterns {
			if strings.HasPrefix(pattern, "!") || !Match(pattern, path) {
				continue
			}
			vs = append(vs, verdict.Verdict{
				Severity: verdict.SeverityBlock,
				Summary:  fmt.Sprintf("writing to a protected path: %s", path),
				Evidence: []verdict.Evidence{
					{Kind: verdict.EvidenceFile, Value: path, Note: "the file being written"},
					{Kind: verdict.EvidenceRule, Value: pattern, Note: "the pattern it matches"},
				},
				Suggestion: "leave this file alone; if the change is genuinely needed, make it by hand",
			})
			break // one verdict per path, naming the first pattern that matched
		}
	}

	if len(vs) == 0 {
		return verdict.Clean(Name)
	}
	return verdict.Finding(Name, vs...)
}

// couldWrite is deliberately broader than "is a write".
//
// A shell command is not classified as a write, but `echo X >> .env` plainly is
// one. Anything that might modify a file has to reach the paths check, so that
// an action we cannot see through is reported as unchecked rather than waved
// past as irrelevant.
func couldWrite(t event.ActionType) bool {
	switch t {
	case event.ActionWriteFile, event.ActionEditFile, event.ActionDeleteFile,
		event.ActionRunCommand, event.ActionOther:
		return true
	default:
		// Reads and tool calls cannot modify a file on disk.
		return false
	}
}

func describeTool(e event.Event) string {
	if e.Action.ToolName != "" {
		return e.Action.ToolName
	}
	return string(e.Action.Type)
}
