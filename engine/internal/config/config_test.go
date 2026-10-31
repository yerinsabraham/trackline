package config_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/yerinsabraham/trackline/engine/internal/config"
)

// Under 1% of agent configs declare any permission boundary, so the defaults
// have to stand on their own.
func TestDefaultsAreUsefulWithNoFile(t *testing.T) {
	cfg, err := config.Load(t.TempDir())
	if err != nil {
		t.Fatalf("a missing config file is the normal state, not an error: %v", err)
	}
	if cfg.Mode != config.ModeWarn {
		t.Errorf("default mode = %q; a new install must not interrupt", cfg.Mode)
	}
	if len(cfg.OffLimits) == 0 {
		t.Error("secrets should be protected without anyone configuring it")
	}
	if len(cfg.RuleFiles) == 0 {
		t.Error("the common rule-file conventions should be read by default")
	}
}

// Silently running on defaults when someone believes they configured something
// is how a tool ends up not doing what its owner thinks it does.
func TestMalformedConfigIsAnError(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".trackline.json"), []byte(`{"mode":`), 0o600)

	if _, err := config.Load(dir); err == nil {
		t.Fatal("a malformed config must error rather than fall back silently")
	}
}

// Setting one option must not switch everything else off.
func TestUserFileLaysOverDefaults(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".trackline.json"),
		[]byte(`{"modes":{"off-limits":"auto"},"offLimits":["secrets/**"]}`), 0o600)

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Mode != config.ModeWarn {
		t.Errorf("unset fields must keep their default, got mode %q", cfg.Mode)
	}
	if cfg.ModeFor("off-limits") != config.ModeAuto {
		t.Error("a per-check mode should win over the global one")
	}
	if cfg.ModeFor("anything-else") != config.ModeWarn {
		t.Error("other checks keep the global mode")
	}

	// Adding a protected path must not lose protection on the defaults.
	var hasEnv, hasCustom bool
	for _, p := range cfg.OffLimits {
		if p == ".env" {
			hasEnv = true
		}
		if p == "secrets/**" {
			hasCustom = true
		}
	}
	if !hasEnv || !hasCustom {
		t.Errorf("offLimits = %v; user entries must add to the defaults, not replace them", cfg.OffLimits)
	}
}

func TestInvalidModeFallsBackRatherThanBreaking(t *testing.T) {
	cfg := config.Config{Mode: "nonsense", Modes: map[string]config.Mode{"x": "also-nonsense"}}
	if cfg.ModeFor("x") != config.ModeWarn {
		t.Error("an unknown mode must fall back to the safest one")
	}
}

func TestRulesAreReadFromProjectFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(`# Project

Some ordinary prose describing the layout.

- Never edit secrets.env directly.
- Configuration must live in config.local.json.

`+"```"+`bash
# this is an example, never treat it as a rule
rm -rf build
`+"```"+`

Another paragraph that states nothing in particular.
`), 0o600)

	rules, err := config.LoadRules(dir, config.Default())
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 2 {
		t.Fatalf("got %d rules, want the 2 outside the code fence: %+v", len(rules), rules)
	}
	for _, r := range rules {
		if !strings.HasPrefix(r.Source, "AGENTS.md:") {
			t.Errorf("a rule must say where it came from, got %q", r.Source)
		}
		if strings.Contains(r.Text, "rm -rf") {
			t.Error("fenced code is an example, not a rule")
		}
	}
}

func TestMissingRuleFilesAreSilent(t *testing.T) {
	rules, err := config.LoadRules(t.TempDir(), config.Default())
	if err != nil {
		t.Fatalf("no rule files is a normal state: %v", err)
	}
	if len(rules) != 0 {
		t.Errorf("got %d rules from an empty directory", len(rules))
	}
}

