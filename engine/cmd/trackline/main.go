// Command trackline sets up and inspects the alignment hook.
//
// This is the CLI a person uses. The hook binary itself is separate and is
// never run by hand: it is what the agent invokes, and it exists apart so that
// the thing on the latency-critical path carries nothing it does not need.
//
//	trackline init [--host claude|codex|cursor] [--root DIR]
//	trackline status [--root DIR]
//	trackline doctor
//	trackline mcp [--root DIR]
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/yerinsabraham/trackline/engine/internal/config"
	"github.com/yerinsabraham/trackline/engine/internal/hosts"
	"github.com/yerinsabraham/trackline/engine/internal/install"
	"github.com/yerinsabraham/trackline/engine/internal/mcp"
	"github.com/yerinsabraham/trackline/engine/internal/override"
)

// version is stamped at release by scripts/build-binaries.sh.
var version = "dev"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	// Checked before dispatch, so no command starts working when it was only
	// asked to explain itself. `trackline mcp --help` used to start a server
	// waiting on stdin, and `review --help` went looking for a judge.
	for _, a := range os.Args[2:] {
		// Not the bare word "help": it is a legal target for allow and
		// revoke, and a directory can be called that.
		if a == "-h" || a == "--help" {
			if text, ok := commandHelp[os.Args[1]]; ok {
				fmt.Print(text)
				return
			}
		}
	}

	var err error
	switch os.Args[1] {
	case "init":
		err = cmdInit(os.Args[2:])
	case "status":
		err = cmdStatus(os.Args[2:])
	case "doctor":
		err = cmdDoctor(os.Args[2:])
	case "review":
		err = cmdReview(os.Args[2:])
	case "show":
		err = cmdShow(os.Args[2:])
	case "allow":
		err = cmdAllow(os.Args[2:])
	case "allowed":
		err = cmdAllowed(os.Args[2:])
	case "revoke":
		err = cmdRevoke(os.Args[2:])
	case "traces":
		err = cmdTraces(os.Args[2:])
	case "serve":
		err = cmdServe(os.Args[2:])
	case "connect":
		err = cmdConnect(os.Args[2:])
	case "disconnect":
		err = cmdDisconnect(os.Args[2:])
	case "account":
		err = cmdAccount(os.Args[2:])
	case "mcp":
		f := parse(os.Args[2:])
		// stdout is the protocol, so warnings go to stderr, which clients log.
		if mcp.UnlikelyRoot(f.root) {
			fmt.Fprintf(os.Stderr, "trackline mcp: checking against %s, which is not a project. "+
				"Pass --root /path/to/project in the client's server config.\n", f.root)
		}
		err = (&mcp.Server{Root: f.root, Version: version}).Serve(os.Stdin, os.Stdout)
	case "help", "-h", "--help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `trackline — an alignment layer for AI agents

  init     wire the hook into an agent's configuration
  status   show what the hook has seen, and whether it has ever run
  doctor   check the installation without changing anything
  show     replay a recorded session as a readable story
  review   ask a model whether each turn's work served its request
  allow    approve something a check objected to
  allowed  list what has been approved
  revoke   withdraw an approval
  traces   check exported production traces (OTLP, protobuf or JSON)
  serve    receive traces from a running agent and check them live
  connect  link this machine to a trackline account (optional)
  account  who this machine is connected as
  disconnect  unlink this machine
  mcp      serve check_action and get_rules to any MCP client (the agent
           must choose to ask; a hook does not give it the choice)

Flags: --host claude|codex|cursor   --root DIR
`)
}

var commandHelp = map[string]string{
	"init": `trackline init [--host claude|codex|cursor] [--root DIR]

Wire the hook into an agent's configuration, merging with what is there.
Starts in warn mode: it records, and never interrupts.
`,
	"status": `trackline status [--root DIR]

Whether the hook has ever run here, and what each check has seen.
`,
	"doctor": `trackline doctor [--host claude|codex|cursor] [--root DIR]

Check the install without changing anything: the binary, its behaviour, the
config, the rules files, the wiring, and what trackline can and cannot do in
that agent.
`,
	"show": `trackline show [--root DIR] [--task TEXT] [--json FILE]

Replay a recorded session as a readable story.
`,
	"review": `trackline review [--provider cli|http] [--binary claude|codex|cursor-agent] [--json] [--root DIR]

Ask a model whether each turn's work served its request. Off unless configured,
and never on the hook's path. Use a different model family from the agent that
did the work. --json prints one line per turn.
`,
	"allow": `trackline allow <check> <target> [--project] [--reason TEXT] [--root DIR]

Approve something a check objected to. For the current request only, unless
--project.
`,
	"allowed": `trackline allowed [--root DIR]

List what has been approved.
`,
	"revoke": `trackline revoke <check> [target] [--root DIR]

Withdraw an approval.
`,
	"traces": `trackline traces [--root DIR] [--json] [--judge codex|claude|cursor-agent] FILE|DIR...

Check exported production traces: OTLP export requests, protobuf or .json.
Tool calls are checked against the tool policy in .trackline.json, and the
stream is watched for outages, loops and latency or token drift.

A trace with no message content cannot show which tool was called; that is
reported as not checkable, never as clean.

--judge asks a model whether each run's tools served its request, which is the
only thing that catches a permitted tool used off-task. It is sent the request
and the tool names, never the arguments.
`,
	"serve": `trackline serve [--addr 127.0.0.1:4318] [--root DIR] [--sample 1] [--alert URL] [--out FILE]

Receive traces from a running agent: point an OTLP/HTTP exporter, protobuf or
JSON, at http://ADDR/v1/traces. Each conversation is checked when it completes.

--sample keeps a fraction of conversations, each one whole.
--alert posts findings and incidents to a webhook as {"text": ...}, which
Slack and most chat tools accept directly.
--out appends every result as a JSON line.

Listens on localhost unless told otherwise: traces carry customer messages.
`,
	"connect": `trackline connect [--no-requests] [--root DIR] [--yes] [--no-browser] [--api URL]

Link this machine to a trackline account, then choose whether this project
uploads. Optional: trackline works fully without an account.

Shows a code and a link; approve it on the site while signed in. The
credential is kept in your user config directory, readable only by you, never
inside a project. You are asked whether to share what you ask your agent;
--no-requests says no. File contents and command text are never sent.
`,
	"account": `trackline account

Who this machine is connected as, and which projects upload.
`,
	"disconnect": `trackline disconnect

Revoke this machine's credential on the account and remove it from here.
`,
	"mcp": `trackline mcp [--root DIR]

Serve check_action and get_rules to any MCP client over stdio.

Advisory: the agent chooses whether to ask, so an agent that does not ask is
not watched. Pass --root in the client's server config; some clients start
servers far from the project.
`,
}

type flags struct {
	host string
	root string
}

func parse(args []string) flags {
	f := flags{host: "claude"}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--host", "-host":
			if i+1 < len(args) {
				i++
				f.host = args[i]
			}
		case "--root", "-root":
			if i+1 < len(args) {
				i++
				f.root = args[i]
			}
		}
	}
	if f.root == "" {
		f.root, _ = os.Getwd()
	}
	return f
}

// hookPath finds the hook binary next to this one, which is how it is shipped.
func hookPath() (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", err
	}
	candidate := filepath.Join(filepath.Dir(self), "trackline-hook")
	if _, err := os.Stat(candidate); err == nil {
		return candidate, nil
	}
	// Fall back to PATH, which covers a development checkout.
	if p, err := exec.LookPath("trackline-hook"); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("could not find the trackline-hook binary next to %s or on PATH", self)
}

func cmdInit(args []string) error {
	f := parse(args)

	binary, err := hookPath()
	if err != nil {
		return err
	}

	// Check the binary works before writing it into anyone's configuration.
	// Wiring up something broken is worse than not wiring it at all, because
	// it looks installed.
	fmt.Print("checking the hook binary... ")
	if err := install.SelfTest(binary); err != nil {
		fmt.Println("failed")
		return err
	}
	fmt.Println("ok")

	res, err := install.Install(install.Host(f.host), f.root, binary)
	if err != nil {
		return err
	}

	switch {
	case res.AlreadyPresent:
		fmt.Printf("already installed in %s\n", rel(f.root, res.Path))
	case res.Created:
		fmt.Printf("created %s\n", rel(f.root, res.Path))
	default:
		fmt.Printf("updated %s, leaving your other settings alone\n", rel(f.root, res.Path))
	}

	fmt.Printf("\nmode: warn — it will notice things and write them down, and never interrupt you.\n")
	fmt.Printf("findings go to %s\n", filepath.Join(".trackline", "findings.jsonl"))

	// Said at install, not discovered later. See internal/hosts.
	if caps, ok := hosts.For(f.host); ok {
		fmt.Print("\n" + caps.Describe())
	}

	fmt.Print("\nOne thing left, and it matters: a misconfigured hook does not warn, it\n" +
		"simply never runs. Make one edit with your agent, then:\n\n    trackline status\n\n" +
		"which will tell you whether it actually fired.\n")
	return nil
}

func cmdDoctor(args []string) error {
	f := parse(args)

	binary, err := hookPath()
	if err != nil {
		return err
	}
	fmt.Printf("hook binary: %s\n", binary)

	fmt.Print("behaviour:   ")
	if err := install.SelfTest(binary); err != nil {
		fmt.Println("FAILING")
		return err
	}
	fmt.Println("ok — blocks a protected write and explains why")

	cfg, err := config.Load(f.root)
	if err != nil {
		return err
	}
	fmt.Printf("mode:        %s\n", cfg.Mode)
	fmt.Printf("protected:   %d patterns\n", len(cfg.OffLimits))

	rules, rulesErr := config.LoadRules(f.root, cfg)
	fmt.Printf("rules found: %d, from %v\n", len(rules), cfg.RuleFiles)
	if rulesErr != nil {
		fmt.Printf("rules:       UNREADABLE — %v\n", rulesErr)
	}

	path, err := install.Plan(install.Host(f.host), f.root)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err != nil {
		fmt.Printf("wiring:      not installed for %s (run trackline init)\n", f.host)
	} else {
		fmt.Printf("wiring:      %s exists\n", rel(f.root, path))
	}
	if caps, ok := hosts.For(f.host); ok {
		fmt.Print("\n" + caps.Describe())
	}
	return nil
}

func cmdStatus(args []string) error {
	f := parse(args)

	seen, at, err := install.Observed(f.root)
	if err != nil {
		return err
	}
	if !seen {
		fmt.Print("The hook has never run here.\n\n" +
			"That is the failure worth catching: a hook can be configured correctly\n" +
			"and still never fire, and nothing warns you. Check that init was run for\n" +
			"the agent you are actually using, and on Codex that the hook is trusted.\n")
		return nil
	}

	fmt.Printf("The hook is running. Last seen %s.\n\n", humanAge(time.Since(at)))
	return summarise(filepath.Join(f.root, ".trackline", "findings.jsonl"))
}

func rel(root, path string) string {
	if r, err := filepath.Rel(root, path); err == nil {
		return r
	}
	return path
}

func humanAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%d minutes ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d hours ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%d days ago", int(d.Hours()/24))
	}
}

func cmdAllow(args []string) error {
	// trackline allow <signal> <target> [--project] [--reason "..."]
	var positional []string
	scope := override.ScopeOnce
	reason := ""
	root := ""
	session, turn := "", ""

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--project", "-project":
			scope = override.ScopeProject
		case "--reason", "-reason":
			if i+1 < len(args) {
				i++
				reason = args[i]
			}
		case "--root", "-root":
			if i+1 < len(args) {
				i++
				root = args[i]
			}
		case "--session":
			if i+1 < len(args) {
				i++
				session = args[i]
			}
		case "--turn":
			if i+1 < len(args) {
				i++
				turn = args[i]
			}
		default:
			positional = append(positional, args[i])
		}
	}
	if len(positional) < 2 {
		return fmt.Errorf(`usage: trackline allow <check> <target> [--project] [--reason "..."]`)
	}
	if root == "" {
		root, _ = os.Getwd()
	}

	// A one-off approval needs to know which request it belongs to. The most
	// recent action tells us, which is the one the agent was just stopped on.
	if scope == override.ScopeOnce && (session == "" || turn == "") {
		s, tn, err := lastAction(root)
		if err != nil {
			return fmt.Errorf("could not work out which request this belongs to; pass --project to approve it for good: %w", err)
		}
		session, turn = s, tn
	}

	store := override.NewStore(root)
	g := override.Grant{
		Signal: positional[0], Target: positional[1],
		Scope: scope, Session: session, Turn: turn, Reason: reason,
	}
	if err := store.Add(g); err != nil {
		return err
	}

	fmt.Printf("approved: %s\n", g.Describe())
	if scope == override.ScopeOnce {
		fmt.Println("this covers the current request only; use --project to make it permanent")
	}
	return nil
}

func cmdAllowed(args []string) error {
	f := parse(args)
	grants := override.NewStore(f.root).List()
	if len(grants) == 0 {
		fmt.Println("nothing has been approved in this project.")
		return nil
	}
	fmt.Printf("%d approval(s):\n", len(grants))
	for _, g := range grants {
		fmt.Printf("  %s\n", g.Describe())
	}
	return nil
}

func cmdRevoke(args []string) error {
	var positional []string
	root := ""
	for i := 0; i < len(args); i++ {
		if args[i] == "--root" || args[i] == "-root" {
			if i+1 < len(args) {
				i++
				root = args[i]
			}
			continue
		}
		positional = append(positional, args[i])
	}
	if len(positional) < 1 {
		return fmt.Errorf("usage: trackline revoke <check> [target]")
	}
	if root == "" {
		root, _ = os.Getwd()
	}
	target := ""
	if len(positional) > 1 {
		target = positional[1]
	}

	n, err := override.NewStore(root).Remove(positional[0], target)
	if err != nil {
		return err
	}
	fmt.Printf("removed %d approval(s)\n", n)
	return nil
}

// lastAction reads which request the agent was most recently stopped on, so a
// one-off approval attaches to the right one.
func lastAction(root string) (session, turn string, err error) {
	b, err := os.ReadFile(filepath.Join(root, ".trackline", "turn.json"))
	if err != nil {
		return "", "", err
	}
	var st struct {
		SessionID string `json:"sessionId"`
		TurnID    string `json:"turnId"`
	}
	if err := json.Unmarshal(b, &st); err != nil {
		return "", "", err
	}
	if st.TurnID == "" {
		return "", "", fmt.Errorf("no recent action recorded")
	}
	return st.SessionID, st.TurnID, nil
}
