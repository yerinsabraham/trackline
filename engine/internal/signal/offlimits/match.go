package offlimits

import (
	"path/filepath"
	"strings"
)

// Path patterns, with the one extension people expect: `**` crosses directory
// separators. Go's filepath.Match does not, so `secrets/**` would match
// `secrets/a` and miss `secrets/a/b`, which is the opposite of what anyone
// writing that pattern intends.
//
// Matching is attempted against the full path and against the path relative to
// the project root, because a user writes `.env` meaning "the one in my
// project" and the event carries an absolute path.

// Match reports whether path matches pattern.
func Match(pattern, path string) bool {
	path = filepath.ToSlash(path)
	pattern = filepath.ToSlash(pattern)

	if strings.Contains(pattern, "**") {
		return matchDoubleStar(pattern, path)
	}

	// A bare name with no separator matches that file anywhere in the tree,
	// which is what someone writing ".env" means.
	if !strings.Contains(pattern, "/") {
		ok, _ := filepath.Match(pattern, filepath.Base(path))
		return ok
	}

	if ok, _ := filepath.Match(pattern, path); ok {
		return true
	}
	// Also try suffix segments, so "config/secrets.yml" matches
	// "/work/project/config/secrets.yml".
	segs := strings.Split(path, "/")
	for i := range segs {
		if ok, _ := filepath.Match(pattern, strings.Join(segs[i:], "/")); ok {
			return true
		}
	}
	return false
}

func matchDoubleStar(pattern, path string) bool {
	i := strings.Index(pattern, "**")
	prefix, suffix := pattern[:i], pattern[i+2:]
	suffix = strings.TrimPrefix(suffix, "/")

	idx := strings.Index(path, strings.TrimSuffix(prefix, "/"))
	if prefix != "" && idx < 0 {
		return false
	}
	rest := path
	if prefix != "" {
		rest = path[idx+len(strings.TrimSuffix(prefix, "/")):]
		rest = strings.TrimPrefix(rest, "/")
	}
	if suffix == "" {
		// "secrets/**" means everything under it, but not the directory entry
		// itself with nothing after.
		return rest != ""
	}
	for _, seg := range allSuffixes(rest) {
		if ok, _ := filepath.Match(suffix, seg); ok {
			return true
		}
	}
	return false
}

func allSuffixes(p string) []string {
	segs := strings.Split(p, "/")
	out := make([]string, 0, len(segs))
	for i := range segs {
		out = append(out, strings.Join(segs[i:], "/"))
	}
	return out
}
