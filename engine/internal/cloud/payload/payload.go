// Package payload builds what a connected machine uploads, and nothing else.
//
// The contract is docs/contract/ingest-v1.schema.json. These types are that
// schema in Go, field for field, and Build fills them one field at a time from
// an event and its results. It never marshals the engine's own event: that
// carries the raw hook payload, command text and file contents, and a field
// added to it for some local purpose must not quietly start travelling.
//
// Everything that leaves passes through clean(): paths made relative to the
// project, the home directory replaced, token-shaped strings redacted, and
// every string cut to its limit. Finding summaries need this as much as
// paths do: the off-limits check writes "writing to a protected path:
// /Users/<name>/...", and a username is not the user's to lose.
package payload

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/yerinsabraham/trackline/engine/internal/event"
	"github.com/yerinsabraham/trackline/engine/internal/verdict"
)

// Version is the contract version this package writes.
const Version = 1

// Batch is one upload.
type Batch struct {
	V      int     `json:"v"`
	Events []Event `json:"events"`
}

// Event is one action as it is allowed to leave the machine.
type Event struct {
	ID      string   `json:"id"`
	At      string   `json:"at"`
	Project Project  `json:"project"`
	Host    string   `json:"host"`
	Session string   `json:"session"`
	Turn    string   `json:"turn,omitempty"`
	Request string   `json:"request,omitempty"`
	Action  Action   `json:"action"`
	Results []Result `json:"results"`
	Mode    string   `json:"mode"`
	Blocked bool     `json:"blocked"`
}

// Project names the project without revealing where it is.
type Project struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Action is what kind of thing was done, and to which files. Never its content.
type Action struct {
	Type              string   `json:"type"`
	Tool              string   `json:"tool,omitempty"`
	Paths             []string `json:"paths,omitempty"`
	PathsUnknown      bool     `json:"pathsUnknown,omitempty"`
	Installs          []string `json:"installs,omitempty"`
	CommandUnderstood *bool    `json:"commandUnderstood,omitempty"`
}

// Result is one check's conclusion. Evidence values are not carried: they
// quote the action, and the action's text is exactly what stays home.
type Result struct {
	Check    string    `json:"check"`
	Outcome  string    `json:"outcome"`
	Reason   string    `json:"reason,omitempty"`
	Findings []Finding `json:"findings,omitempty"`
}

// Finding is one thing a check objected to.
type Finding struct {
	Severity   string `json:"severity"`
	Summary    string `json:"summary"`
	Target     string `json:"target,omitempty"`
	Suggestion string `json:"suggestion,omitempty"`
}

// Limits, from the schema. A test holds these to the schema file.
const (
	maxID, maxAt, maxProjectID, maxProjectName  = 64, 40, 64, 100
	maxSession, maxRequest, maxTool             = 128, 2000, 64
	maxPath, maxPaths, maxInstall, maxInstalls  = 512, 50, 214, 50
	maxResults, maxFindings, maxText, maxEvents = 12, 10, 300, 100
)

// Options are what Build needs beyond the event itself.
type Options struct {
	// Root is the project's absolute path. It is used to make paths relative
	// and is never sent.
	Root string
	// ProjectID is random, chosen at connect time.
	ProjectID string
	// ShareRequest is the user's answer at connect time.
	ShareRequest bool
	// Mode and Blocked are what the hook decided.
	Mode    string
	Blocked bool
	// ID identifies this event across retries.
	ID string
	// Home is the user's home directory, replaced wherever it appears.
	// Defaults to os.UserHomeDir.
	Home string
}

var modes = map[string]bool{"warn": true, "ask": true, "auto": true}

var checks = map[string]bool{
	"off-limits": true, "dependency-added": true, "scope": true, "diff-size": true,
	"repetition": true, "tool-policy": true, "config": true, "rules": true,
}

var hosts = map[event.Host]string{
	event.HostClaudeCode: "claude-code",
	event.HostCodex:      "codex",
	event.HostCursor:     "cursor",
}

