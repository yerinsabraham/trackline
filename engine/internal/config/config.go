// Package config decides how trackline behaves, and defaults to something
// useful when nobody has told it anything.
//
// That default matters more than the options. A study of 10,008 public
// repositories found that fewer than 1% of agent configuration files declare
// any permission boundary at all, and 58% have exactly one commit: written
// once, never revised. Anything that needs configuring before it does something
// useful will not be adopted.
//
// So every field here has a working default, and configuration is something a
// user grows into rather than a step they face on install.
package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Mode is how far trackline is allowed to go when a check fires.
type Mode string

const (
	// ModeWarn logs and never interrupts. The default, and where every new
	// check starts: a tool that interrupts wrongly gets muted on the first day
	// and uninstalled on the second.
	ModeWarn Mode = "warn"

	// ModeAsk pauses and waits for a human.
	ModeAsk Mode = "ask"

	// ModeAuto blocks and hands the reason back to the agent, which measured
	// 11 corrections out of 11 in trials across two agents. Still opt-in:
	// being correct in a trial is not the same as being welcome by default.
	ModeAuto Mode = "auto"
)

// Valid reports whether m is a mode trackline knows.
func (m Mode) Valid() bool { return m == ModeWarn || m == ModeAsk || m == ModeAuto }

// Config is the whole of trackline's behaviour.
type Config struct {
	// Mode is the fallback for any check without its own setting.
	Mode Mode `json:"mode,omitempty"`

	// Modes overrides Mode for a named check, so "never touch .env" can be
	// automatic while "this looks out of scope" still asks.
	Modes map[string]Mode `json:"modes,omitempty"`

	// RuleFiles are read for project rules, in order, first found wins per
	// name. Defaults cover the two conventions in common use.
	RuleFiles []string `json:"ruleFiles,omitempty"`

	// OffLimits are paths that must never be written, whatever the task says.
	OffLimits []string `json:"offLimits,omitempty"`

	// Disabled names checks that should not run at all.
	Disabled []string `json:"disabled,omitempty"`

	// Judge configures the one check that asks a model whether work served the
	// request. Off unless configured, and never on the hook's path.
	Judge JudgeConfig `json:"judge,omitempty"`

	// Tools is the policy for a production agent's tool calls. A deployed
	// agent does not write files, it calls tools, and the rule people state
	// about it is of this shape: never do X, or never do X unless Y happened
	// first. Empty means no policy, which the check reports as such.
	Tools ToolPolicy `json:"tools,omitempty"`
}

// ToolPolicy is what a production agent may and may not call.
type ToolPolicy struct {
	// Never lists tools that must not be called at all.
	Never []string `json:"never,omitempty"`
	// RequireApproval maps a tool to the tool that must have run earlier in
	// the same conversation for it to be allowed, e.g.
	// {"fintech_change_limit": "request_human_approval"}.
	RequireApproval map[string]string `json:"requireApproval,omitempty"`
}

// Empty reports whether any policy was configured.
func (p ToolPolicy) Empty() bool { return len(p.Never) == 0 && len(p.RequireApproval) == 0 }

// JudgeConfig says how to reach a model, if at all.
//
// Off by default, and deliberately not a single vendor. A judge should be a
// different model from the one under test, and nobody can follow that rule if
// the tool only speaks to one company. An OpenAI-compatible endpoint covers
// OpenAI, Ollama, vLLM and anything local, so someone unwilling to send their
// code to a third party can point this at their own machine.
type JudgeConfig struct {
	// Provider is "cli", "http", or empty for off.
	Provider string `json:"provider,omitempty"`
	// Binary is the command to run for the cli provider, e.g. "claude".
	Binary string `json:"binary,omitempty"`
	// BaseURL and Model configure the http provider.
	BaseURL string `json:"baseUrl,omitempty"`
	Model   string `json:"model,omitempty"`
	// APIKeyEnv names the environment variable holding the key, so a key is
	// never written into a file that gets committed.
	APIKeyEnv string `json:"apiKeyEnv,omitempty"`
}

// Enabled reports whether a judge has been configured at all.
func (j JudgeConfig) Enabled() bool { return j.Provider != "" }

// Default is a configuration that works with no file present.
func Default() Config {
	return Config{
		Mode:      ModeWarn,
		Modes:     map[string]Mode{},
		RuleFiles: []string{"AGENTS.md", "CLAUDE.md", ".cursorrules"},
		// Secrets are the one category where the default is not a judgement
		// call. Nobody asks an agent to rewrite their credentials file.
		//
		// The "!" entries are exceptions, and they are not optional. Protecting
		// ".env.*" also catches ".env.example", which is a committed template
		// people edit all day. Firing on it would be a false alarm on ordinary
		// work, and a tool that interrupts wrongly gets uninstalled.
		OffLimits: []string{
			".env", ".env.*", "*.pem", "*.key", "id_rsa", ".npmrc", ".netrc",
			"!.env.example", "!.env.sample", "!.env.template", "!.env.defaults",
		},
	}
}

// ModeFor returns the mode a named check should run in.
func (c Config) ModeFor(signal string) Mode {
	if m, ok := c.Modes[signal]; ok && m.Valid() {
		return m
	}
	if c.Mode.Valid() {
		return c.Mode
	}
	return ModeWarn
}

// RemoteEnv is set by `trackline remote run` on every agent it starts, and
// reaches the hook through the agent's environment.
const RemoteEnv = "TRACKLINE_REMOTE_JOB"

