package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/yerinsabraham/trackline/engine/internal/cloud/account"
	"github.com/yerinsabraham/trackline/engine/internal/cloud/outbox"
	"github.com/yerinsabraham/trackline/engine/internal/cloud/payload"
	"github.com/yerinsabraham/trackline/engine/internal/cloud/remote"
	"github.com/yerinsabraham/trackline/engine/internal/install"
	"github.com/yerinsabraham/trackline/engine/internal/relay"
)

// connectOptions are connect's answers given up front, by flag or by init.
type connectOptions struct {
	api        string
	noRequests bool
	share      bool
	yes        bool
	noBrowser  bool
	// asked is set by init, which has just asked whether to show the project
	// on the dashboard, so the same question is not put twice.
	asked    bool
	noRemote bool
}

// cmdConnect asks about the project first, then links this machine to an
// account if it is not linked yet. The questions come before the browser so
// that approving there is the last step, not the first of two.
//
//	trackline connect [--api URL] [--root DIR] [--share | --no-requests] [--yes] [--no-browser]
func cmdConnect(args []string) error {
	var o connectOptions
	var root string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--api":
			if i+1 < len(args) {
				i++
				o.api = args[i]
			}
		case "--root", "-root":
			if i+1 < len(args) {
				i++
				root = args[i]
			}
		case "--no-requests":
			o.noRequests = true
		case "--share":
			o.share = true
		case "--yes", "-y":
			o.yes = true
		case "--no-browser":
			o.noBrowser = true
		case "--no-remote":
			o.noRemote = true
		}
	}
	if root == "" {
		root, _ = os.Getwd()
	}
	return connect(root, o)
}

func connect(root string, o connectOptions) error {
	if remoteJob() {
		return errors.New("a task from your phone cannot connect a project")
	}
	root, _ = filepath.Abs(root)
	api := remote.Base(o.api)
	ask := prompter(o.yes)

	var who string
	creds, err := account.LoadCredentials()
	// A machine already connected stays with the account it is connected to,
	// unless told otherwise. Falling back to the default instead sent a second
	// project's connect to a different server.
	if err == nil && o.api == "" && os.Getenv("TRACKLINE_API") == "" {
		api = creds.API
	}
	if err == nil && creds.API == api {
		if me, err := (remote.Client{Base: api, Token: creds.Token}).Me(); err == nil {
			who = display(me.User.Name, me.User.Email)
		}
	}

	name := filepath.Base(root)
	if !o.asked && !ask(fmt.Sprintf("Show %s's findings on your dashboard?", name), true) {
		fmt.Println("Nothing connected.")
		return nil
	}
	share, err := shareChoice(root, o.noRequests, o.share, o.yes, ask)
	if err != nil {
		return err
	}

	if who == "" {
		who, err = linkMachine(api, o.noBrowser, interactive())
		if errors.Is(err, errApprovalPending) {
			// Not a failure: the person has a link to approve, and running
			// connect again, with the same answers, finishes the job.
			return nil
		}
		if err != nil {
			return err
		}
	}
	if _, err := account.ConnectProject(root, share); err != nil {
		return err
	}
	if creds, err := account.LoadCredentials(); err == nil {
		sendSetup(remote.Client{Base: creds.API, Token: creds.Token}, true)
	}

	fmt.Printf("\n%s is connected to %s.\n", name, who)
	connectRemote(root, name, o, ask)
	return nil
}

// connectRemote makes the phone able to reach what was just connected. A
// laptop already set up needs nothing more. Otherwise it is done here, with a
// person at the terminal: a code has to pass from this screen to their phone,
// and without a terminal that is left for later. Never a reason for connect
// itself to fail.
func connectRemote(root, name string, o connectOptions, ask func(string, bool) bool) {
	if o.noRemote {
		// Said once, it holds, or the runner brings it in at its next start.
		if _, p, ok := account.ProjectFor(root); ok {
			relay.Update(func(s *relay.State) error { s.TurnOff(p.ID, root); return nil })
		}
		return
	}
	client, creds, err := remoteClient()
	if err != nil {
		return
	}
	s, err := relay.Load()
	if err != nil {
		return
	}
	if !s.All || len(s.Keys) == 0 {
		if o.yes || !interactive() {
			fmt.Println("To reply to your agents from your phone, run this once in a terminal: trackline remote enable")
			return
		}
		if !ask("Reply to your agents from your phone, in every project on this laptop?", true) {
			return
		}
		if len(s.Keys) == 0 {
			if err := pairPasskey(client, creds, s); err != nil {
				fmt.Fprintf(os.Stderr, "Not paired: %v\nConnected anyway. To pair later: trackline remote enable\n", err)
				return
			}
		}
	}
	if _, _, err := enrollAll(client, true); err != nil {
		fmt.Fprintf(os.Stderr, "Could not turn on replies from your phone: %v\n", err)
		return
	}
	if !runnerRunning() {
		if err := startAtLogin(); err != nil {
			fmt.Fprintf(os.Stderr, "Could not start the runner: %v\nRun it yourself: trackline remote run\n", err)
			return
		}
	}
	fmt.Printf("You can reply to %s from your phone. Every message needs your passkey or send password.\n", name)
}

