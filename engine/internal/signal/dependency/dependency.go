// Package dependency notices a package being added to a manifest.
//
// Adding a dependency is a decision with consequences beyond the task: it is
// supply-chain surface, it is licence exposure, and it is the most common form
// of scope creep an agent commits. It is also frequently legitimate, so this
// check reports rather than blocks.
//
// The hard part is telling an addition from an upgrade. Both look like a line
// mentioning a package, and only the before-and-after distinguishes them. Where
// the host supplies both sides, the answer is exact. Where it supplies only the
// new content — a whole-file write — the answer is honestly unavailable, and
// this check says so instead of guessing.
package dependency

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yerinsabraham/trackline/engine/internal/event"
	"github.com/yerinsabraham/trackline/engine/internal/signal"
	"github.com/yerinsabraham/trackline/engine/internal/verdict"
)

// Name is stable and appears on every verdict this check produces.
const Name = "dependency-added"

// Signal reports packages added to a manifest.
type Signal struct{}

// New builds the check.
func New() Signal { return Signal{} }

func (Signal) Name() string { return Name }

// manifests maps a filename to the way that ecosystem declares a dependency.
var manifests = map[string]*regexp.Regexp{
	// "lodash": "^4.17.21"
	"package.json": regexp.MustCompile(`"([@\w][\w.@/-]*)"\s*:\s*"([~^>=<]*[\d*][^"]*)"`),
	// github.com/foo/bar v1.2.3
	"go.mod": regexp.MustCompile(`^\s*([\w.\-]+\.[\w.\-]+/[\w.\-/]+)\s+v\d`),
	// requests==2.31.0, requests>=2
	"requirements.txt": regexp.MustCompile(`^\s*([\w.\-\[\]]+)\s*[=><~!]=`),
	// serde = "1.0"
	"Cargo.toml": regexp.MustCompile(`^\s*([\w-]+)\s*=\s*[{"]`),
	// gem "rails", "~> 7.0"
	"Gemfile": regexp.MustCompile(`^\s*gem\s+["']([\w.\-]+)["']`),
	// "monolog/monolog": "^2.0"
	"composer.json": regexp.MustCompile(`"([\w.\-]+/[\w.\-]+)"\s*:\s*"[~^>=<]*[\d*]`),
}

func (s Signal) Check(in signal.Input) verdict.Result {
	if !couldAddDependency(in.Event.Action.Type) {
		return verdict.NotApplicable(Name, "this action cannot add a dependency")
	}
	// A command that installs states its packages outright, with no manifest
	// diff to read.
	if len(in.Event.Action.Installs) > 0 {
		added := notRequested(in.Event.Action.Installs, in)
		if len(added) == 0 {
			return verdict.Clean(Name)
		}
		return verdict.Finding(Name, verdict.Verdict{
			Severity: verdict.SeverityWarn,
			Target:   strings.Join(added, ","),
			Summary:  fmt.Sprintf("installed %s", strings.Join(added, ", ")),
			Evidence: []verdict.Evidence{
				{Kind: verdict.EvidenceCommand, Value: in.Event.Action.Command, Note: "the command run"},
				{Kind: verdict.EvidenceRule, Value: strings.Join(added, ", "), Note: "packages added"},
			},
			Suggestion: "confirm this dependency was intended before it is committed",
		})
	}

	if in.Event.Action.PathsUnknown {
		// A shell command is how a dependency is usually added: `npm install`
		// touches the manifest without any file write we can see. Measured, a
		// real session added lodash through a command, and this check reported
		// not-applicable on it — which reads as nothing to see here, when the
		// truth was that nobody looked.
		return verdict.CannotMeasure(Name, fmt.Sprintf(
			"%s does not report what it changed, and a command can install a dependency without a visible file write",
			describeTool(in.Event)))
	}

	var manifest string
	var pattern *regexp.Regexp
	for _, p := range in.Event.Action.Paths {
		if re, ok := manifests[filepath.Base(p)]; ok {
			manifest, pattern = p, re
			break
		}
	}
	if pattern == nil {
		return verdict.NotApplicable(Name, "no dependency manifest is being modified")
	}

	// A body cut short may be missing the very line that added something.
	if in.Event.Action.Truncated {
		return verdict.CannotMeasure(Name, fmt.Sprintf(
			"the change to %s was too large to read in full, so added packages could not be identified",
			filepath.Base(manifest)))
	}

	added, ok := addedPackages(in.Event.Action, pattern)
	if ok {
		// Firing on a package the user just asked for is technically correct
		// and genuinely irritating, which is the same thing as wrong. Measured:
		// "add the zod package to package.json" produced an alert saying a
		// dependency had been added. Nobody needs telling.
		added = notRequested(added, in)
	}
	if !ok {
		return verdict.CannotMeasure(Name, fmt.Sprintf(
			"%s is being rewritten whole, so an added package cannot be told from an upgraded one",
			filepath.Base(manifest)))
	}
	if len(added) == 0 {
		return verdict.Clean(Name)
	}

	ev := []verdict.Evidence{{Kind: verdict.EvidenceFile, Value: manifest, Note: "the manifest being changed"}}
	for _, pkg := range added {
		ev = append(ev, verdict.Evidence{Kind: verdict.EvidenceRule, Value: pkg, Note: "package added"})
	}

	return verdict.Finding(Name, verdict.Verdict{
		// Reported, not blocked. Adding a dependency is often exactly right,
		// and a check that stops ordinary work gets uninstalled.
		Severity: verdict.SeverityWarn,
		Target:   strings.Join(added, ","),
		Summary: fmt.Sprintf("added %s to %s",
			strings.Join(added, ", "), filepath.Base(manifest)),
		Evidence:   ev,
		Suggestion: "confirm this dependency was intended before it is committed",
	})
}

