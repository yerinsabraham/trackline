package scope

import (
	"path/filepath"
	"regexp"
	"strings"
)

// Working out what part of the tree a request is about.
//
// The obvious approach is to match words from the request against the path, and
// it does not survive contact with real work. "Add rate limiting" produces
// `src/middleware/throttle.ts`, which shares no word with the request, and
// firing there would be a false alarm on a correct answer. Loose matching makes
// this check noise, and noise is what gets a tool uninstalled.
//
// So this looks only for *explicit* references: something shaped like a path, a
// quoted name, or a word immediately qualified as a directory or module. If the
// request names nothing, the check declines to have an opinion rather than
// inventing one.

var (
	// src/auth/login.ts, auth/, ./config
	pathLike = regexp.MustCompile(`[\w.\-]*(?:/[\w.\-]+)+/?`)
	// `auth`, "payments", 'src/api'
	quoted = regexp.MustCompile("[`\"']([\\w./\\-]+)[`\"']")
	// the auth module, the payments directory, in the api package.
	//
	// The trailing class matters: without it "Edit package.json" matches,
	// because "package" is a qualifier and "Edit" precedes it. RE2 has no
	// lookahead, so the qualifier must be followed by something that is
	// neither a word character nor a dot.
	qualified = regexp.MustCompile(`(?i)\b([\w.\-]+)\s+(?:module|directory|folder|package|service|component)(?:[^.\w]|$)`)
	// a bare filename with a known-ish extension
	filename = regexp.MustCompile(`\b([\w\-]+\.(?:go|ts|tsx|js|jsx|py|rb|rs|java|md|json|yml|yaml|toml|sql|sh))\b`)
	// URLs contain slashes and are not references to the project tree
	url = regexp.MustCompile(`\w+://\S+`)
)

// Container directories that say nothing about scope.
//
// Nearly every file in a project is under src, so "src" matching "src" is not
// evidence that a write was in scope. These are stripped from both sides of the
// comparison, leaving the segments that actually distinguish one area from
// another.
var containers = map[string]bool{
	"src": true, "lib": true, "app": true, "pkg": true, "internal": true,
	"test": true, "tests": true, "spec": true, "__tests__": true,
	"cmd": true, "packages": true, "apps": true, "modules": true,
	"source": true, "code": true, "main": true, "dist": true, "build": true,
}

// Segments returns the parts of a path that carry meaning for scope, lowercased
// and without container directories or file extensions.
func Segments(path string) []string {
	var out []string
	for _, seg := range strings.Split(strings.ToLower(filepath.ToSlash(path)), "/") {
		seg = strings.TrimSpace(seg)
		if seg == "" || containers[seg] {
			continue
		}
		// A test file names the thing it tests: login.test.ts is about login.
		seg = strings.TrimSuffix(seg, filepath.Ext(seg))
		for _, suffix := range []string{".test", ".spec", "_test", "_spec"} {
			seg = strings.TrimSuffix(seg, suffix)
		}
		if seg != "" && !containers[seg] {
			out = append(out, seg)
		}
	}
	return out
}

// Mention is one place a request pointed at.
type Mention struct {
	// Raw is the text as written, for evidence.
	Raw string
	// Parts are the meaningful segments of it, which is what scope is judged
	// on. A mention of "src/auth" yields ["auth"], because "src" distinguishes
	// nothing.
	Parts []string
}

// Mentions extracts the places a request explicitly names.
//
// Deliberately returns nothing for a request that names nowhere. A vague
// instruction has no scope to be outside of, and treating it as if it did is
// how this check would fire on everything.
func Mentions(text string) []Mention {
	seen := map[string]bool{}
	var out []Mention

	add := func(raw string) {
		raw = strings.Trim(strings.TrimSpace(raw), "./`\"'")
		if raw == "" || seen[raw] || isCommonWord(raw) {
			return
		}
		parts := Segments(raw)
		// A mention made entirely of container directories names no particular
		// area. "Look in src" is not a scope.
		if len(parts) == 0 {
			return
		}
		for _, p := range parts {
			if len(p) < 2 || isCommonWord(p) {
				return
			}
		}
		seen[raw] = true
		out = append(out, Mention{Raw: raw, Parts: parts})
	}

	text = url.ReplaceAllString(text, " ")

	for _, m := range pathLike.FindAllString(text, -1) {
		// A bare "and/or" or a URL is not a path reference.
		if strings.Contains(m, "://") {
			continue
		}
		add(m)
	}
	for _, m := range quoted.FindAllStringSubmatch(text, -1) {
		add(m[1])
	}
	for _, m := range qualified.FindAllStringSubmatch(text, -1) {
		add(m[1])
	}
	for _, m := range filename.FindAllStringSubmatch(text, -1) {
		add(m[1])
	}
	return out
}

// Words that turn up next to "module" or "directory" and name nothing.
var commonWords = map[string]bool{
	"the": true, "a": true, "an": true, "this": true, "that": true,
	"each": true, "every": true, "any": true, "some": true, "other": true,
	"same": true, "whole": true, "entire": true, "main": true, "new": true,
	"and": true, "or": true, "it": true, "its": true, "their": true,
}

func isCommonWord(s string) bool { return commonWords[strings.ToLower(s)] }