// shareChoice settles whether a project's messages go with its findings.
//
// That is the person's call, never an agent's. Run by an agent there is no
// terminal, and the old default of yes meant an agent following the install
// guide opted the person in without asking. So without a terminal it shares
// only on an explicit --share; at a terminal the choice already made for the
// person's other projects is the default offered, rather than the same
// question from scratch every time.
func shareChoice(root string, no, yesShare, assumeDefaults bool, ask func(string, bool) bool) (bool, error) {
	if no && yesShare {
		return false, errors.New("--share and --no-requests contradict each other")
	}
	if no || yesShare {
		return yesShare, nil
	}
	prev, known := account.PreviousShare(root)
	if !assumeDefaults && interactive() {
		q := "Include your messages and your agent's replies?"
		if known {
			q = fmt.Sprintf("Include your messages and your agent's replies? (%s for your other projects)", map[bool]string{true: "yes", false: "no"}[prev])
			return ask(q, prev), nil
		}
		return ask(q, true), nil
	}
	// Consent to upload messages is given per project. Without a terminal an
	// earlier no is safe to repeat; an earlier yes is not, or connecting a
	// work project would start uploading because a personal one does.
	switch {
	case known && !prev:
		fmt.Println("Your messages and your agent's replies are not included, as for your other projects.")
	case known && prev:
		fmt.Println("Your messages and your agent's replies are not included. Your other projects include them; to match: trackline connect --share")
	default:
		fmt.Println("Your messages and your agent's replies are not included. To include them: trackline connect --share")
	}
	return false, nil
}

// interactive is whether a person can answer at this terminal. Asked of the
// terminal driver, not read off the file type: /dev/null is a character
// device too, and it is what an agent's commands often get as input, so
// every question was once answered with its default, sharing included.
var interactive = func() bool { return isTerminal(os.Stdin.Fd()) }

// errApprovalPending means the person has a link to approve, and the next
// connect will pick it up.
var errApprovalPending = errors.New("approval pending")