// addedPackages returns packages present after the change and absent before.
//
// ok is false when the prior state is unknown, which is the whole-file-write
// case: every package looks new because nothing is known to be old.
func addedPackages(a event.Action, re *regexp.Regexp) (added []string, ok bool) {
	if strings.Contains(a.Body, "*** Begin Patch") {
		return fromPatch(a.Body, re), true
	}
	if a.PriorBody == "" {
		return nil, false
	}

	before := packageSet(a.PriorBody, re)
	for _, p := range packages(a.Body, re) {
		if !before[p] {
			added = append(added, p)
		}
	}
	return added, true
}

// fromPatch reads added lines only. A patch states both sides explicitly, so an
// upgrade appears as a removal and an addition of the same package and is
// correctly excluded.
func fromPatch(patch string, re *regexp.Regexp) []string {
	removed := map[string]bool{}
	var addedLines []string
	for _, line := range strings.Split(patch, "\n") {
		switch {
		case strings.HasPrefix(line, "-"):
			for _, p := range packages(strings.TrimPrefix(line, "-"), re) {
				removed[p] = true
			}
		case strings.HasPrefix(line, "+"):
			addedLines = append(addedLines, strings.TrimPrefix(line, "+"))
		}
	}

	var out []string
	for _, p := range packages(strings.Join(addedLines, "\n"), re) {
		if !removed[p] {
			out = append(out, p)
		}
	}
	return out
}

// packages lists the packages a block of text declares, in the order they
// appear and without repeats. Order matters only so that a verdict reads the
// same way twice.
func packages(text string, re *regexp.Regexp) []string {
	var out []string
	seen := map[string]bool{}
	for _, line := range strings.Split(text, "\n") {
		for _, m := range re.FindAllStringSubmatch(line, -1) {
			if len(m) > 1 && m[1] != "" && !seen[m[1]] {
				seen[m[1]] = true
				out = append(out, m[1])
			}
		}
	}
	return out
}

// packageSet is packages as a set, for membership tests.
func packageSet(text string, re *regexp.Regexp) map[string]bool {
	out := map[string]bool{}
	for _, p := range packages(text, re) {
		out[p] = true
	}
	return out
}

// notRequested drops packages the user explicitly named.
//
// Only an exact name match counts. A vague "add a dependency" does not excuse
// adding something in particular, and a request mentioning one package does not
// license adding three.
func notRequested(added []string, in signal.Input) []string {
	var asked []string
	for _, turn := range in.Intent.Recent(3) {
		asked = append(asked, strings.ToLower(turn.Text))
	}
	if len(asked) == 0 {
		return added
	}

	var out []string
	for _, pkg := range added {
		name := strings.ToLower(pkg)
		// A scoped or pathed name is recognisable by its last segment too:
		// "@scope/thing" is asked for as "thing" about as often.
		short := name
		if i := strings.LastIndex(short, "/"); i >= 0 {
			short = short[i+1:]
		}
		named := false
		for _, text := range asked {
			if strings.Contains(text, name) || (len(short) > 3 && strings.Contains(text, short)) {
				named = true
				break
			}
		}
		if !named {
			out = append(out, pkg)
		}
	}
	return out
}

// couldAddDependency is broader than "edits a manifest".
//
// `npm install lodash` adds a dependency without any file write this hook can
// see, so a command has to reach the paths check and be reported as unchecked
// rather than waved past as irrelevant.
func couldAddDependency(t event.ActionType) bool {
	switch t {
	case event.ActionWriteFile, event.ActionEditFile, event.ActionRunCommand, event.ActionOther:
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
