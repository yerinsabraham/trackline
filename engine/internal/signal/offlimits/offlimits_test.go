package offlimits_test

import (
	"strings"
	"testing"

	"github.com/yerinsabraham/trackline/engine/internal/config"
	"github.com/yerinsabraham/trackline/engine/internal/event"
	"github.com/yerinsabraham/trackline/engine/internal/signal"
	"github.com/yerinsabraham/trackline/engine/internal/signal/offlimits"
	"github.com/yerinsabraham/trackline/engine/internal/verdict"
)

func write(paths ...string) signal.Input {
	return signal.Input{Event: event.Event{
		Action: event.Action{Type: event.ActionWriteFile, ToolName: "Write", Paths: paths},
	}}
}

func run(t *testing.T, in signal.Input) verdict.Result {
	t.Helper()
	res := offlimits.New(config.Default().OffLimits).Check(in)
	if err := res.Validate(); err != nil {
		t.Fatalf("the check produced an invalid result: %v", err)
	}
	return res
}

func TestFiresOnProtectedPaths(t *testing.T) {
	for _, p := range []string{
		"/work/project/.env",
		"/work/project/.env.production",
		"/work/project/certs/server.pem",
		"/work/project/deploy/id_rsa",
		"/work/project/.npmrc",
		"/home/dev/.netrc",
		"/work/project/config/service.key",
	} {
		t.Run(p, func(t *testing.T) {
			res := run(t, write(p))
			if res.Outcome != verdict.OutcomeFinding {
				t.Fatalf("outcome = %s, want a finding for %s", res.Outcome, p)
			}
			v := res.Verdicts[0]
			if len(v.Evidence) != 2 {
				t.Errorf("a finding must name the file and the pattern, got %+v", v.Evidence)
			}
			if v.Suggestion == "" {
				t.Error("a block should say what to do instead; Phase 0 measured 11 of 11 corrections when it did")
			}
		})
	}
}

// The tests that decide whether this tool survives contact with real work.
// Every one of these is a file someone edits legitimately, all day.
func TestStaysSilentOnOrdinaryFiles(t *testing.T) {
	for _, p := range []string{
		"/work/project/src/app.ts",
		"/work/project/README.md",
		"/work/project/.env.example",        // the committed template, not the secrets
		"/work/project/docs/environment.md", // contains "environment", not ".env"
		"/work/project/src/keyboard.ts",     // contains "key", not a key file
		"/work/project/package.json",
		"/work/project/.github/workflows/ci.yml",
		"/work/project/test/fixtures/pemberton.txt", // starts with "pem"
	} {
		t.Run(p, func(t *testing.T) {
			if res := run(t, write(p)); res.Outcome != verdict.OutcomeClean {
				t.Errorf("outcome = %s (%s); this is an ordinary file and firing on it is a false alarm",
					res.Outcome, res.Reason)
			}
		})
	}
}

// Reading a protected file is not writing to it. An agent that reads .env to
// understand configuration has done nothing wrong, and firing there would make
// the check unusable.
func TestReadsAndCommandsAreNotWrites(t *testing.T) {
	for _, tc := range []struct {
		name string
		a    event.Action
	}{
		{"read", event.Action{Type: event.ActionReadFile, Paths: []string{"/work/.env"}}},
		{"tool call", event.Action{Type: event.ActionCallTool, ToolName: "search", PathsUnknown: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := run(t, signal.Input{Event: event.Event{Action: tc.a}})
			if res.Outcome != verdict.OutcomeNotApplicable {
				t.Errorf("outcome = %s, want not-applicable", res.Outcome)
			}
		})
	}
}

// The bug this project keeps finding in itself: a check that could not see
// anything must not report clean.
func TestUnseeableActionCannotBeMeasured(t *testing.T) {
	res := run(t, signal.Input{Event: event.Event{Action: event.Action{
		Type: event.ActionWriteFile, ToolName: "Bash",
		Command: "echo TOKEN=x >> .env", PathsUnknown: true,
	}}})

	if res.Outcome != verdict.OutcomeCannotMeasure {
		t.Fatalf("outcome = %s; a shell write could well hit a protected path and must not read as clean", res.Outcome)
	}
	if !strings.Contains(res.Reason, "Bash") {
		t.Errorf("reason should name the tool, got %q", res.Reason)
	}
}

func TestNoPatternsConfiguredIsNotApplicable(t *testing.T) {
	res := offlimits.New(nil).Check(write("/work/.env"))
	if res.Outcome != verdict.OutcomeNotApplicable {
		t.Errorf("outcome = %s, want not-applicable when nothing is protected", res.Outcome)
	}
}

