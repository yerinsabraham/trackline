package relay

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/yerinsabraham/trackline/engine/internal/cloud/account"
)

// State is what this laptop trusts for remote jobs. It lives beside the
// account credential in the user's config directory, never in a project, and
// only this laptop writes it: nothing the server sends can add a key or a
// project.
type State struct {
	// RP is fixed when the first passkey is paired, so the runner started at
	// login checks against the same site as the person who paired it.
	RP       *RP                `json:"rp,omitempty"`
	Keys     []Key              `json:"keys"`
	Projects map[string]Enabled `json:"projects"`
	// Seen holds each job id until it could no longer pass the expiry check,
	// which keeps this small.
	Seen map[string]int64 `json:"seen"`
}

// Key is a passkey paired with this laptop.
type Key struct {
	ID        string    `json:"id"`
	PublicKey string    `json:"publicKey"`
	Name      string    `json:"name"`
	PairedAt  time.Time `json:"pairedAt"`
}

// Enabled is a project remote jobs may run in.
type Enabled struct {
	Root      string    `json:"root"`
	Name      string    `json:"name"`
	EnabledAt time.Time `json:"enabledAt"`
	LastJobAt time.Time `json:"lastJobAt,omitzero"`
}

func (s *State) rp() RP {
	if s.RP != nil {
		return *s.RP
	}
	return DefaultRP
}

func (s *State) key(id string) (Key, bool) {
	for _, k := range s.Keys {
		if k.ID == id {
			return k, true
		}
	}
	return Key{}, false
}

func (s *State) see(id string, until, now time.Time) {
	for old, exp := range s.Seen {
		if now.After(time.UnixMilli(exp)) {
			delete(s.Seen, old)
		}
	}
	s.Seen[id] = until.UnixMilli()
}

// AddKey trusts a passkey, replacing an earlier pairing of the same one.
func (s *State) AddKey(k Key) {
	for i, old := range s.Keys {
		if old.ID == k.ID {
			s.Keys[i] = k
			return
		}
	}
	s.Keys = append(s.Keys, k)
}

// Enable lets remote jobs run in a project. Enabling again restarts the idle
// clock.
func (s *State) Enable(id, root, name string, now time.Time) {
	s.Projects[id] = Enabled{Root: root, Name: name, EnabledAt: now}
}

// ProjectAt finds the enabled project whose folder is root.
func (s *State) ProjectAt(root string) (string, Enabled, bool) {
	for id, p := range s.Projects {
		if p.Root == root {
			return id, p, true
		}
	}
	return "", Enabled{}, false
}

func path() (string, error) {
	d, err := account.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "remote.json"), nil
}

// Load reads the state, empty if remote was never enabled.
func Load() (*State, error) {
	s := &State{Projects: map[string]Enabled{}, Seen: map[string]int64{}}
	p, err := path()
	if err != nil {
		return s, err
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	} else if err != nil {
		return s, err
	}
	if err := json.Unmarshal(b, s); err != nil {
		return s, err
	}
	if s.Projects == nil {
		s.Projects = map[string]Enabled{}
	}
	if s.Seen == nil {
		s.Seen = map[string]int64{}
	}
	return s, nil
}

// Update reads the state, changes it and writes it back, holding a lock so
// the runner and a `trackline remote` command in a terminal never overwrite
// each other, and two runners never both accept one job.
func Update(change func(*State) error) error {
	p, err := path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	unlock, err := lock(p + ".lock")
	if err != nil {
		return err
	}
	defer unlock()
	s, err := Load()
	if err != nil {
		return err
	}
	if err := change(s); err != nil {
		// A refused job still changes the state (it is now seen), so the
		// change is kept whether or not it returned an error.
		if werr := save(p, s); werr != nil {
			return werr
		}
		return err
	}
	return save(p, s)
}

func save(p string, s *State) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// Forget removes every key and project: the laptop's kill switch. Afterwards
// no job passes until a passkey is paired again, on the laptop.
func Forget() error {
	return Update(func(s *State) error {
		s.Keys, s.RP = nil, nil
		s.Projects = map[string]Enabled{}
		return nil
	})
}
