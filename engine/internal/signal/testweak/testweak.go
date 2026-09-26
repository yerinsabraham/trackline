// Package testweak notices a test being made easier to pass instead of the
// code being fixed.
//
// The cheapest way to turn a red build green is to skip the failing case or
// loosen what it asserts. It is often a two-line edit inside a file the agent
// was entitled to touch, so scope and diff size both stay quiet on it. The
// only way to see it is to read what changed in the test itself.
//
// Three things count: a skip or focus marker added (a focused test skips
// every other one), assertions removed, and an exact assertion swapped for
// one that accepts almost anything. It only ever warns: deleting a test that
// tested the wrong thing is sometimes exactly right, and the person can tell.
package testweak

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
const Name = "test-weakened"

// Signal reports a test made easier to pass.
type Signal struct{}

// New builds the check.
func New() Signal { return Signal{} }

func (Signal) Name() string { return Name }

var (
	// Markers that stop a test running, or stop the others running.
	skipRe = regexp.MustCompile(`\b(?:it|test|describe|context|suite)\.(?:skip|only|todo)\s*\(|\b(?:xit|xtest|xdescribe|fit|fdescribe)\s*\(|@pytest\.mark\.(?:skip|skipif|xfail)\b|@unittest\.skip|\bpytest\.skip\s*\(|\bt\.Skip(?:f|Now)?\s*\(|@Disabled\b|@Ignore\b|#\[ignore\]`)

	// A statement that checks something. Counted, not parsed: the question is
	// only whether there are fewer than before.
	assertRe = regexp.MustCompile(`\bexpect\s*\(|\bassert\w*\s*[(!]|\bassert\s|\bt\.(?:Error|Errorf|Fatal|Fatalf|Fail|FailNow)\s*\(|\brequire\.\w+\s*\(|\bshould\.|\.should\b|\bself\.assert\w+\s*\(`)

	// Exact assertions, and the loose ones they get swapped for.
	exactRe = regexp.MustCompile(`\.(?:toBe|toEqual|toStrictEqual|toMatchObject|toHaveBeenCalledWith|toThrow)\s*\(\s*[^)\s]|\bassert(?:Equal|Equals|_eq|\.Equal|\.equal|\.strictEqual|\.deepEqual|\.deepStrictEqual|DeepEqual|ListEqual|DictEqual|Raises)\b|\bassert_eq!`)
	looseRe = regexp.MustCompile(`\.(?:toBeTruthy|toBeDefined|toBeFalsy|toBeUndefined|toBeNull)\s*\(\s*\)|\.not\.toThrow\s*\(|\.toBeGreaterThan(?:OrEqual)?\s*\(\s*-?0\s*\)|\bassert(?:True|IsNotNone|IsNone|\.True|\.NotNil|\.ok)\b|\bexpect\s*\(\s*(?:true|1)\s*\)|\bassert\s+True\b|\bassert!\s*\(\s*true\s*\)`)
)

// asked marks a request that plainly wants tests skipped or removed. Firing
// on "skip the flaky test" would be correct and useless.
var asked = []string{"skip", "disable", "remove the test", "remove test", "delete the test", "delete test", "flaky", "xfail", "loosen", "relax"}

