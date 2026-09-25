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

// cmdConnect asks about the project first, then links this machine to an
// account if it is not linked yet. The questions come before the browser so
// that approving there is the last step, not the first of two.
//
//	trackline connect [--api URL] [--root DIR] [--no-requests] [--yes] [--no-browser]
func cmdConnect(args []string) error {
	var apiFlag, root string
	noRequests, yes, noBrowser := false, false, false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--api":
			if i+1 < len(args) {
				i++
				apiFlag = args[i]
			}
		case "--root", "-root":
			if i+1 < len(args) {
				i++
				root = args[i]
			}
		case "--no-requests":
			noRequests = true
		case "--yes", "-y":
			yes = true
		case "--no-browser":
			noBrowser = true
		}
	}
	if root == "" {
		root, _ = os.Getwd()
	}
	root, _ = filepath.Abs(root)
	api := remote.Base(apiFlag)
	ask := prompter(yes)

	var who string
	creds, err := account.LoadCredentials()
	// A machine already connected stays with the account it is connected to,
	// unless told otherwise. Falling back to the default instead sent a second
	// project's connect to a different server.
	if err == nil && apiFlag == "" && os.Getenv("TRACKLINE_API") == "" {
		api = creds.API
	}
	if err == nil && creds.API == api {
		if me, err := (remote.Client{Base: api, Token: creds.Token}).Me(); err == nil {
			who = display(me.User.Name, me.User.Email)
		}
	}

	name := filepath.Base(root)
	if !ask(fmt.Sprintf("Show %s's findings on your dashboard?", name), true) {
		fmt.Println("Nothing connected.")
		return nil
	}
	share := false
	if !noRequests {
		share = ask("Include your messages and your agent's replies?", true)
	}

	if who == "" {
		if who, err = linkMachine(api, noBrowser); err != nil {
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
	return nil
}

// linkMachine runs the browser approval and saves the machine's credential.
// It returns who the machine is now connected as.
func linkMachine(api string, noBrowser bool) (string, error) {
	host, _ := os.Hostname()
	c := remote.Client{Base: api}
	code, err := c.StartConnect(host)
	if err != nil {
		return "", err
	}
	fmt.Printf("\nApprove this computer in your browser (code %s):\n  %s\n", code.UserCode, code.VerificationURIComplete)
	if !noBrowser {
		openBrowser(code.VerificationURIComplete)
	}
	fmt.Print("Waiting for approval")

	interval := time.Duration(code.Interval) * time.Second
	if interval < time.Second {
		interval = 5 * time.Second
	}
	expired := errors.New("the code expired before it was approved. Run trackline connect again")
	deadline := time.Now().Add(time.Duration(code.ExpiresIn) * time.Second)
	var got remote.Poll
	for {
		if time.Now().After(deadline) {
			fmt.Println()
			return "", expired
		}
		time.Sleep(interval)
		fmt.Print(".")
		got, err = c.Poll(code.DeviceCode)
		if err != nil {
			fmt.Println()
			return "", err
		}
		if got.Status == "approved" {
			break
		}
		if got.Status == "denied" {
			fmt.Println()
			return "", errors.New("the connection was declined on the site")
		}
		if got.Status == "expired" {
			fmt.Println()
			return "", expired
		}
	}
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
	fi, err := os.Stdin.Stat()
	interactive := err == nil && fi.Mode()&os.ModeCharDevice != 0
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
// reported, such as Codex before its hook is trusted. It sends only what
// changed since last time unless forced, and a failure waits for the next
// sync: setup is a courtesy, never a reason to hold back events.
func sendSetup(c remote.Client, force bool) {
	ps, err := account.Projects()
	if err != nil {
		return
	}
	var batch []payload.Setup
	var roots []string
	for root, p := range ps {
		agents := install.Wired(root)
		if agents == nil {
			agents = []string{}
		}
		if !force && strings.Join(agents, ",") == strings.Join(p.Agents, ",") {
			continue
		}
		batch = append(batch, payload.Setup{ID: p.ID, Name: filepath.Base(root), Agents: agents})
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
		p.Agents = batch[i].Agents
		account.SaveProject(root, p)
	}
}
