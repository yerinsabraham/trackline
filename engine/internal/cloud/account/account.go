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
	for _, f := range []string{"credentials.json", "projects.json", "sync.backoff"} {
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
	ps[root] = p
	d, _ := Dir()
	return p, write(filepath.Join(d, "projects.json"), ps)
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