func (s Signal) Check(in signal.Input) verdict.Result {
	a := in.Event.Action
	if a.Type != event.ActionWriteFile && a.Type != event.ActionEditFile {
		return verdict.NotApplicable(Name, "this action does not change a file")
	}
	if a.PathsUnknown {
		return verdict.NotApplicable(Name, "the files this action changes are not known")
	}
	var file string
	for _, p := range a.Paths {
		if isTest(p) {
			file = p
			break
		}
	}
	if file == "" {
		return verdict.NotApplicable(Name, "no test file is being changed")
	}
	if a.Truncated {
		return verdict.CannotMeasure(Name, fmt.Sprintf(
			"the change to %s was too large to read in full", filepath.Base(file)))
	}

	before, after, ok := sides(a)
	if !ok {
		// A new file: nothing to compare against, and a new test file with a
		// skip in it is not a weakened test, so this stays silent.
		return verdict.NotApplicable(Name, fmt.Sprintf(
			"%s is new, so there is no earlier version to compare", filepath.Base(file)))
	}

	var found []string
	var ev []verdict.Evidence
	if added := newMatches(skipRe, before, after); len(added) > 0 {
		found = append(found, "skipped a test")
		ev = append(ev, verdict.Evidence{Kind: verdict.EvidenceRule, Value: strings.Join(added, ", "), Note: "skip marker added"})
	}
	if b, n := count(assertRe, before), count(assertRe, after); n < b {
		found = append(found, fmt.Sprintf("removed %d assertion%s", b-n, plural(b-n)))
		ev = append(ev, verdict.Evidence{Kind: verdict.EvidenceCount, Value: fmt.Sprintf("%d before, %d after", b, n), Note: "assertions"})
	}
	if count(exactRe, after) < count(exactRe, before) && count(looseRe, after) > count(looseRe, before) {
		found = append(found, "loosened an assertion")
		ev = append(ev, verdict.Evidence{Kind: verdict.EvidenceRule, Value: strings.Join(newMatches(looseRe, before, after), ", "), Note: "looser check added"})
	}
	if len(found) == 0 {
		return verdict.Clean(Name)
	}

	if anchor, ok := in.Intent.Anchor(); ok {
		low := strings.ToLower(anchor.Text)
		for _, w := range asked {
			if strings.Contains(low, w) {
				return verdict.Clean(Name)
			}
		}
	}

	ev = append([]verdict.Evidence{{Kind: verdict.EvidenceFile, Value: file, Note: "the test being changed"}}, ev...)
	return verdict.Finding(Name, verdict.Verdict{
		Severity:   verdict.SeverityWarn,
		Target:     file,
		Summary:    fmt.Sprintf("%s in %s", strings.Join(found, ", "), filepath.Base(file)),
		Evidence:   ev,
		Suggestion: "fix the code the test covers rather than the test, unless the test itself is wrong; say which",
	})
}

// sides returns the text before and after the change. A patch carries both;
// an edit carries both when the host sends the replaced text.
func sides(a event.Action) (before, after string, ok bool) {
	if strings.Contains(a.Body, "*** Begin Patch") || strings.HasPrefix(a.Body, "--- ") || strings.Contains(a.Body, "\n@@ ") {
		var b, n strings.Builder
		for _, line := range strings.Split(a.Body, "\n") {
			switch {
			case strings.HasPrefix(line, "---"), strings.HasPrefix(line, "+++"):
			case strings.HasPrefix(line, "-"):
				b.WriteString(line[1:] + "\n")
			case strings.HasPrefix(line, "+"):
				n.WriteString(line[1:] + "\n")
			}
		}
		return b.String(), n.String(), true
	}
	// An edit's replaced text, or, for a whole-file write, what the hook read
	// from disk just before it (runner.fillPriorBody).
	if a.PriorBody != "" {
		return a.PriorBody, a.Body, true
	}
	return "", "", false
}

// isTest recognises test files by the conventions of the common languages.
func isTest(p string) bool {
	base := strings.ToLower(filepath.Base(p))
	switch {
	case strings.HasSuffix(base, "_test.go"),
		strings.Contains(base, ".test."), strings.Contains(base, ".spec."),
		strings.HasPrefix(base, "test_") && strings.HasSuffix(base, ".py"),
		strings.HasSuffix(base, "_test.py"), strings.HasSuffix(base, "_spec.rb"),
		strings.HasSuffix(base, "test.java"), strings.HasSuffix(base, "tests.cs"):
		return true
	}
	dir := "/" + filepath.ToSlash(strings.ToLower(filepath.Dir(p))) + "/"
	return strings.Contains(dir, "/__tests__/") || strings.Contains(dir, "/tests/") || strings.Contains(dir, "/test/") || strings.Contains(dir, "/spec/")
}

func count(re *regexp.Regexp, s string) int { return len(re.FindAllString(s, -1)) }

// newMatches lists matches in after beyond how many times each appeared before.
func newMatches(re *regexp.Regexp, before, after string) []string {
	seen := map[string]int{}
	for _, m := range re.FindAllString(before, -1) {
		seen[strings.TrimSpace(m)]++
	}
	var out []string
	for _, m := range re.FindAllString(after, -1) {
		m = strings.TrimSpace(m)
		if seen[m] > 0 {
			seen[m]--
			continue
		}
		out = append(out, m)
	}
	return out
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
