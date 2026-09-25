package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/yerinsabraham/trackline/engine/internal/cloud/account"
	"github.com/yerinsabraham/trackline/engine/internal/cloud/remote"
	"github.com/yerinsabraham/trackline/engine/internal/relay"
)

// Remote jobs: a prompt sent from the site runs on this laptop.
//
// Everything that decides whether a job runs happens here, on the laptop:
// remote is turned on here, per project, passkeys are paired here, and every
// job is checked here against what this laptop trusts. The account relays.

func cmdRemote(args []string) error {
	if len(args) == 0 {
		fmt.Print(commandHelp["remote"])
		return nil
	}
	switch args[0] {
	case "enable":
		return remoteEnable(args[1:])
	case "disable":
		return remoteDisable(args[1:])
	case "status":
		return remoteStatus()
	case "run":
		return remoteRun()
	}
	return fmt.Errorf("unknown: trackline remote %s. Try: enable, disable, status", args[0])
}

func remoteClient() (remote.Client, account.Credentials, error) {
	creds, err := account.LoadCredentials()
	if errors.Is(err, os.ErrNotExist) {
		return remote.Client{}, creds, errors.New("this machine is not connected. Run: trackline connect")
	} else if err != nil {
		return remote.Client{}, creds, err
	}
	return remote.Client{Base: creds.API, Token: creds.Token}, creds, nil
}

// rpFor is the site passkeys must belong to. TRACKLINE_ORIGIN points it at a
// local site for development; it is read once, at the first pairing, and
// kept, so the runner started at login never depends on the environment.
func rpFor(s *relay.State) (relay.RP, error) {
	if s.RP != nil {
		return *s.RP, nil
	}
	o := os.Getenv("TRACKLINE_ORIGIN")
	if o == "" {
		return relay.DefaultRP, nil
	}
	u, err := url.Parse(o)
	if err != nil || u.Host == "" {
		return relay.RP{}, fmt.Errorf("TRACKLINE_ORIGIN is not an origin: %q", o)
	}
	return relay.RP{ID: u.Hostname(), Origin: u.Scheme + "://" + u.Host}, nil
}

func remoteEnable(args []string) error {
	root, pair, start := "", false, true
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--root", "-root":
			if i+1 < len(args) {
				i++
				root = args[i]
			}
		case "--pair":
			pair = true
		case "--no-start":
			start = false
		}
	}
	if root == "" {
		root, _ = os.Getwd()
	}
	root, _ = filepath.Abs(root)

	client, creds, err := remoteClient()
	if err != nil {
		return err
	}
	projRoot, proj, ok := account.ProjectFor(root)
	if !ok {
		return fmt.Errorf("%s is not connected. Run trackline connect there first", root)
	}
	name := filepath.Base(projRoot)

	s, err := relay.Load()
	if err != nil {
		return err
	}
	if len(s.Keys) == 0 || pair {
		if err := pairPasskey(client, creds, s); err != nil {
			return err
		}
	}

	if err := relay.Update(func(s *relay.State) error {
		s.Enable(proj.ID, projRoot, name, time.Now())
		return nil
	}); err != nil {
		return err
	}
	if err := client.SetRemoteProject(proj.ID, name); err != nil {
		fmt.Fprintf(os.Stderr, "Turned on here, but the account could not be told (%v). The runner tells it when it starts.\n", err)
	}
	if start {
		if err := startAtLogin(); err != nil {
			fmt.Fprintf(os.Stderr, "Could not start the runner: %v\nRun it yourself: trackline remote run\n", err)
		}
	}
	fmt.Printf("\nRemote is on for %s. Turn it off: trackline remote disable\n", name)
	return nil
}