// linkMachine runs the browser approval and saves the machine's credential.
// It returns who the machine is now connected as.
//
// wait is false when nobody is at a terminal: an agent's command is usually
// stopped after a couple of minutes, and a person reading the link from the
// agent takes longer. So it prints the link and code, keeps the approval, and
// returns errApprovalPending; the next connect, once approved, finishes.
func linkMachine(api string, noBrowser, wait bool) (string, error) {
	c := remote.Client{Base: api}
	p, resumed := account.LoadPending(api, time.Now())
	if !resumed {
		host, _ := os.Hostname()
		code, err := c.StartConnect(host)
		if err != nil {
			return "", err
		}
		p = account.PendingLink{
			API: api, DeviceCode: code.DeviceCode, UserCode: code.UserCode, URL: code.VerificationURIComplete,
			Interval: code.Interval, ExpiresAt: time.Now().Add(time.Duration(code.ExpiresIn) * time.Second),
		}
		if err := account.SavePending(p); err != nil {
			return "", err
		}
	}

	expired := errors.New("the code expired before it was approved. Run trackline connect again")
	check := func() (remote.Poll, bool, error) {
		got, err := c.Poll(p.DeviceCode)
		if err != nil {
			return got, false, err
		}
		switch got.Status {
		case "approved":
			return got, true, nil
		case "denied":
			account.ClearPending()
			return got, false, errors.New("the connection was declined on the site")
		case "expired":
			account.ClearPending()
			return got, false, expired
		}
		return got, false, nil
	}

	var got remote.Poll
	if resumed {
		var done bool
		var err error
		if got, done, err = check(); err != nil {
			return "", err
		}
		if done {
			fmt.Print("This computer was approved.")
			goto approved
		}
	}

	fmt.Printf("\nApprove this computer in your browser (code %s):\n  %s\n", p.UserCode, p.URL)
	if !noBrowser && !resumed {
		openBrowser(p.URL)
	}
	if !wait {
		fmt.Println("\nOnce it is approved, run the same trackline connect again to finish. The code is good until " + p.ExpiresAt.Local().Format("15:04") + ".")
		return "", errApprovalPending
	}
	fmt.Print("Waiting for approval")
	{
		interval := time.Duration(p.Interval) * time.Second
		if interval < time.Second {
			interval = 5 * time.Second
		}
		for {
			if time.Now().After(p.ExpiresAt) {
				fmt.Println()
				account.ClearPending()
				return "", expired
			}
			time.Sleep(interval)
			fmt.Print(".")
			var done bool
			var err error
			if got, done, err = check(); err != nil {
				fmt.Println()
				return "", err
			}
			if done {
				break
			}
		}
	}
approved:
	account.ClearPending()
	fmt.Println(" approved.")

	creds := account.Credentials{API: api, Token: got.Token, DeviceID: got.Device.ID, DeviceName: got.Device.Name}
	who := "your account"
	if me, err := (remote.Client{Base: api, Token: got.Token}).Me(); err == nil {
		creds.Email = me.User.Email
		who = display(me.User.Name, me.User.Email)
	}
	if err := account.SaveCredentials(creds); err != nil {
		return "", fmt.Errorf("approved, but the credential could not be saved: %w", err)
	}
	return who, nil
}

// cmdDisconnect revokes this machine on the account and forgets it locally.
func cmdDisconnect(args []string) error {
	creds, err := account.LoadCredentials()
	if errors.Is(err, os.ErrNotExist) {
		fmt.Println("This machine is not connected.")
		return nil
	} else if err != nil {
		return err
	}
	revoked := true
	if err := (remote.Client{Base: creds.API, Token: creds.Token}).Disconnect(); err != nil {
		var apiErr *remote.Error
		// Already revoked from the site is not a failure: the goal is reached.
		if !errors.As(err, &apiErr) || apiErr.Status != 401 {
			revoked = false
			fmt.Fprintf(os.Stderr, "Could not reach the account to revoke this machine: %v\n", err)
		}
	}
	if err := account.Forget(); err != nil {
		return err
	}
	// Remote jobs were for this machine's identity, which is now gone.
	stopAtLogin()
	if err := relay.Forget(); err != nil {
		return err
	}
	if revoked {
		fmt.Println("Disconnected. The credential is revoked and removed from this machine.")
	} else {
		fmt.Println("Removed from this machine. Revoke it from the account page too, to be sure.")
	}
	return nil
}

// cmdSync sends what the outbox holds. Started by the hook with --quiet, so it
// says nothing unless asked by a person.
func cmdSync(args []string) error {
	quiet := false
	for _, a := range args {
		if a == "--quiet" || a == "-q" {
			quiet = true
		}
	}
	say := func(format string, a ...any) {
		if !quiet {
			fmt.Printf(format, a...)
		}
	}
	creds, err := account.LoadCredentials()
	if errors.Is(err, os.ErrNotExist) {
		say("This machine is not connected.\n")
		return nil
	} else if err != nil {
		return err
	}
	dir, err := account.Dir()
	if err != nil {
		return err
	}
	// Started by a hook, an agent is mid-task and more calls are coming. A
	// moment's wait sends them as one batch instead of a request per call.
	var wait time.Duration
	if quiet {
		wait = 2 * time.Second
	}
	client := remote.Client{Base: creds.API, Token: creds.Token}
	sendSetup(client, false)
	res, err := outbox.Box{Dir: dir}.DrainAfter(client, wait)
	if res.Revoked {
		// The account no longer knows this machine. Keeping its credential
		// would only queue more for nobody.
		account.Forget()
		say("This machine was disconnected from the account. Run trackline connect to link it again.\n")
		return nil
	}
	if res.Rejected > 0 {
		say("%d refused by the server and set aside in %s.\n", res.Rejected, filepath.Join(dir, "rejected"))
	}
	if err != nil {
		if quiet {
			return nil
		}
		return fmt.Errorf("sent %d, the rest wait and will be retried: %w", res.Sent, err)
	}
	say("Sent %d.\n", res.Sent)
	return nil
}

