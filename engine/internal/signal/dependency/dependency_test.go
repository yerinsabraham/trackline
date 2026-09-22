package dependency_test

import (
	"strings"
	"testing"

	"github.com/yerinsabraham/trackline/engine/internal/event"
	"github.com/yerinsabraham/trackline/engine/internal/signal"
	"github.com/yerinsabraham/trackline/engine/internal/signal/dependency"
	"github.com/yerinsabraham/trackline/engine/internal/verdict"
)

func edit(path, before, after string) signal.Input {
	return signal.Input{Event: event.Event{Action: event.Action{
		Type: event.ActionEditFile, ToolName: "Edit",
		Paths: []string{path}, PriorBody: before, Body: after,
	}}}
}

func run(t *testing.T, in signal.Input) verdict.Result {
	t.Helper()
	res := dependency.New().Check(in)
	if err := res.Validate(); err != nil {
		t.Fatalf("invalid result: %v", err)
	}
	return res
}

func TestNoticesAnAddedPackage(t *testing.T) {
	cases := []struct{ file, before, after, want string }{
		{"/w/package.json",
			`{"dependencies":{"react":"^18.0.0"}}`,
			`{"dependencies":{"react":"^18.0.0","left-pad":"^1.3.0"}}`, "left-pad"},
		{"/w/go.mod",
			"require (\n\tgithub.com/a/b v1.0.0\n)",
			"require (\n\tgithub.com/a/b v1.0.0\n\tgithub.com/evil/pkg v0.1.0\n)", "github.com/evil/pkg"},
		{"/w/requirements.txt", "requests==2.31.0", "requests==2.31.0\nleftpad==1.0.0", "leftpad"},
		{"/w/Cargo.toml", `serde = "1.0"`, "serde = \"1.0\"\nsketchy = \"0.1\"", "sketchy"},
		{"/w/Gemfile", `gem "rails", "~> 7.0"`, "gem \"rails\", \"~> 7.0\"\ngem \"sketchy\"", "sketchy"},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			res := run(t, edit(c.file, c.before, c.after))
			if res.Outcome != verdict.OutcomeFinding {
				t.Fatalf("outcome = %s (%s), want a finding", res.Outcome, res.Reason)
			}
			if !strings.Contains(res.Verdicts[0].Summary, c.want) {
				t.Errorf("summary = %q, want it to name %q", res.Verdicts[0].Summary, c.want)
			}
			if res.Verdicts[0].Severity != verdict.SeverityWarn {
				t.Error("adding a dependency is often legitimate; it reports, it does not block")
			}
		})
	}
}

// The distinction the whole check turns on. An upgrade is not an addition, and
// firing on every version bump would make this unusable.
func TestUpgradeIsNotAnAddition(t *testing.T) {
	for _, c := range []struct{ file, before, after string }{
		{"/w/package.json", `{"dependencies":{"react":"^18.0.0"}}`, `{"dependencies":{"react":"^18.2.0"}}`},
		{"/w/requirements.txt", "requests==2.31.0", "requests==2.32.0"},
		{"/w/go.mod", "github.com/a/b v1.0.0", "github.com/a/b v1.1.0"},
	} {
		t.Run(c.file, func(t *testing.T) {
			if res := run(t, edit(c.file, c.before, c.after)); res.Outcome != verdict.OutcomeClean {
				t.Errorf("outcome = %s; a version bump is not a new dependency", res.Outcome)
			}
		})
	}
}

func TestRemovalIsNotAnAddition(t *testing.T) {
	res := run(t, edit("/w/package.json",
		`{"dependencies":{"react":"^18.0.0","left-pad":"^1.3.0"}}`,
		`{"dependencies":{"react":"^18.0.0"}}`))
	if res.Outcome != verdict.OutcomeClean {
		t.Errorf("outcome = %s; removing a package adds nothing", res.Outcome)
	}
}

// A patch states both sides, so an upgrade shows as a removal and an addition
// of the same package and must cancel out.
func TestPatchDistinguishesUpgradeFromAddition(t *testing.T) {
	upgrade := signal.Input{Event: event.Event{Action: event.Action{
		Type: event.ActionEditFile, ToolName: "apply_patch", Paths: []string{"/w/package.json"},
		Body: "*** Begin Patch\n*** Update File: package.json\n-    \"react\": \"^18.0.0\"\n+    \"react\": \"^18.2.0\"\n*** End Patch\n",
	}}}
	if res := run(t, upgrade); res.Outcome != verdict.OutcomeClean {
		t.Errorf("outcome = %s; a patch that bumps a version adds nothing", res.Outcome)
	}

	addition := signal.Input{Event: event.Event{Action: event.Action{
		Type: event.ActionEditFile, ToolName: "apply_patch", Paths: []string{"/w/package.json"},
		Body: "*** Begin Patch\n*** Update File: package.json\n+    \"left-pad\": \"^1.3.0\"\n*** End Patch\n",
	}}}
	res := run(t, addition)
	if res.Outcome != verdict.OutcomeFinding || !strings.Contains(res.Verdicts[0].Summary, "left-pad") {
		t.Errorf("outcome = %s; a patch that adds a package is an addition", res.Outcome)
	}
}

// A whole-file write says nothing about what was there before, so every package
// looks new. Guessing would fire on every dependency in the file.
func TestWholeFileWriteCannotBeMeasured(t *testing.T) {
	res := run(t, signal.Input{Event: event.Event{Action: event.Action{
		Type: event.ActionWriteFile, ToolName: "Write", Paths: []string{"/w/package.json"},
		Body: `{"dependencies":{"react":"^18.0.0","left-pad":"^1.3.0"}}`,
	}}})
	if res.Outcome != verdict.OutcomeCannotMeasure {
		t.Fatalf("outcome = %s; without the prior state an addition cannot be told from an upgrade", res.Outcome)
	}
	if !strings.Contains(res.Reason, "whole") {
		t.Errorf("reason = %q, should explain why", res.Reason)
	}
}

func TestTruncatedChangeCannotBeMeasured(t *testing.T) {
	in := edit("/w/package.json", `{"dependencies":{}}`, `{"dependencies":{"a":"^1"}}`)
	in.Event.Action.Truncated = true
	if res := run(t, in); res.Outcome != verdict.OutcomeCannotMeasure {
		t.Errorf("outcome = %s; a cut-short change may be missing the line that added something", res.Outcome)
	}
}

// The silence tests. Every one of these is ordinary work.
func TestStaysSilentOnOrdinaryWork(t *testing.T) {
	for _, c := range []struct {
		name string
		in   signal.Input
	}{
		{"a source file", edit("/w/src/app.ts", "const a = 1", "const a = 2")},
		{"a readme", edit("/w/README.md", "# App", "# App\n\nNow with docs.")},
		{"a lockfile is not a manifest", edit("/w/package-lock.json", "{}", `{"packages":{"":{"name":"x"}}}`)},
		{"reformatting a manifest", edit("/w/package.json",
			`{"dependencies":{"react":"^18.0.0"}}`,
			"{\n  \"dependencies\": {\n    \"react\": \"^18.0.0\"\n  }\n}")},
		{"editing a script, not a dependency", edit("/w/package.json",
			`{"scripts":{"build":"tsc"},"dependencies":{"react":"^18.0.0"}}`,
			`{"scripts":{"build":"tsc -p ."},"dependencies":{"react":"^18.0.0"}}`)},
	} {
		t.Run(c.name, func(t *testing.T) {
			res := run(t, c.in)
			if res.Outcome == verdict.OutcomeFinding {
				t.Errorf("fired on ordinary work: %s", res.Verdicts[0].Summary)
			}
		})
	}
}