// pairPasskey shows a code, waits for the phone to sign it, and trusts the
// passkey only if the signature is over this laptop's code.
func pairPasskey(client remote.Client, creds account.Credentials, s *relay.State) error {
	rp, err := rpFor(s)
	if err != nil {
		return err
	}
	p, err := client.StartPairing()
	if err != nil {
		return fmt.Errorf("could not start pairing: %w", err)
	}
	code, err := relay.NewCode()
	if err != nil {
		return err
	}
	fmt.Printf("\nOn your phone, open %s/remote and enter:\n\n    %s\n\n", rp.Origin, relay.ShowCode(code))
	fmt.Print("Waiting")

	deadline := time.Now().Add(time.Duration(p.ExpiresIn) * time.Second)
	for {
		if time.Now().After(deadline) {
			fmt.Println()
			return errors.New("the code expired. Run trackline remote enable again")
		}
		time.Sleep(pairPoll)
		fmt.Print(".")
		st, err := client.PairingState(p.ID)
		if err != nil {
			fmt.Println()
			return err
		}
		if st.Status == "expired" {
			fmt.Println()
			return errors.New("the code expired. Run trackline remote enable again")
		}
		if st.Status != "answered" || st.Answer == nil {
			continue
		}
		fmt.Println()
		key, err := relay.VerifyPair(rp, creds.DeviceID, code, *st.Answer, time.Now())
		if err != nil {
			client.PairingResult(p.ID, false, err.Error())
			return fmt.Errorf("nothing was paired: %w", err)
		}
		if err := relay.Update(func(s *relay.State) error {
			if s.RP == nil {
				s.RP = &rp
			}
			s.AddKey(key)
			return nil
		}); err != nil {
			return err
		}
		client.PairingResult(p.ID, true, "")
		fmt.Printf("Paired with %s.\n", key.Name)
		return nil
	}
}

func remoteDisable(args []string) error {
	root, all := "", false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--root", "-root":
			if i+1 < len(args) {
				i++
				root = args[i]
			}
		case "--all":
			all = true
		}
	}
	client, _, clientErr := remoteClient()

	if all {
		if err := relay.Forget(); err != nil {
			return err
		}
		stopAtLogin()
		if clientErr == nil {
			client.UnsetAllRemote()
		}
		fmt.Println("Remote is off on this laptop, and its paired passkeys are forgotten.")
		return nil
	}

	if root == "" {
		root, _ = os.Getwd()
	}
	root, _ = filepath.Abs(root)
	if r, _, ok := account.ProjectFor(root); ok {
		root = r
	}
	var id string
	left := 0
	if err := relay.Update(func(s *relay.State) error {
		var ok bool
		if id, _, ok = s.ProjectAt(root); ok {
			delete(s.Projects, id)
		}
		left = len(s.Projects)
		return nil
	}); err != nil {
		return err
	}
	if id == "" {
		fmt.Printf("Remote was not on for %s.\n", filepath.Base(root))
		return nil
	}
	if clientErr == nil {
		client.UnsetRemoteProject(id)
	}
	if left == 0 {
		stopAtLogin()
	}
	fmt.Printf("Remote is off for %s.\n", filepath.Base(root))
	return nil
}

func remoteStatus() error {
	s, err := relay.Load()
	if err != nil {
		return err
	}
	if len(s.Projects) == 0 && len(s.Keys) == 0 {
		fmt.Println("Remote is off. Turn it on inside a project: trackline remote enable")
		return nil
	}
	fmt.Println("Passkeys that can send jobs here:")
	for _, k := range s.Keys {
		fmt.Printf("  %s, paired %s\n", k.Name, k.PairedAt.Local().Format("2 Jan 2006"))
	}
	fmt.Println("\nProjects:")
	if len(s.Projects) == 0 {
		fmt.Println("  none")
	}
	for _, p := range s.Projects {
		last := "no jobs yet"
		if !p.LastJobAt.IsZero() {
			last = "last job " + ago(time.Since(p.LastJobAt))
		}
		fmt.Printf("  %s  (%s)\n", p.Root, last)
	}
	fmt.Println()
	fmt.Println(runnerLine())
	return nil
}

func ago(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%d min ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%d h ago", int(d.Hours()))
	}
	return fmt.Sprintf("%d days ago", int(d.Hours()/24))
}

// runnerStatus is what the runner writes after each wait, for status to read.
type runnerStatus struct {
	PID     int       `json:"pid"`
	Contact time.Time `json:"contact,omitzero"`
	Error   string    `json:"error,omitempty"`
}

func statusPath() string {
	d, _ := account.Dir()
	return filepath.Join(d, "remote.status")
}

