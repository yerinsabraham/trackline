package config_test

import (
	"os"
	"path/filepath"
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
