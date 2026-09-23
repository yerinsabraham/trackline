package shell_test

import (
	"strings"
	"testing"

	"github.com/yerinsabraham/trackline/engine/internal/shell"
)

func join(s []string) string { return strings.Join(s, ",") }

func TestRedirectsAreWrites(t *testing.T) {
	cases := []struct{ cmd, want string }{
		{`echo "TOKEN=x" > .env`, ".env"},
		{`echo "TOKEN=x" >> .env`, ".env"},
		{`cat a.txt > b.txt`, "b.txt"},
		{`printf 'x' >config/prod.yml`, "config/prod.yml"},
		{`node build.js > dist/out.js`, "dist/out.js"},
	}
	for _, c := range cases {
		if got := join(shell.Parse(c.cmd).Writes); got != c.want {
			t.Errorf("Parse(%q).Writes = %q, want %q", c.cmd, got, c.want)
		}
	}
}

// The case that made this package necessary: a dependency added by a command,
// invisible to every check.
func TestInstallsAreRecognised(t *testing.T) {
	cases := []struct{ cmd, want string }{
		{`npm install lodash`, "lodash"},
		{`npm i lodash @types/lodash`, "lodash,@types/lodash"},
		{`npm install lodash@^4.17.21`, "lodash"},
		{`yarn add zod`, "zod"},
		{`pnpm add left-pad`, "left-pad"},
		{`pip install requests==2.31.0`, "requests"},
		{`go get github.com/foo/bar`, "github.com/foo/bar"},
		{`cargo add serde`, "serde"},
		{`npm install --save-dev vitest`, "vitest"},
	}
	for _, c := range cases {
		if got := join(shell.Parse(c.cmd).Installs); got != c.want {
			t.Errorf("Parse(%q).Installs = %q, want %q", c.cmd, got, c.want)
		}
	}

	// An install naming nothing reinstalls what is already declared.
	if got := shell.Parse("npm install").Installs; len(got) != 0 {
		t.Errorf("a bare install adds nothing, got %v", got)
	}
}

func TestDeletesAndMoves(t *testing.T) {
	if got := join(shell.Parse("rm -rf dist").Deletes); got != "dist" {
		t.Errorf("rm: %q", got)
	}
	e := shell.Parse("mv old.ts new.ts")
	if join(e.Writes) != "new.ts" || join(e.Deletes) != "old.ts" {
		t.Errorf("mv: writes=%v deletes=%v", e.Writes, e.Deletes)
	}
	if got := join(shell.Parse("cp a.ts b.ts").Writes); got != "b.ts" {
		t.Errorf("cp: %q", got)
	}
}

func TestInPlaceEditsOnly(t *testing.T) {
	if got := shell.Parse(`sed -i '' 's/a/b/' src/app.ts`).Writes; join(got) != "src/app.ts" {
		t.Errorf("sed -i should be a write, got %v", got)
	}
	// Without -i, sed prints and changes nothing.
	if got := shell.Parse(`sed 's/a/b/' src/app.ts`).Writes; len(got) != 0 {
		t.Errorf("sed without -i writes nothing, got %v", got)
	}
}

// Reading changes nothing, and saying so is what keeps "unknown" meaningful.
func TestReadsAreUnderstoodAndHarmless(t *testing.T) {
	for _, cmd := range []string{
		"ls -la", "cat package.json", "grep -r TODO src", "git status",
		"find . -name '*.ts'", "wc -l src/app.ts",
	} {
		e := shell.Parse(cmd)
		if !e.Understood {
			t.Errorf("Parse(%q) not understood; reads should be recognised", cmd)
		}
		if len(e.Writes)+len(e.Deletes)+len(e.Installs) != 0 {
			t.Errorf("Parse(%q) claimed effects: %+v", cmd, e)
		}
	}
}

// Claiming to have understood a command we did not is worse than admitting we
// did not, because an unexamined command would then read as a safe one.
func TestUnknownCommandsAreAdmitted(t *testing.T) {
	for _, cmd := range []string{
		"./scripts/deploy.sh", "make build", "docker compose up",
		"curl -X POST https://example.com", "xargs rm",
	} {
		if shell.Parse(cmd).Understood {
			t.Errorf("Parse(%q) claimed to be understood", cmd)
		}
	}
}

func TestChainedCommands(t *testing.T) {
	e := shell.Parse(`npm install lodash && echo done > log.txt`)
	if join(e.Installs) != "lodash" {
		t.Errorf("installs = %v", e.Installs)
	}
	if join(e.Writes) != "log.txt" {
		t.Errorf("writes = %v", e.Writes)
	}
	if !e.Understood {
		t.Error("both halves were recognised")
	}

	// One unrecognised part makes the whole reading incomplete.
	mixed := shell.Parse(`npm install lodash && ./deploy.sh`)
	if mixed.Understood {
		t.Error("a command with an unrecognised part must not claim to be fully understood")
	}
	if join(mixed.Installs) != "lodash" {
		t.Error("what was recognised should still be reported")
	}
}

func TestQuotedPathsSurvive(t *testing.T) {
	if got := join(shell.Parse(`echo x > "my file.txt"`).Writes); got != "my file.txt" {
		t.Errorf("writes = %q", got)
	}
}

// Commands that only read in some forms. Each of these was once reported as
// understood and touching nothing, which is the one answer a safety check must
// never give about a command that can write anywhere.
func TestCommandsThatCanWriteAreNotReads(t *testing.T) {
	for _, cmd := range []string{
		`node -e "require('fs').writeFileSync('.env','x')"`,
		`node scripts/migrate.js`,
		`python3 -c "open('.env','w').write('x')"`,
		`python manage.py flush`,
		`awk '{print > "out.txt"}' in.txt`,
		`find . -name '*.log' -delete`,
		`find . -name '*.tmp' -exec rm {} +`,
		`git reset --hard`,
		`git checkout -- .`,
		`git clean -fdx`,
		`git -C sub reset --hard HEAD~3`,
		`git branch -D main`,
		`git stash drop`,
		`env FOO=1 ./deploy.sh`,
		`command rm -rf build`,
		`sort -o data.txt data.txt`,
	} {
		if e := shell.Parse(cmd); e.Understood {
			t.Errorf("%s: reported as understood; it can change files and must be unknown", cmd)
		}
	}
}

// The narrowing must not swing the other way. Read-only forms of the same
// commands stay understood, or every session fills with unknowns and the
// signal drowns.
func TestReadOnlyFormsStayUnderstood(t *testing.T) {
	for _, cmd := range []string{
		`node --version`,
		`python3 -V`,
		`find . -name '*.go'`,
		`git status`,
		`git diff --stat`,
		`git -C sub log --oneline`,
		`git branch`,
		`git branch -a`,
		`git remote -v`,
		`git stash list`,
		`sort data.txt`,
		`env`,
		`command -v go`,
	} {
		if e := shell.Parse(cmd); !e.Understood {
			t.Errorf("%s: reported as unknown; it only reads", cmd)
		}
	}
}