func runnerLine() string {
	var st runnerStatus
	b, err := os.ReadFile(statusPath())
	if err != nil || json.Unmarshal(b, &st) != nil || !alive(st.PID) {
		return "Runner: not running. Start it: trackline remote enable"
	}
	switch {
	case st.Error != "":
		return fmt.Sprintf("Runner: running, but cannot reach the account: %s", st.Error)
	case st.Contact.IsZero():
		return "Runner: starting."
	}
	return fmt.Sprintf("Runner: running, in touch with the account %s.", ago(time.Since(st.Contact)))
}

func alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return p.Signal(syscall.Signal(0)) == nil
}

// Waits: how long the server holds each request, and how long to back off
// when the account cannot be reached.
var (
	pollWait   = 25 * time.Second
	maxBackoff = time.Minute
	pairPoll   = 2 * time.Second
)

// remoteRun is the runner: wait for a job, check it, do it, report, repeat.
// launchd starts it at login and again if it crashes; it exits cleanly, and
// is not restarted, when there is nothing left to run for.
func remoteRun() error {
	trimLog()
	client, creds, err := remoteClient()
	if err != nil {
		return err
	}
	s, err := relay.Load()
	if err != nil {
		return err
	}
	if len(s.Projects) == 0 {
		fmt.Println("Remote is not on for any project. Nothing to run.")
		return nil
	}
	// The account may have missed an enable made while offline.
	for id, p := range s.Projects {
		client.SetRemoteProject(id, p.Name)
	}
	logf("runner started for %d project(s)", len(s.Projects))

	backoff := time.Second
	for {
		next, err := client.NextJob(pollWait)
		var apiErr *remote.Error
		if errors.As(err, &apiErr) && apiErr.Status == 401 {
			writeStatus(runnerStatus{PID: os.Getpid()})
			logf("this machine was disconnected from the account; stopping")
			return nil
		}
		if err != nil {
			writeStatus(runnerStatus{PID: os.Getpid(), Error: err.Error()})
			// Jitter, so a laptop waking up does not retry in lockstep with
			// every other one after an outage.
			time.Sleep(backoff/2 + rand.N(backoff/2+1))
			backoff = min(backoff*2, maxBackoff)
			continue
		}
		backoff = time.Second
		writeStatus(runnerStatus{PID: os.Getpid(), Contact: time.Now()})
		if next.Stop {
			logf("stopped from the site; run trackline remote enable on this laptop to start again")
			return nil
		}
		if next.Job != nil {
			handle(client, creds, *next.Job)
		}
		if s, err := relay.Load(); err == nil && len(s.Projects) == 0 {
			logf("remote was turned off for every project; stopping")
			return nil
		}
	}
}

func handle(client remote.Client, creds account.Credentials, d remote.Delivery) {
	var got relay.Accepted
	err := relay.Update(func(s *relay.State) error {
		var err error
		got, err = relay.Check(s, d.Envelope, creds.DeviceID, time.Now())
		return err
	})
	var r *relay.Refusal
	if errors.As(err, &r) {
		logf("refused job %s: %s (%s)", d.ID, r.Reason, r.Code)
		notify("trackline refused a remote job", r.Reason)
		client.JobResult(d.ID, remote.Result{Status: "refused", Code: r.Code, Reason: r.Reason})
		return
	}
	if err != nil {
		logf("job %s: %v", d.ID, err)
		client.JobResult(d.ID, remote.Result{Status: "failed", Reason: "the laptop could not check the job"})
		return
	}

	name := filepath.Base(got.Root)
	logf("accepted %s job %s for %s", got.Job.Kind, d.ID, name)
	notify("trackline: remote job", fmt.Sprintf("A %s job arrived for %s.", got.Job.Kind, name))
	// A test job proves the path from the phone to this folder, and runs
	// nothing.
	out := fmt.Sprintf("Received by %s in %s. Test jobs run nothing.", creds.DeviceName, name)
	if err := client.JobResult(d.ID, remote.Result{Status: "done", Output: out}); err != nil {
		logf("could not report job %s: %v", d.ID, err)
	}
}

func writeStatus(st runnerStatus) {
	b, _ := json.Marshal(st)
	os.WriteFile(statusPath(), b, 0o600)
}

