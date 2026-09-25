package main

import "testing"

// Every public command answers --help. A new command without an entry would
// run instead of explaining itself, which is how mcp --help came to start a
// server.
func TestEveryCommandHasHelp(t *testing.T) {
	for _, c := range []string{"init", "status", "doctor", "show", "review", "allow", "allowed", "revoke", "traces", "serve", "connect", "account", "disconnect", "sync", "remote", "mcp"} {
		if commandHelp[c] == "" {
			t.Errorf("%s has no --help text", c)
		}
	}
}