func TestOneVerdictPerOffendingPath(t *testing.T) {
	res := run(t, write("/work/.env", "/work/src/ok.ts", "/work/deploy/id_rsa"))
	if res.Outcome != verdict.OutcomeFinding {
		t.Fatalf("outcome = %s", res.Outcome)
	}
	if len(res.Verdicts) != 2 {
		t.Errorf("got %d verdicts, want one per protected path and none for the ordinary file", len(res.Verdicts))
	}
}

func TestPatternMatching(t *testing.T) {
	cases := []struct {
		pattern, path string
		want          bool
	}{
		{".env", "/work/project/.env", true},
		{".env", "/work/project/src/.env", true},    // a bare name matches anywhere
		{".env", "/work/project/.env.local", false}, // exact name, not a prefix
		{".env.*", "/work/project/.env.local", true},
		{"*.pem", "/a/b/c/server.pem", true},
		{"*.pem", "/a/b/pemberton.txt", false},
		{"secrets/**", "/work/secrets/a.txt", true},
		{"secrets/**", "/work/secrets/nested/deep/a.txt", true}, // ** crosses separators
		{"secrets/**", "/work/secrets", false},
		{"config/prod.yml", "/work/project/config/prod.yml", true},
		{"config/prod.yml", "/work/project/config/dev.yml", false},
		{"**/*.key", "/work/a/b/service.key", true},
	}
	for _, c := range cases {
		if got := offlimits.Match(c.pattern, c.path); got != c.want {
			t.Errorf("Match(%q, %q) = %v, want %v", c.pattern, c.path, got, c.want)
		}
	}
}

// Protecting ".env.*" also catches ".env.example", a committed template people
// edit all day. Exceptions are what make the protective pattern usable.
func TestExceptionsBeatProtection(t *testing.T) {
	for _, p := range []string{
		"/work/project/.env.example",
		"/work/project/.env.sample",
		"/work/project/.env.template",
	} {
		if res := run(t, write(p)); res.Outcome != verdict.OutcomeClean {
			t.Errorf("%s: outcome = %s; a committed template is not a secret", p, res.Outcome)
		}
	}
	// The exception must not punch a hole in the real protection.
	if res := run(t, write("/work/project/.env.production")); res.Outcome != verdict.OutcomeFinding {
		t.Errorf(".env.production: outcome = %s, want a finding", res.Outcome)
	}
}

func TestUserExceptionsWork(t *testing.T) {
	s := offlimits.New([]string{"secrets/**", "!secrets/README.md"})
	in := write("/work/secrets/README.md")
	if res := s.Check(in); res.Outcome != verdict.OutcomeClean {
		t.Errorf("outcome = %s; a user exception must be honoured", res.Outcome)
	}
	if res := s.Check(write("/work/secrets/prod.key")); res.Outcome != verdict.OutcomeFinding {
		t.Errorf("outcome = %s; the exception must not disable the pattern", res.Outcome)
	}
}

// `echo TOKEN=x >> .env` is a write by any reasonable reading, even though the
// action type is "run command". Waving it past as not-applicable would be a
// false clean on the exact thing this check exists for.
func TestShellCommandsReachTheCheck(t *testing.T) {
	res := run(t, signal.Input{Event: event.Event{Action: event.Action{
		Type: event.ActionRunCommand, ToolName: "Bash",
		Command: "echo TOKEN=x >> .env", PathsUnknown: true,
	}}})
	if res.Outcome != verdict.OutcomeCannotMeasure {
		t.Errorf("outcome = %s, want cannot-measure: a shell command may write to a protected path", res.Outcome)
	}
}

// Reads and tool calls genuinely cannot modify a file, so they are not
// applicable rather than unchecked. Reporting them as unchecked would bury the
// real coverage gaps in noise.
func TestNonWritingActionsAreNotApplicableNotUnmeasured(t *testing.T) {
	for _, a := range []event.Action{
		{Type: event.ActionReadFile, Paths: []string{"/work/.env"}},
		{Type: event.ActionCallTool, ToolName: "search", PathsUnknown: true},
	} {
		res := run(t, signal.Input{Event: event.Event{Action: a}})
		if res.Outcome != verdict.OutcomeNotApplicable {
			t.Errorf("%s: outcome = %s, want not-applicable", a.Type, res.Outcome)
		}
	}
}
