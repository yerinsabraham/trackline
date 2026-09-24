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
)

// cmdConnect links this machine to a trackline account, then asks about the
// project it was run in.
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
	api := account.API(apiFlag)
	ask := prompter(yes)

	creds, err := account.LoadCredentials()
	connected := false
	if err == nil && creds.API == api {
		if me, err := (account.Client{Base: api, Token: creds.Token}).Me(); err == nil {
			fmt.Printf("This machine is connected as %s (%s).\n", display(me.User.Name, me.User.Email), me.Device.Name)
			connected = true
		}
	}

	if !connected {
		host, _ := os.Hostname()
		c := account.Client{Base: api}
		code, err := c.StartConnect(host)
		if err != nil {
			return err
		}
		fmt.Printf("\nTo connect this machine, open\n\n    %s\n\nand check the code on the page is  %s\n\n", code.VerificationURIComplete, code.UserCode)
		if !noBrowser {
			openBrowser(code.VerificationURIComplete)
		}
		fmt.Print("Waiting for you to approve it")

		interval := time.Duration(code.Interval) * time.Second
		if interval < time.Second {
			interval = 5 * time.Second
		}
		deadline := time.Now().Add(time.Duration(code.ExpiresIn) * time.Second)
		var got account.Poll
		for {
			if time.Now().After(deadline) {
				fmt.Println()
				return errors.New("the code expired before it was approved. Run trackline connect again")
			}
			time.Sleep(interval)
			fmt.Print(".")
			got, err = c.Poll(code.DeviceCode)
			if err != nil {
				fmt.Println()
				return err
			}
			if got.Status == "approved" {
				break
			}
			if got.Status == "denied" {
				fmt.Println()
				return errors.New("the connection was declined on the site")
			}
			if got.Status == "expired" {
				fmt.Println()
				return errors.New("the code expired before it was approved. Run trackline connect again")
			}
		}
		fmt.Println(" approved.")

		creds = account.Credentials{API: api, Token: got.Token, DeviceID: got.Device.ID, DeviceName: got.Device.Name}
		if me, err := (account.Client{Base: api, Token: got.Token}).Me(); err == nil {
			creds.Email = me.User.Email
			fmt.Printf("Connected as %s.\n", display(me.User.Name, me.User.Email))
		}
		if err := account.SaveCredentials(creds); err != nil {
			return fmt.Errorf("connected, but the credential could not be saved: %w", err)
		}
	}

	// The project. Stated plainly before asking: paths are sent and they can
	// say more than they seem to.
	name := filepath.Base(root)
	fmt.Printf("\nProject: %s\n", name)
	fmt.Println("If you connect it, trackline sends what it notices as your agent works: which")
	fmt.Println("files were touched (their paths in this project), what each check found, and")
	fmt.Println("packages installed. Never file contents, never command text.")
	if !ask("Connect this project?", true) {
		fmt.Println("Not connected. Run trackline connect here again whenever you like.")
		return nil
	}
	share := false
	if !noRequests {
		share = ask("Share what you ask your agent, so the dashboard can show it?", true)
	}
	if _, err := account.ConnectProject(root, share); err != nil {
		return err
	}
	if share {
		fmt.Printf("%s is connected, and what you ask your agent will be shared.\n", name)
		fmt.Println("To stop sharing it: trackline connect --no-requests")
	} else {
		fmt.Printf("%s is connected. What you ask your agent will not be shared.\n", name)
	}
	fmt.Println("\nUploading starts in a later release; nothing is sent yet.")
	return nil
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
	if err := (account.Client{Base: creds.API, Token: creds.Token}).Disconnect(); err != nil {
		var apiErr *account.Error
		// Already revoked from the site is not a failure: the goal is reached.
		if !errors.As(err, &apiErr) || apiErr.Status != 401 {
			revoked = false
			fmt.Fprintf(os.Stderr, "Could not reach the account to revoke this machine: %v\n", err)
		}
	}
	if err := account.Forget(); err != nil {
		return err
	}
	if revoked {
		fmt.Println("Disconnected. The credential is revoked and removed from this machine.")
	} else {
		fmt.Println("Removed from this machine. Revoke it from the account page too, to be sure.")
	}
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
	me, err := (account.Client{Base: creds.API, Token: creds.Token}).Me()
	if err != nil {
		var apiErr *account.Error
		if errors.As(err, &apiErr) && apiErr.Status == 401 {
			fmt.Println("This machine was disconnected from the account. Run trackline connect to link it again.")
			return nil
		}
		return err
	}
	fmt.Printf("Connected as %s, as %q.\n", display(me.User.Name, me.User.Email), me.Device.Name)
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