// cmdAccount says who this machine is connected as, and which projects upload.
func cmdAccount(args []string) error {
	creds, err := account.LoadCredentials()
	if errors.Is(err, os.ErrNotExist) {
		fmt.Println("This machine is not connected. Run: trackline connect")
		return nil
	} else if err != nil {
		return err
	}
	me, err := (remote.Client{Base: creds.API, Token: creds.Token}).Me()
	if err != nil {
		var apiErr *remote.Error
		if errors.As(err, &apiErr) && apiErr.Status == 401 {
			fmt.Println("This machine was disconnected from the account. Run trackline connect to link it again.")
			return nil
		}
		return err
	}
	fmt.Printf("Connected as %s, as %q.\n", display(me.User.Name, me.User.Email), me.Device.Name)
	if dir, err := account.Dir(); err == nil {
		box := outbox.Box{Dir: dir}
		pending, rejected := box.Counts()
		if pending > 0 {
			line := fmt.Sprintf("%d waiting to send", pending)
			if until, ok := box.WaitingUntil(); ok {
				line += fmt.Sprintf(", next try at %s", until.Local().Format("15:04"))
			}
			fmt.Println(line + ".")
		}
		if rejected > 0 {
			fmt.Printf("%d refused by the server, kept in %s.\n", rejected, filepath.Join(dir, "rejected"))
		}
	}
	ps, _ := account.Projects()
	if len(ps) == 0 {
		fmt.Println("No projects connected. Run trackline connect inside one.")
		return nil
	}
	fmt.Println("\nProjects:")
	for path, p := range ps {
		share := "requests not shared"
		if p.ShareRequests {
			share = "requests shared"
		}
		fmt.Printf("  %s  (%s)\n", path, share)
	}
	return nil
}

func display(name, email string) string {
	switch {
	case name != "" && email != "":
		return fmt.Sprintf("%s <%s>", name, email)
	case email != "":
		return email
	case name != "":
		return name
	}
	return "your account"
}

// prompter asks yes/no questions when there is a person to ask. Without a
// terminal, or with --yes, it takes the default, which for both questions is
// yes; --no-requests is how a script says no to sharing.
func prompter(yes bool) func(q string, def bool) bool {
	interactive := interactive()
	in := bufio.NewReader(os.Stdin)
	return func(q string, def bool) bool {
		hint := "[Y/n]"
		if !def {
			hint = "[y/N]"
		}
		if yes || !interactive {
			fmt.Printf("%s %s %s\n", q, hint, map[bool]string{true: "yes", false: "no"}[def])
			return def
		}
		fmt.Printf("%s %s ", q, hint)
		line, err := in.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return def
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "":
			return def
		case "y", "yes":
			return true
		default:
			return false
		}
	}
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

// sendSetup tells the account which agents are wired into each connected
// project, so the dashboard can show an agent that is set up but has never
// reported, such as Codex before its hook is trusted, and the release here and
// any agent whose hooks are older than it, so an update that needs a step in
// the project can say so on the phone. It sends only what
// changed since last time unless forced, and a failure waits for the next
// sync: setup is a courtesy, never a reason to hold back events.
func sendSetup(c remote.Client, force bool) {
	ps, err := account.Projects()
	if err != nil {
		return
	}
	hook, _ := hookPath()
	var batch []payload.Setup
	var roots []string
	for root, p := range ps {
		agents := install.Wired(root)
		if agents == nil {
			agents = []string{}
		}
		behind := install.Behind(root, hook)
		if !force && strings.Join(agents, ",") == strings.Join(p.Agents, ",") &&
			strings.Join(behind, ",") == strings.Join(p.Behind, ",") && p.CLI == version {
			continue
		}
		batch = append(batch, payload.Setup{ID: p.ID, Name: filepath.Base(root), Agents: agents, CLI: version, Behind: behind})
		roots = append(roots, root)
	}
	if len(batch) == 0 {
		return
	}
	if c.Ingest(payload.Batch{V: payload.Version, Events: []payload.Event{}, Projects: batch}) != nil {
		return
	}
	for i, root := range roots {
		p := ps[root]
		p.Agents, p.CLI, p.Behind = batch[i].Agents, batch[i].CLI, batch[i].Behind
		account.SaveProject(root, p)
	}
}
