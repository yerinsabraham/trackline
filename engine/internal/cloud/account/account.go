// Package account is what a machine remembers about its trackline account:
// the credential, and which projects upload. Files only; talking to the
// account is package remote.
//
// No network code on purpose. The hook reads this on every tool call, and
// merely linking net/http into it added 3.4ms to every start (measured,
// docs/experiments/08).
//
// Everything is kept in the user's config directory, never inside a project:
// a credential written into a repository is one `git add .` from being
// published. The credential file is readable only by its owner.
package account

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Dir is where trackline keeps account state for this user.
func Dir() (string, error) {
	if d := os.Getenv("TRACKLINE_CONFIG_DIR"); d != "" {
		return d, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "trackline"), nil
}

// Credentials are what `trackline connect` saves.
type Credentials struct {
	API        string `json:"api"`
	Token      string `json:"token"`
	DeviceID   string `json:"deviceId"`
	DeviceName string `json:"deviceName"`
	Email      string `json:"email,omitempty"`
}

// Project is one project this machine uploads for.
type Project struct {
	// ID is random, so the account never learns the project's path.
	ID            string `json:"id"`
	ShareRequests bool   `json:"shareRequests"`
	// ShareReplies is agreed with the same question as ShareRequests, "your
	// messages and your agent's replies". Projects connected before replies
	// existed have it off until connected again.
	ShareReplies bool `json:"shareReplies,omitempty"`
	// ShareCommands sends a command's full text. Off unless asked for, and only
	// from this computer: a command can hold a secret no filter recognises, so
	// the phone cannot be the place that lets it out.
	ShareCommands bool `json:"shareCommands,omitempty"`
	// Agents is the setup last sent to the account, so it is sent again only
	// when it changes.
	Agents []string `json:"agents,omitempty"`
	// CLI and Behind are what was last sent with Agents, for the same reason.
	CLI    string   `json:"cli,omitempty"`
	Behind []string `json:"behind,omitempty"`
}

func write(path string, v any) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	// The directory too: on a shared machine a readable directory lists what
	// is in it.
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func read(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// LoadCredentials returns the saved credentials, or os.ErrNotExist.
func LoadCredentials() (Credentials, error) {
	var c Credentials
	d, err := Dir()
	if err != nil {
		return c, err
	}
	err = read(filepath.Join(d, "credentials.json"), &c)
	return c, err
}

// SaveCredentials writes the credentials, owner-readable only.
func SaveCredentials(c Credentials) error {
	d, err := Dir()
	if err != nil {
		return err
	}
	return write(filepath.Join(d, "credentials.json"), c)
}

// Forget removes the credentials, the project list, and anything still
// waiting to upload: a disconnected machine has nowhere to send it.
func Forget() error {
	d, err := Dir()
	if err != nil {
		return err
	}
	for _, f := range []string{"credentials.json", "projects.json", "sync.backoff", "pending-link.json"} {
		if err := os.Remove(filepath.Join(d, f)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	for _, sub := range []string{"outbox", "rejected"} {
		if err := os.RemoveAll(filepath.Join(d, sub)); err != nil {
			return err
		}
	}
	return nil
}

// Projects returns the connected projects, keyed by absolute path.
func Projects() (map[string]Project, error) {
	out := map[string]Project{}
	d, err := Dir()
	if err != nil {
		return out, err
	}
	if err := read(filepath.Join(d, "projects.json"), &out); err != nil && !errors.Is(err, os.ErrNotExist) {
		return out, err
	}
	return out, nil
}

// ConnectProject records a project, keeping its id if it was connected before.
func ConnectProject(root string, shareRequests bool) (Project, error) {
	ps, err := Projects()
	if err != nil {
		return Project{}, err
	}
	p, ok := ps[root]
	if !ok {
		b := make([]byte, 12)
		if _, err := rand.Read(b); err != nil {
			return Project{}, err
		}
		p.ID = "proj_" + hex.EncodeToString(b)
	}
	p.ShareRequests = shareRequests
	p.ShareReplies = shareRequests
	ps[root] = p
	d, _ := Dir()
	return p, write(filepath.Join(d, "projects.json"), ps)
}

// SetShareCommands records the choice for the project holding root, and returns
// that project's folder.
func SetShareCommands(root string, on bool) (string, error) {
	found, p, ok := ProjectFor(root)
	if !ok {
		return "", errors.New(root + " is not connected. Run trackline connect there first")
	}
	ps, err := Projects()
	if err != nil {
		return "", err
	}
	p.ShareCommands = on
	ps[found] = p
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return found, write(filepath.Join(d, "projects.json"), ps)
}

// PreviousShare is the sharing choice the person already made for their
// other projects, when they all agree, so a new project can offer it rather
// than ask from scratch. ok is false when there is no single answer to reuse.
func PreviousShare(except string) (share, ok bool) {
	ps, err := Projects()
	if err != nil {
		return false, false
	}
	seen := 0
	for root, p := range ps {
		if root == except {
			continue
		}
		if seen > 0 && p.ShareRequests != share {
			return false, false
		}
		share = p.ShareRequests
		seen++
	}
	return share, seen > 0
}

// PendingLink is a browser approval started for this computer and not yet
// finished. An agent's command is usually stopped long before a person gets
// to the browser, so the approval is kept, and the next connect picks it up
// rather than asking the person to approve a code that no longer exists.
type PendingLink struct {
	API        string    `json:"api"`
	DeviceCode string    `json:"deviceCode"`
	UserCode   string    `json:"userCode"`
	URL        string    `json:"url"`
	Interval   int       `json:"interval"`
	ExpiresAt  time.Time `json:"expiresAt"`
}

func pendingPath() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "pending-link.json"), nil
}

// SavePending keeps an approval in progress.
func SavePending(p PendingLink) error {
	path, err := pendingPath()
	if err != nil {
		return err
	}
	return write(path, p)
}

// LoadPending returns the approval in progress for api, if one is still
// valid.
func LoadPending(api string, now time.Time) (PendingLink, bool) {
	path, err := pendingPath()
	if err != nil {
		return PendingLink{}, false
	}
	var p PendingLink
	if read(path, &p) != nil || p.API != api || p.DeviceCode == "" {
		return PendingLink{}, false
	}
	if !now.Before(p.ExpiresAt) {
		os.Remove(path)
		return PendingLink{}, false
	}
	return p, true
}

// ClearPending forgets the approval in progress.
func ClearPending() {
	if path, err := pendingPath(); err == nil {
		os.Remove(path)
	}
}

// SaveProject replaces what is kept for one connected project.
func SaveProject(root string, p Project) error {
	ps, err := Projects()
	if err != nil {
		return err
	}
	if _, ok := ps[root]; !ok {
		return errors.New("not a connected project: " + root)
	}
	ps[root] = p
	d, _ := Dir()
	return write(filepath.Join(d, "projects.json"), ps)
}

// ProjectFor finds the connected project a directory belongs to: the project
// itself or any folder inside it. Agents often run from a subfolder, and its
// actions still belong to the project that was connected. The deepest match
// wins, so a connected folder inside another connected one is its own project.
func ProjectFor(dir string) (root string, p Project, ok bool) {
	ps, err := Projects()
	if err != nil {
		return "", Project{}, false
	}
	dir = filepath.Clean(dir)
	for r, candidate := range ps {
		if (dir == r || strings.HasPrefix(dir, r+string(filepath.Separator))) && len(r) > len(root) {
			root, p, ok = r, candidate, true
		}
	}
	return root, p, ok
}
