package outside_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yerinsabraham/trackline/engine/internal/event"
	"github.com/yerinsabraham/trackline/engine/internal/signal"
	"github.com/yerinsabraham/trackline/engine/internal/signal/outside"
	"github.com/yerinsabraham/trackline/engine/internal/verdict"
)

func write(p string) signal.Input {
	return signal.Input{Event: event.Event{Action: event.Action{Type: event.ActionEditFile, Paths: []string{p}}}}
}

func TestOutsideTheProjectIsRefused(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "app")
	os.MkdirAll(filepath.Join(root, "src"), 0o755)
	s := outside.New(root)
	for p, want := range map[string]verdict.Outcome{
		filepath.Join(root, "src", "new.ts"):          verdict.OutcomeClean,
		filepath.Join(root, "brand-new-dir", "x.txt"): verdict.OutcomeClean,
		"src/relative.ts":                             verdict.OutcomeClean,
		filepath.Join(base, "app-typo", "hello.txt"):  verdict.OutcomeFinding, // the measured typo
		filepath.Join(root, "..", "sibling.txt"):      verdict.OutcomeFinding,
		"/etc/hosts":                                  verdict.OutcomeFinding,
	} {
		r := s.Check(write(p))
		if r.Outcome != want {
			t.Errorf("%s: %s, want %s", p, r.Outcome, want)
		}
		if r.Outcome == verdict.OutcomeFinding && r.Verdicts[0].Severity != verdict.SeverityBlock {
			t.Errorf("%s: must block", p)
		}
	}
}

// The same folder reached through a link is inside, not outside.
func TestALinkedProjectIsStillInside(t *testing.T) {
	base := t.TempDir()
	real := filepath.Join(base, "real")
	os.MkdirAll(real, 0o755)
	link := filepath.Join(base, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skip(err)
	}
	if r := outside.New(link).Check(write(filepath.Join(real, "a.txt"))); r.Outcome != verdict.OutcomeClean {
		t.Fatalf("%s: %s", r.Outcome, r.Reason)
	}
}

func TestOnlyWritesAreJudged(t *testing.T) {
	in := signal.Input{Event: event.Event{Action: event.Action{Type: event.ActionReadFile, Paths: []string{"/etc/hosts"}}}}
	if r := outside.New("/work/app").Check(in); r.Outcome != verdict.OutcomeNotApplicable {
		t.Fatal(r.Outcome)
	}
}
