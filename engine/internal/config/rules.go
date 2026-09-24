package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yerinsabraham/trackline/engine/internal/signal"
)

// Reading project rules out of AGENTS.md or CLAUDE.md.
//
// These files are prose, not policy, so extraction is a heuristic and is kept
// deliberately dumb: pull out the lines that read like constraints, record
// where each came from, and let the checks decide what to do with them. Phase 1
// does not classify severity or scope — baking that in here would put policy in
// the foundation, and it is the checks that will be measured on false alarms.
//
// The markers are the words people actually use when writing a rule for an
// agent. This will both over- and under-collect, and tuning it belongs to the
// phase that can measure the consequences.
var ruleMarkers = []string{
	"never", "always", "must not", "must ", "do not", "don't",
	"avoid", "forbidden", "off limits", "off-limits", "required",
	"only ever", "under no circumstances",
}

// maxRuleLine guards against a wall of prose being treated as one rule.
const maxRuleLine = 400

// LoadRules reads every configured rule file under root.
//
// A missing file is normal and silent. Rules are returned in file order so a
// verdict citing one can be traced back by eye.
//
// A file that exists but cannot be read is not silent. Every caller dropped
// this error once, so a CLAUDE.md with the wrong permissions read exactly like
// a project with no rules at all. The files that did read are still returned,
// and the error names each one that did not: one bad file must not hide the
// rest.
func LoadRules(root string, cfg Config) ([]signal.Rule, error) {
	var out []signal.Rule
	var failed []error

	for _, name := range cfg.RuleFiles {
		path := filepath.Join(root, name)
		f, err := os.Open(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			failed = append(failed, fmt.Errorf("%s: %w", name, err))
			continue
		}
		rules, err := parseRules(f, name)
		f.Close()
		if err != nil {
			failed = append(failed, fmt.Errorf("%s: %w", name, err))
			continue
		}
		out = append(out, rules...)
	}
	return out, errors.Join(failed...)
}

func parseRules(r interface{ Read([]byte) (int, error) }, source string) ([]signal.Rule, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)

	var out []signal.Rule
	line := 0
	inCode := false

	for sc.Scan() {
		line++
		raw := sc.Text()
		trimmed := strings.TrimSpace(raw)

		// Fenced code is examples, not rules. Collecting "rm -rf" from a
		// snippet as a constraint would be noise of exactly the kind that gets
		// a tool uninstalled.
		if strings.HasPrefix(trimmed, "```") {
			inCode = !inCode
			continue
		}
		if inCode || trimmed == "" {
			continue
		}

		text := strings.TrimLeft(trimmed, "-*+> ")
		text = strings.TrimSpace(strings.TrimLeft(text, "0123456789."))
		if text == "" || !looksLikeRule(text) {
			continue
		}

		truncated := false
		if len(text) > maxRuleLine {
			text = text[:maxRuleLine]
			truncated = true
		}

		out = append(out, signal.Rule{
			ID:        fmt.Sprintf("%s:%d", source, line),
			Text:      text,
			Source:    fmt.Sprintf("%s:%d", source, line),
			Truncated: truncated,
		})
	}
	return out, sc.Err()
}

func looksLikeRule(s string) bool {
	l := strings.ToLower(s)
	for _, m := range ruleMarkers {
		if strings.Contains(l, m) {
			return true
		}
	}
	return false
}