// Build turns one event and its results into what may be uploaded.
func Build(ev event.Event, results []verdict.Result, o Options) (Event, error) {
	host, ok := hosts[ev.Host]
	if !ok {
		return Event{}, errors.New("this host does not upload: " + string(ev.Host))
	}
	if o.Root == "" || o.ProjectID == "" || o.ID == "" {
		return Event{}, errors.New("root, project id and event id are required")
	}
	if !modes[o.Mode] {
		return Event{}, errors.New("unknown mode: " + o.Mode)
	}
	if o.Home == "" {
		o.Home, _ = os.UserHomeDir()
	}
	c := cleaner{root: filepath.Clean(o.Root), home: o.Home}

	out := Event{
		ID:      cut(o.ID, maxID),
		At:      cut(ev.At.UTC().Format(time.RFC3339Nano), maxAt),
		Project: Project{ID: cut(o.ProjectID, maxProjectID), Name: cut(filepath.Base(c.root), maxProjectName)},
		Host:    host,
		Session: cut(ev.SessionID, maxSession),
		Turn:    cut(ev.TurnID, maxSession),
		Mode:    o.Mode,
		Blocked: o.Blocked,
		Action: Action{
			Type:         string(ev.Action.Type),
			Tool:         cut(ev.Action.ToolName, maxTool),
			PathsUnknown: ev.Action.PathsUnknown,
		},
	}
	if o.ShareRequest {
		out.Request = cut(redact(c.text(ev.Request)), maxRequest)
	}
	for _, p := range ev.Action.Paths {
		if rel, inside := c.rel(p); inside && len(out.Action.Paths) < maxPaths {
			out.Action.Paths = append(out.Action.Paths, cut(rel, maxPath))
		}
	}
	for _, pkg := range ev.Action.Installs {
		if len(out.Action.Installs) < maxInstalls {
			out.Action.Installs = append(out.Action.Installs, cut(redact(pkg), maxInstall))
		}
	}
	if ev.Action.Type == event.ActionRunCommand {
		understood := !ev.Action.PathsUnknown
		out.Action.CommandUnderstood = &understood
	}

	for _, r := range results {
		if len(out.Results) == maxResults {
			break
		}
		// A check this contract does not name is left out, not sent under a
		// name the backend would refuse. A newer CLI must not break uploads
		// to a backend that has not learned its checks yet.
		if !checks[r.Signal] {
			continue
		}
		res := Result{Check: r.Signal, Outcome: string(r.Outcome), Reason: cut(redact(c.text(r.Reason)), maxText)}
		for _, v := range r.Verdicts {
			if len(res.Findings) == maxFindings {
				break
			}
			res.Findings = append(res.Findings, Finding{
				Severity:   string(v.Severity),
				Summary:    cut(redact(c.text(v.Summary)), maxText),
				Target:     cut(redact(c.text(v.Target)), maxPath),
				Suggestion: cut(redact(c.text(v.Suggestion)), maxText),
			})
		}
		out.Results = append(out.Results, res)
	}
	return out, nil
}

type cleaner struct{ root, home string }

// rel makes a path relative to the project, and reports whether it was inside.
func (c cleaner) rel(p string) (string, bool) {
	if !filepath.IsAbs(p) {
		p = filepath.Join(c.root, p)
	}
	r, err := filepath.Rel(c.root, filepath.Clean(p))
	if err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(r), true
}

// text rewrites free text so it carries no absolute location. The project
// root becomes nothing; anything else under the home directory becomes ~.
func (c cleaner) text(s string) string {
	if s == "" {
		return s
	}
	s = strings.ReplaceAll(s, c.root+string(filepath.Separator), "")
	s = strings.ReplaceAll(s, c.root, ".")
	if c.home != "" && c.home != string(filepath.Separator) {
		s = strings.ReplaceAll(s, c.home, "~")
	}
	return s
}

// Token shapes that turn up in what people paste into a request. Replaced
// wherever they appear, request text or not. Broad on purpose: a false
// "[redacted]" costs a word, a missed key costs the key.
var tokenShapes = regexp.MustCompile(strings.Join([]string{
	`gh[pousr]_[A-Za-z0-9]{20,}`,                                 // GitHub tokens
	`github_pat_[A-Za-z0-9_]{20,}`,                               //
	`sk-(?:ant-|proj-)?[A-Za-z0-9_-]{20,}`,                       // OpenAI, Anthropic
	`AKIA[0-9A-Z]{16}`,                                           // AWS access key id
	`xox[abposr]-[A-Za-z0-9-]{10,}`,                              // Slack
	`AIza[0-9A-Za-z_-]{35}`,                                      // Google API key
	`npm_[A-Za-z0-9]{36}`,                                        // npm
	`eyJ[A-Za-z0-9_-]{8,}\.eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]+`, // JWT
	`-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?(?:-----END [A-Z ]*PRIVATE KEY-----|$)`,
}, "|"))

func redact(s string) string { return tokenShapes.ReplaceAllString(s, "[redacted]") }

// cut shortens to at most n bytes without splitting a character.
func cut(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[:n]
	for len(s) > 0 && !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}
