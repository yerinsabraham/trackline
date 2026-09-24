// Package account links a machine to a trackline account, and remembers which
// projects it uploads for.
//
// Everything is kept in the user's config directory, never inside a project:
// a credential written into a repository is one `git add .` from being
// published. The credential file is readable only by its owner.
package account

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DefaultAPI is where accounts live unless TRACKLINE_API says otherwise.
const DefaultAPI = "https://api.creovine.com/trackline/v1"

// API returns the API base: the flag if given, then TRACKLINE_API, then the default.
func API(flag string) string {
	for _, v := range []string{flag, os.Getenv("TRACKLINE_API"), DefaultAPI} {
		if v != "" {
			return strings.TrimRight(v, "/")
		}
	}
	return DefaultAPI
}

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

// Forget removes the credentials and the project list.
func Forget() error {
	d, err := Dir()
	if err != nil {
		return err
	}
	for _, f := range []string{"credentials.json", "projects.json"} {
		if err := os.Remove(filepath.Join(d, f)); err != nil && !errors.Is(err, os.ErrNotExist) {
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

// ── the API ─────────────────────────────────────────────────────────────────

// Client talks to the trackline API.
type Client struct {
	Base  string
	Token string
	HTTP  *http.Client
}

// Error is an API refusal, with the message the API gave.
type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string { return e.Message }

func (c Client) do(method, path string, body, out any) error {
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.Base+path, r)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("content-type", "application/json")
	}
	if c.Token != "" {
		req.Header.Set("authorization", "Bearer "+c.Token)
	}
	h := c.HTTP
	if h == nil {
		h = &http.Client{Timeout: 20 * time.Second}
	}
	res, err := h.Do(req)
	if err != nil {
		return fmt.Errorf("could not reach %s: %w", c.Base, err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode >= 400 {
		var e struct {
			Error string `json:"error"`
		}
		json.Unmarshal(b, &e)
		if e.Error == "" {
			e.Error = res.Status
		}
		return &Error{Status: res.StatusCode, Message: e.Error}
	}
	if out != nil {
		return json.Unmarshal(b, out)
	}
	return nil
}

// Code is what the API returns when a connect starts.
type Code struct {
	DeviceCode              string `json:"deviceCode"`
	UserCode                string `json:"userCode"`
	VerificationURIComplete string `json:"verificationUriComplete"`
	ExpiresIn               int    `json:"expiresIn"`
	Interval                int    `json:"interval"`
}

// Poll is one answer to "has it been approved yet".
type Poll struct {
	Status string `json:"status"`
	Token  string `json:"token"`
	Device struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"device"`
}

// Me is who a device is connected as.
type Me struct {
	Device struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"device"`
	User struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"user"`
}

func (c Client) StartConnect(name string) (Code, error) {
	var out Code
	return out, c.do("POST", "/devices/code", map[string]string{"name": name}, &out)
}

func (c Client) Poll(deviceCode string) (Poll, error) {
	var out Poll
	return out, c.do("POST", "/devices/token", map[string]string{"deviceCode": deviceCode}, &out)
}

func (c Client) Me() (Me, error) {
	var out Me
	return out, c.do("GET", "/devices/me", nil, &out)
}

func (c Client) Disconnect() error { return c.do("DELETE", "/devices/me", nil, nil) }