// remoteBlocks are the checks that block in a remote session whatever the
// project says: with nobody at the keyboard, writing a secret or pulling in a
// new dependency is not something to find out about afterwards.
var remoteBlocks = []string{"off-limits", "dependency-added"}

// ForRemote is the configuration for a session started from the phone. It
// only ever tightens: an agent that sets RemoteEnv on itself gains nothing,
// and a check switched off for local work is on again here.
func (c Config) ForRemote() Config {
	modes := make(map[string]Mode, len(c.Modes)+len(remoteBlocks))
	for k, v := range c.Modes {
		modes[k] = v
	}
	var disabled []string
	for _, d := range c.Disabled {
		if !slices.Contains(remoteBlocks, d) {
			disabled = append(disabled, d)
		}
	}
	for _, name := range remoteBlocks {
		modes[name] = ModeAuto
	}
	c.Modes, c.Disabled = modes, disabled
	// The agent may not edit what watches it. Guard catches it after the
	// fact whatever the route; this tells the agent at the first attempt.
	var limits []string
	for _, p := range c.OffLimits {
		if strings.HasPrefix(p, "!") && exemptsGuarded(strings.TrimPrefix(p, "!")) {
			continue
		}
		limits = append(limits, p)
	}
	c.OffLimits = append(limits, guarded...)
	return c
}

// Where a remote session reads the fingerprint of its guarded files, and the
// approvals made from the phone. Both are set by the runner on the agent's
// environment, which the agent cannot change for the hook: the host starts
// the hook, not the agent's shell.
const (
	RemoteGuardEnv  = "TRACKLINE_REMOTE_GUARD"
	RemoteGrantsEnv = "TRACKLINE_REMOTE_GRANTS"
)

// guarded are the files that decide how an agent is watched: trackline's
// configuration, and the host settings that wire its hook in. They live in the
// project, which is exactly where a remote agent is allowed to write, so a
// prompt-injected agent could switch its own checks off or wire the hook out
// for the next run (found in the R6 review).
var guarded = []string{
	".trackline.json", ".trackline/config.json",
	".claude/settings.json", ".codex/hooks.json", ".codex/config.toml",
}

// Not guarded, on purpose: .trackline/overrides.json, which a phone session
// does not read (the runner keeps its approvals elsewhere), and
// .claude/settings.local.json, which a phone session does not load. Both
// change during ordinary local work, and guarding them would pause remote
// every time someone clicked "always allow" at the desk.

func exemptsGuarded(pattern string) bool {
	for _, g := range guarded {
		if ok, _ := filepath.Match(pattern, g); ok {
			return true
		}
		if ok, _ := filepath.Match(pattern, filepath.Base(g)); ok {
			return true
		}
	}
	return false
}

// Guard fingerprints the guarded files in root, present or not. The runner
// records it when remote is enabled, refuses a job when it has changed since,
// and the hook refuses every action in a remote session once it no longer
// matches what the job started with.
func Guard(root string) string {
	h := sha256.New()
	for _, name := range guarded {
		h.Write([]byte(name))
		h.Write([]byte{0})
		b, err := os.ReadFile(filepath.Join(root, name))
		switch {
		case err == nil:
			h.Write([]byte{1})
			h.Write(b)
		case errors.Is(err, fs.ErrNotExist):
			h.Write([]byte{0})
		default:
			// Unreadable is not the same as absent, and must not match it.
			h.Write([]byte{2})
			h.Write([]byte(err.Error()))
		}
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// IsDisabled reports whether a check has been switched off.
func (c Config) IsDisabled(signal string) bool {
	for _, d := range c.Disabled {
		if d == signal {
			return true
		}
	}
	return false
}

// Filenames searched for configuration, in order.
var Filenames = []string{".trackline.json", ".trackline/config.json"}

// Load reads configuration from root, falling back to defaults.
//
// A missing file is not an error: it is the normal state and the reason
// Default exists. A malformed file *is* an error, because silently running on
// defaults when someone believes they configured something is how a tool ends
// up not doing what its owner thinks it does.
func Load(root string) (Config, error) {
	cfg := Default()

	for _, name := range Filenames {
		path := filepath.Join(root, name)
		b, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return cfg, err
		}

		var file Config
		if err := json.Unmarshal(b, &file); err != nil {
			return cfg, &ParseError{Path: path, Err: err}
		}
		return cfg.merge(file), nil
	}
	return cfg, nil
}

// merge lays a user's file over the defaults. Absent fields keep their default
// rather than becoming empty, so a file that sets one option does not silently
// switch everything else off.
func (c Config) merge(f Config) Config {
	if f.Mode.Valid() {
		c.Mode = f.Mode
	}
	for k, v := range f.Modes {
		if v.Valid() {
			c.Modes[k] = v
		}
	}
	if len(f.RuleFiles) > 0 {
		c.RuleFiles = f.RuleFiles
	}
	if f.Judge.Provider != "" {
		c.Judge = f.Judge
	}
	// Off-limits adds to the defaults rather than replacing them. Someone
	// protecting one more path should not lose protection on their keys.
	c.OffLimits = append(c.OffLimits, f.OffLimits...)
	c.Disabled = append(c.Disabled, f.Disabled...)
	if !f.Tools.Empty() {
		c.Tools = f.Tools
	}
	return c
}

// ParseError says which file failed and why.
type ParseError struct {
	Path string
	Err  error
}

func (e *ParseError) Error() string {
	return "config " + e.Path + " could not be read: " + e.Err.Error()
}

func (e *ParseError) Unwrap() error { return e.Err }
