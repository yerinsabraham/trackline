// Package scope notices an agent writing somewhere the request never mentioned.
//
// This is the check the product is for, and it is also the one most likely to
// cry wolf, so it is built deliberately narrow. It fires only when both halves
// are unambiguous: the request explicitly named a place, and the write landed
// in a different top-level area of the tree.
//
// Everything else declines. A request that names nowhere has no scope to be
// outside of. A write into an area the request did name is fine. A write the
// engine cannot see is reported as unchecked, not as clean.
//
// The cost of that narrowness is real: it will miss drift. The alternative was
// measured against how requests actually read, and loose matching fires on
// correct work. "Add rate limiting" legitimately produces
// `src/middleware/throttle.ts`, which shares no word with the request. A check
// that flags that is noise, and noise is what gets a tool uninstalled. Missing
// some drift is recoverable; being ignored is not.
package scope

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yerinsabraham/trackline/engine/internal/event"
	"github.com/yerinsabraham/trackline/engine/internal/signal"
	"github.com/yerinsabraham/trackline/engine/internal/verdict"
)

// Name is stable and appears on every verdict this check produces.
const Name = "scope"

// Signal compares written paths against the places a request named.
type Signal struct {
	// Root is the project root, so paths can be read relative to it. Without
	// it every absolute path shares the same leading segments and nothing is
	// ever out of scope.
	Root string
}

// New builds the check.
func New(root string) Signal { return Signal{Root: root} }

func (Signal) Name() string { return Name }

func (s Signal) Check(in signal.Input) verdict.Result {
	if !writes(in.Event.Action.Type) {
		return verdict.NotApplicable(Name, "this action does not write to a file")
	}

	if in.IntentUnavailable != "" {
		return verdict.CannotMeasure(Name, in.IntentUnavailable)
	}
	anchor, ok := in.Intent.Anchor()
	if !ok {
		return verdict.NotApplicable(Name, "nothing substantive has been asked yet, so there is no scope to compare against")
	}

	// Intent is often a task, then a correction, then a constraint. Reading
	// only the anchor would call a file mentioned two turns ago out of scope.
	var mentions []Mention
	for _, turn := range in.Intent.Recent(3) {
		mentions = append(mentions, Mentions(turn.Text)...)
	}
	if len(mentions) == 0 {
		return verdict.NotApplicable(Name,
			"the request does not name a file or directory, so there is no stated scope")
	}

	if in.Event.Action.PathsUnknown {
		return verdict.CannotMeasure(Name, fmt.Sprintf(
			"%s does not report which files it touches, so scope could not be checked",
			describeTool(in.Event)))
	}
	if len(in.Event.Action.Paths) == 0 {
		return verdict.NotApplicable(Name, "this action touches no files")
	}

	// The segments the request actually pointed at, with container directories
	// stripped. Comparing on those is what lets "src/auth" match
	// "test/auth/login.test.ts" while still distinguishing it from
	// "src/payments/charge.ts".
	inScope := map[string]bool{}
	var named []string
	for _, m := range mentions {
		named = append(named, m.Raw)
		for _, p := range m.Parts {
			inScope[p] = true
		}
	}

	var vs []verdict.Verdict
	for _, path := range in.Event.Action.Paths {
		rel := s.relative(path)
		if !strings.Contains(filepath.ToSlash(rel), "/") {
			// A file directly at the project root belongs to no area, so no
			// area can claim it is out of place.
			continue
		}

		segs := Segments(rel)
		if len(segs) == 0 {
			continue
		}
		overlap := false
		for _, seg := range segs {
			if inScope[seg] {
				overlap = true
				break
			}
		}
		if overlap {
			continue
		}

		vs = append(vs, verdict.Verdict{
			// Warn, never block. This is a heuristic, and a heuristic does not
			// get to stop someone's work until it has a measured false-alarm
			// rate that earns it.
			Severity: verdict.SeverityWarn,
			Target:   rel,
			Summary:  fmt.Sprintf("wrote to %s, which the request did not mention", rel),
			Evidence: []verdict.Evidence{
				{Kind: verdict.EvidenceTurn, Value: anchor.Text, Note: "what was asked"},
				{Kind: verdict.EvidenceFile, Value: rel, Note: "what was written"},
				{Kind: verdict.EvidenceRule, Value: strings.Join(sorted(named), ", "),
					Note: "the places the request named"},
			},
			Suggestion: fmt.Sprintf("if this is needed for the task, say so; otherwise stay in %s",
				strings.Join(sorted(named), ", ")),
		})
	}

	if len(vs) == 0 {
		return verdict.Clean(Name)
	}
	return verdict.Finding(Name, vs...)
}

// relative reduces a path to its position in the project, so the leading
// segments of an absolute path do not swamp the comparison.
func (s Signal) relative(path string) string {
	if s.Root == "" {
		return path
	}
	if rel, err := filepath.Rel(s.Root, path); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return path
}

// sorted returns a stable copy, so a verdict reads the same way twice.
func sorted(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

func writes(t event.ActionType) bool {
	switch t {
	case event.ActionWriteFile, event.ActionEditFile, event.ActionDeleteFile:
		return true
	default:
		return false
	}
}

func describeTool(e event.Event) string {
	if e.Action.ToolName != "" {
		return e.Action.ToolName
	}
	return string(e.Action.Type)
}
