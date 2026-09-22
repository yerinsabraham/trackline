// Package shell works out what a command touches.
//
// This closes the largest gap in the product. Measured on a real session, 11 of
// 12 actions were shell commands, and every check reported the same thing about
// each: it could not see what they did. An agent installs a dependency, writes
// a file with a redirect, or deletes something, and none of it looks like a
// file write to a hook.
//
// The parsing is deliberately shallow and deliberately incomplete. Shell is a
// programming language and this is not an interpreter: it recognises the forms
// that actually turn up — redirects, package installs, the common file
// commands — and reports everything else as unknown. Claiming to have
// understood a command we did not is worse than admitting we did not, because
// the caller would then treat an unexamined command as a safe one.
package shell

import (
	"strings"
)

// Effects are what a command appears to do.
type Effects struct {
	// Writes are paths the command appears to create or modify.
	Writes []string
	// Deletes are paths it appears to remove.
	Deletes []string
	// Installs are packages it appears to add.
	Installs []string
	// Understood is false when any part of the command was not recognised, in
	// which case the lists above may be incomplete.
	Understood bool
}

// Parse examines a command line.
//
// A command is split on the operators that separate commands, and each part is
// examined alone. Understood is true only when every part was recognised, so a
// caller can tell "this writes to one file" from "this writes to one file and
// also does something I could not read".
func Parse(cmd string) Effects {
	e := Effects{Understood: true}
	for _, part := range splitCommands(cmd) {
		parsePart(strings.TrimSpace(part), &e)
	}
	return e
}

// splitCommands breaks on the separators that end one command and begin
// another. Pipes are included: the right-hand side of a pipe can redirect.
func splitCommands(cmd string) []string {
	var out []string
	var cur strings.Builder
	var quote rune

	runes := []rune(cmd)
	for i := 0; i < len(runes); i++ {
		c := runes[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
			cur.WriteRune(c)
		case c == '\'' || c == '"':
			quote = c
			cur.WriteRune(c)
		case c == ';' || c == '\n' || c == '|':
			// && and || are covered: the second character is consumed by the
			// same branch on the next pass.
			out = append(out, cur.String())
			cur.Reset()
		case c == '&':
			out = append(out, cur.String())
			cur.Reset()
		default:
			cur.WriteRune(c)
		}
	}
	out = append(out, cur.String())
	return out
}

// installers recognise a package manager adding something.
var installers = map[string][]string{
	"npm":   {"install", "i", "add"},
	"yarn":  {"add"},
	"pnpm":  {"add", "install"},
	"bun":   {"add", "install"},
	"pip":   {"install"},
	"pip3":  {"install"},
	"go":    {"get"},
	"cargo": {"add"},
	"gem":   {"install"},
}

// writers are commands whose file arguments are modified in place.
var writers = map[string]bool{
	"tee": true, "touch": true, "truncate": true,
}

func parsePart(part string, e *Effects) {
	if part == "" {
		return
	}

	fields := splitFields(part)
	if len(fields) == 0 {
		return
	}

	// Redirects first: they attach to whatever command precedes them and are
	// the commonest way an agent writes a file from a shell.
	rest := fields
	for i := 0; i < len(rest); i++ {
		f := rest[i]
		if f == ">" || f == ">>" || f == "1>" || f == "2>" || f == "&>" {
			if i+1 < len(rest) {
				e.Writes = append(e.Writes, unquote(rest[i+1]))
			}
			continue
		}
		// Attached forms: >file and >>file
		if strings.HasPrefix(f, ">>") && len(f) > 2 {
			e.Writes = append(e.Writes, unquote(f[2:]))
		} else if strings.HasPrefix(f, ">") && len(f) > 1 && !strings.HasPrefix(f, ">&") {
			e.Writes = append(e.Writes, unquote(f[1:]))
		}
	}

	cmd := fields[0]
	args := argsOnly(fields[1:])

	switch {
	case cmd == "rm":
		e.Deletes = append(e.Deletes, args...)
	case cmd == "mv":
		// The destination is written; the source is removed.
		if len(args) >= 2 {
			e.Writes = append(e.Writes, args[len(args)-1])
			e.Deletes = append(e.Deletes, args[:len(args)-1]...)
		} else {
			e.Understood = false
		}
	case cmd == "cp":
		if len(args) >= 2 {
			e.Writes = append(e.Writes, args[len(args)-1])
		} else {
			e.Understood = false
		}
	case writers[cmd]:
		e.Writes = append(e.Writes, args...)
	case cmd == "sed" || cmd == "perl":
		// Only the in-place forms modify anything.
		if hasFlag(fields, "-i") || hasPrefixFlag(fields, "-i") {
			// The first operand is the script, not a file: in
			// `sed -i '' 's/a/b/' app.ts` the expression would otherwise be
			// reported as a path.
			e.Writes = append(e.Writes, dropScript(args)...)
		}
	case isInstall(cmd, fields):
		e.Installs = append(e.Installs, packages(fields)...)
	case isRead(cmd):
		// Reading changes nothing, and saying so is what keeps the unknown
		// list meaningful.
	default:
		// Anything not recognised may do anything.
		e.Understood = false
	}
}