// A rules file that exists but cannot be read must say so, and must not hide
// the files after it. Every caller once dropped this error, so an unreadable
// AGENTS.md looked exactly like a project with no rules.
func TestUnreadableRulesFileIsReportedAndDoesNotHideTheRest(t *testing.T) {
	root := t.TempDir()
	// A directory where a file is expected fails to read for every user,
	// including root, which a permissions trick would not.
	os.Mkdir(filepath.Join(root, "AGENTS.md"), 0o755)
	os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte("- Never touch the billing code.\n"), 0o600)

	rules, err := config.LoadRules(root, config.Default())
	if err == nil || !strings.Contains(err.Error(), "AGENTS.md") {
		t.Errorf("err = %v; an unreadable rules file must be named", err)
	}
	if len(rules) == 0 {
		t.Error("CLAUDE.md read fine and its rules must still be returned")
	}
}

// A remote session blocks on secrets and new dependencies whatever the
// project says, and never loosens anything.
func TestForRemoteOnlyTightens(t *testing.T) {
	c := config.Default()
	c.Mode = config.ModeAsk
	c.Modes = map[string]config.Mode{"off-limits": config.ModeWarn, "scope": config.ModeAuto}
	c.Disabled = []string{"dependency-added", "diff-size"}

	r := c.ForRemote()
	for _, name := range []string{"off-limits", "dependency-added"} {
		if r.ModeFor(name) != config.ModeAuto || r.IsDisabled(name) {
			t.Errorf("%s: mode %s, disabled %v", name, r.ModeFor(name), r.IsDisabled(name))
		}
	}
	if r.ModeFor("scope") != config.ModeAuto || r.ModeFor("repetition") != config.ModeAsk || !r.IsDisabled("diff-size") {
		t.Error("checks outside the remote set must keep what the project chose")
	}
	if c.Modes["off-limits"] != config.ModeWarn || !c.IsDisabled("dependency-added") {
		t.Error("ForRemote changed the configuration it was given")
	}
}

func TestAHandoffCannotTouchWhatTellsAnAgentHowToBehave(t *testing.T) {
	c := config.Config{OffLimits: []string{"secrets/**", "!*.md"}}
	got := c.ForHandoff()
	if slices.Contains(got.OffLimits, "!*.md") {
		t.Error("a project's exception must not free these files")
	}
	for _, want := range []string{"secrets/**", "CLAUDE.md", "AGENTS.md", "AGENTS.override.md", ".mcp.json", ".codex/**", ".claude/**", ".git/hooks/**"} {
		if !slices.Contains(got.OffLimits, want) {
			t.Errorf("%s is not protected", want)
		}
	}
	if len(c.OffLimits) != 2 {
		t.Fatal("the caller's list changed")
	}
}

func TestAgentFilesCatchEveryRouteAndCase(t *testing.T) {
	root := t.TempDir()
	write := func(path, text string) {
		os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0o755)
		os.WriteFile(filepath.Join(root, path), []byte(text), 0o644)
	}
	write("src/app.go", "code")
	write("CLAUDE.md", "be good")
	write("pkg/AGENTS.md", "nested")
	write(".claude/agents/x.md", "an agent")
	write("node_modules/CLAUDE.md", "not ours")
	before := config.AgentFiles(root)

	write("src/app.go", "changed code")
	if got := config.AgentFilesChanged(before, config.AgentFiles(root)); len(got) != 0 {
		t.Fatalf("ordinary work counted: %v", got)
	}
	write("CLAUDE.md", "be bad")
	write("Agents.Override.md", "new, in odd case")
	write(".git/hooks/pre-commit", "new")
	write(".claude/agents/y.md", "new")
	os.Remove(filepath.Join(root, "pkg/AGENTS.md"))
	got := config.AgentFilesChanged(before, config.AgentFiles(root))
	want := []string{".claude/agents/y.md", ".git/hooks/pre-commit", "Agents.Override.md", "CLAUDE.md", "pkg/AGENTS.md"}
	if !slices.Equal(got, want) {
		t.Fatalf("changed = %v", got)
	}
}