func logf(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "%s %s\n", time.Now().Format(time.RFC3339), fmt.Sprintf(format, a...))
}

// trimLog keeps the runner's log from growing without end. launchd opens it
// for appending, so emptying it underneath is safe.
func trimLog() {
	if fi, err := os.Stat(logPath()); err == nil && fi.Size() > 1<<20 {
		os.Truncate(logPath(), 0)
	}
}

func logPath() string {
	d, _ := account.Dir()
	return filepath.Join(d, "remote.log")
}

// notify puts a notice on the laptop's screen, so a job nobody expected is
// seen when it happens, not later in a log.
func notify(title, body string) {
	if os.Getenv("TRACKLINE_NO_NOTIFY") != "" {
		return
	}
	switch runtime.GOOS {
	case "darwin":
		script := fmt.Sprintf("display notification %s with title %s", strconv.Quote(body), strconv.Quote(title))
		exec.Command("osascript", "-e", script).Run()
	case "linux":
		if p, err := exec.LookPath("notify-send"); err == nil {
			exec.Command(p, title, body).Run()
		}
	}
}

// ── starting at login ──────────────────────────────────────────────────────

const launchLabel = "dev.trackline.remote"

// launchctl is replaced in tests, which must never load an agent into the
// real session.
var launchctl = func(args ...string) error {
	out, err := exec.Command("launchctl", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("launchctl %s: %s", strings.Join(args, " "), strings.TrimSpace(string(out)))
	}
	return nil
}

func plistPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", launchLabel+".plist"), nil
}

// launchPlist starts the runner at login, and again if it crashes, but not
// after it exits cleanly: that is it deciding there is nothing to run for.
func launchPlist(binary, log string, env map[string]string) string {
	var envXML strings.Builder
	if len(env) > 0 {
		envXML.WriteString("  <key>EnvironmentVariables</key>\n  <dict>\n")
		for _, k := range []string{"TRACKLINE_API", "TRACKLINE_CONFIG_DIR"} {
			if v, ok := env[k]; ok {
				fmt.Fprintf(&envXML, "    <key>%s</key><string>%s</string>\n", k, xmlEscape(v))
			}
		}
		envXML.WriteString("  </dict>\n")
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>%s</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
    <string>remote</string>
    <string>run</string>
  </array>
%s  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key>
  <dict><key>SuccessfulExit</key><false/></dict>
  <key>ThrottleInterval</key><integer>10</integer>
  <key>ProcessType</key><string>Background</string>
  <key>StandardOutPath</key><string>%s</string>
  <key>StandardErrorPath</key><string>%s</string>
</dict>
</plist>
`, launchLabel, xmlEscape(binary), envXML.String(), xmlEscape(log), xmlEscape(log))
}

func xmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}

func startAtLogin() error {
	if runtime.GOOS != "darwin" {
		fmt.Println("Starting at login is macOS only for now. Keep this running: trackline remote run")
		return nil
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	if self, err = filepath.EvalSymlinks(self); err != nil {
		return err
	}
	// `go run` builds into a temporary folder that is gone by the next login.
	if strings.Contains(self, string(filepath.Separator)+"go-build") {
		return errors.New("this trackline is a temporary build; install it before turning remote on")
	}
	env := map[string]string{}
	for _, k := range []string{"TRACKLINE_API", "TRACKLINE_CONFIG_DIR"} {
		if v := os.Getenv(k); v != "" {
			env[k] = v
		}
	}
	p, err := plistPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(p, []byte(launchPlist(self, logPath(), env)), 0o644); err != nil {
		return err
	}
	domain := "gui/" + strconv.Itoa(os.Getuid())
	// Restarted, so a new binary or a newly enabled project is picked up.
	launchctl("bootout", domain+"/"+launchLabel)
	return launchctl("bootstrap", domain, p)
}

func stopAtLogin() {
	if runtime.GOOS != "darwin" {
		return
	}
	launchctl("bootout", "gui/"+strconv.Itoa(os.Getuid())+"/"+launchLabel)
	if p, err := plistPath(); err == nil {
		os.Remove(p)
	}
}