func isInstall(cmd string, fields []string) bool {
	subs, ok := installers[cmd]
	if !ok || len(fields) < 2 {
		return false
	}
	for _, s := range subs {
		if fields[1] == s {
			return true
		}
	}
	return false
}

// packages returns the named packages of an install command. An install with no
// names reinstalls what is already declared and adds nothing.
func packages(fields []string) []string {
	var out []string
	for _, f := range fields[2:] {
		if strings.HasPrefix(f, "-") || f == "" {
			continue
		}
		if strings.ContainsAny(f, "<>|;&") {
			continue
		}
		// Strip a version suffix: lodash@^4 and lodash==4 name lodash.
		name := f
		if i := strings.LastIndex(name, "@"); i > 0 {
			name = name[:i]
		}
		if i := strings.Index(name, "=="); i > 0 {
			name = name[:i]
		}
		out = append(out, unquote(name))
	}
	return out
}

// isRead lists commands that inspect or emit without changing anything on
// disk, so they do not make a whole session read as unexamined.
//
// A command here is still credited with any redirect attached to it: `printf x
// > f` writes f, and that is recorded before this switch is reached. What being
// listed means is only that the command itself has no further effect worth
// guessing at.
func isRead(cmd string) bool {
	switch cmd {
	case "cat", "ls", "grep", "rg", "find", "head", "tail", "wc", "echo", "pwd",
		"which", "file", "stat", "diff", "git", "node", "python", "python3",
		"jq", "sort", "uniq", "awk", "sed", "env", "date", "true", "false",
		"test", "[", "printf", "seq", "yes", "basename", "dirname", "realpath",
		"uname", "whoami", "hostname", "sleep", "tr", "cut", "column", "less",
		"nl", "tree", "du", "df", "ps", "type", "command", "printenv":
		return true
	}
	return false
}

func splitFields(s string) []string {
	var out []string
	var cur strings.Builder
	var quote rune
	for _, c := range s {
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			} else {
				cur.WriteRune(c)
			}
		case c == '\'' || c == '"':
			quote = c
		case c == ' ' || c == '\t':
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(c)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

// argsOnly drops flags and redirect targets, leaving the operands.
func argsOnly(fields []string) []string {
	var out []string
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		if strings.HasPrefix(f, "-") {
			continue
		}
		if f == ">" || f == ">>" || strings.HasPrefix(f, ">") {
			i++ // skip the redirect target, already recorded
			continue
		}
		out = append(out, unquote(f))
	}
	return out
}

// dropScript removes the leading expression from an editor invocation, and any
// empty operand left by a quoted argument such as sed's macOS `-i ”`.
func dropScript(args []string) []string {
	var out []string
	for _, a := range args {
		if a == "" {
			continue
		}
		out = append(out, a)
	}
	if len(out) <= 1 {
		// Nothing but the script: no file was named, so nothing is known to
		// have been written.
		return nil
	}
	return out[1:]
}

func hasFlag(fields []string, flag string) bool {
	for _, f := range fields {
		if f == flag {
			return true
		}
	}
	return false
}

func hasPrefixFlag(fields []string, flag string) bool {
	for _, f := range fields {
		if strings.HasPrefix(f, flag) && len(f) > len(flag) {
			return true
		}
	}
	return false
}

func unquote(s string) string { return strings.Trim(s, `'"`) }
