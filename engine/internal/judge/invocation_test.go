package judge

import (
	"os"
	"slices"
	"testing"
)

// `codex -p` is a config profile, not a prompt. Passing -p to every binary
// meant the documented `--binary codex` never worked.
func TestEachCLIIsAskedItsOwnWay(t *testing.T) {
	args, last, err := invocation("claude", "q")
	if err != nil || !slices.Equal(args, []string{"-p", "q"}) || last != "" {
		t.Errorf("claude: %v %q %v", args, last, err)
	}

	args, last, err = invocation("/usr/local/bin/codex", "q")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(last)
	if args[0] != "exec" || slices.Contains(args, "-p") {
		t.Errorf("codex must use exec, never -p: %v", args)
	}
	if !slices.Contains(args, "read-only") {
		t.Errorf("a judge must not be able to write: %v", args)
	}
	if last == "" || args[len(args)-1] != "q" {
		t.Errorf("codex must write its answer to a file and take the prompt last: %v %q", args, last)
	}

	args, _, err = invocation("cursor-agent", "q")
	if err != nil || !slices.Contains(args, "plan") {
		t.Errorf("cursor-agent must run in plan mode, which does not edit: %v %v", args, err)
	}

	if _, _, err := invocation("some-other-agent", "q"); err == nil {
		t.Error("an unknown binary must be refused, not guessed at")
	}
}
