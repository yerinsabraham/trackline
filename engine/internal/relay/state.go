package relay

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/yerinsabraham/trackline/engine/internal/cloud/account"
	"github.com/yerinsabraham/trackline/engine/internal/intent"
)

// State is what this laptop trusts for remote jobs. It lives beside the
// account credential in the user's config directory, never in a project, and
// only this laptop writes it: nothing the server sends can add a key or a
// project.
type State struct {
	// RP is fixed when the first passkey is paired, so the runner started at
	// login checks against the same site as the person who paired it.
	RP   *RP   `json:"rp,omitempty"`
	Keys []Key `json:"keys"`
	// Password is the send password, when one is set here. See password.go.
	Password *Password          `json:"password,omitempty"`
	Projects map[string]Enabled `json:"projects"`
	// OpenChats lets a phone message join a Codex chat open on this laptop. That
	// chat keeps its own permissions, not the narrow ones a phone run gets, so
	// only a person at a terminal here turns it on.
	OpenChats bool `json:"openChats,omitempty"`
	// Handoffs are the handoffs this laptop has started, by id, so one runs
	// once whatever is delivered again, and a result survives a failed report.
	Handoffs map[string]Handoff `json:"handoffs,omitempty"`
	// All means every project connected on this laptop takes jobs, so a new
	// one needs no step of its own. Pairing a passkey here is the consent: it
	// is done at this laptop, and no job passes without that passkey's
	// signature.
	All bool `json:"all,omitempty"`
	// Off are projects the person turned off by hand. All never brings one
	// back; only turning it on again in that project does.
	Off []string `json:"off,omitempty"`
	// OffRoots are their folders, as the same folder can be connected again
	// under another name and id.
	OffRoots []string `json:"offRoots,omitempty"`
	// Seen holds each job id until it could no longer pass the expiry check,
	// which keeps this small.
	Seen map[string]int64 `json:"seen"`
	// Agents are the agent binaries found when remote was last enabled, and
	// Path the PATH they were found on. The runner starts at login under
	// launchd, whose PATH is bare, and an agent needs git, node and the rest.
	Agents map[string]string `json:"agents,omitempty"`
	Path   string            `json:"path,omitempty"`
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
	// Guard is config.Guard of the project when remote was enabled: what the
	// person agreed to be watched by. A job is refused once it differs.
	// Accepting a difference by itself between jobs is not safe, however much
	// friction it would save: a job can leave a process behind that rewrites
	// these files after it ends, and the next job then runs unwatched (shown
	// in the review of 2026-10-31).
	Guard string `json:"guard,omitempty"`
	// Sessions are the agent sessions remote jobs started here, newest last,
	// so one can be continued even if it never took an action trackline saw.
	Sessions []string `json:"sessions,omitempty"`
}

// Handoff is what this laptop remembers of one handoff.
type Handoff struct {
	Job     string    `json:"job"`
	Project string    `json:"project"`
	At      time.Time `json:"at"`
	// Finished is set with Result, Reason and Session once the receiver has
	// ended. Reported is set once the account has that.
	Finished bool   `json:"finished,omitempty"`
	Result   string `json:"result,omitempty"`
	Failed   string `json:"failed,omitempty"`
	Session  string `json:"session,omitempty"`
	Reported bool   `json:"reported,omitempty"`
}

// handoffsKept is far longer than any job may wait to arrive.
const handoffsKept = 30 * 24 * time.Hour

// ClaimHandoff records a handoff as started, and reports whether it was the
// first to. A retry would otherwise repeat the task to recover an answer.
func (s *State) ClaimHandoff(id, job, project string, now time.Time) bool {
	if _, done := s.Handoffs[id]; done {
		return false
	}
	if s.Handoffs == nil {
		s.Handoffs = map[string]Handoff{}
	}
	for k, h := range s.Handoffs {
		if now.Sub(h.At) > handoffsKept {
			delete(s.Handoffs, k)
		}
	}
	s.Handoffs[id] = Handoff{Job: job, Project: project, At: now}
	return true
}

// sessionsKept bounds Enabled.Sessions.
const sessionsKept = 100

// Ran records that a remote job in project started or continued session.
func (s *State) Ran(project, session string) {
	p, ok := s.Projects[project]
	if !ok || session == "" {
		return
	}
	p.Sessions = append(slices.DeleteFunc(p.Sessions, func(x string) bool { return x == session }), session)
	if len(p.Sessions) > sessionsKept {
		p.Sessions = p.Sessions[len(p.Sessions)-sessionsKept:]
	}
	s.Projects[project] = p
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

// Enable lets remote jobs run in a project, and is the person saying so: it
// clears any turning off. Enabling again accepts the guarded files as they
// are, restarts the idle clock and keeps the project's conversations.
func (s *State) Enable(id, root, name, guard string, now time.Time) {
	s.Off = slices.DeleteFunc(s.Off, func(x string) bool { return x == id })
	s.OffRoots = slices.DeleteFunc(s.OffRoots, func(x string) bool { return within(root, x) })
	s.Projects[id] = Enabled{Root: root, Name: name, EnabledAt: now, Guard: guard, Sessions: s.Projects[id].Sessions}
}

// TurnOff stops jobs in a project, whether or not it was on yet, and keeps it
// off when All would otherwise bring it in.
func (s *State) TurnOff(id, root string) {
	delete(s.Projects, id)
	if !slices.Contains(s.Off, id) {
		s.Off = append(s.Off, id)
	}
	if !slices.Contains(s.OffRoots, root) {
		s.OffRoots = append(s.OffRoots, root)
	}
}

// within reports whether path is folder or inside it, whatever links lead there.
func within(path, folder string) bool {
	rel, err := filepath.Rel(resolved(folder), resolved(path))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// resolved follows links as far as the path exists, so a path not there yet
// still compares properly with one under a linked folder such as macOS's /var.
func resolved(p string) string {
	p = filepath.Clean(p)
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	parent := filepath.Dir(p)
	if parent == p {
		return p
	}
	return filepath.Join(resolved(parent), filepath.Base(p))
}

// Within is within, for the laptop's other checks of what lies in a project.
func Within(path, folder string) bool { return within(path, folder) }

// knownFolder reports whether root is a project already on or turned off, or
// inside one. Connected again by another name it would get a new id and the
// guard as it stands, walking past both a turning off and a changed guard.
func (s *State) knownFolder(root string, on []string) bool {
	for _, r := range on {
		if within(root, r) {
			return true
		}
	}
	return slices.ContainsFunc(s.OffRoots, func(r string) bool { return within(root, r) })
}

// Enroll enables every connected project that is not on yet, and returns their
// names. It adds nothing unless a passkey is paired here and All is set, skips
// what the person turned off, and skips a folder that has gone.
func (s *State) Enroll(connected map[string]account.Project, guard func(root string) string, now time.Time) []string {
	if !s.All || len(s.Keys) == 0 {
		return nil
	}
	var on []string
	for _, p := range s.Projects {
		on = append(on, p.Root)
	}
	var added []string
	for root, p := range connected {
		if _, already := s.Projects[p.ID]; already || p.ID == "" || slices.Contains(s.Off, p.ID) || s.knownFolder(root, on) {
			continue
		}
		if fi, err := os.Stat(root); err != nil || !fi.IsDir() {
			continue
		}
		name := filepath.Base(root)
		s.Enable(p.ID, root, name, guard(root), now)
		added = append(added, name)
	}
	slices.Sort(added)
	return added
}

// WouldEnroll names what Enroll would add once All is set.
func (s State) WouldEnroll(connected map[string]account.Project) []string {
	s.All = true
	s.Projects = maps.Clone(s.Projects)
	return s.Enroll(connected, func(string) string { return "" }, time.Now())
}

// GrantsPath is where approvals made from the phone for a project are kept:
// beside remote.json, outside the project, where an agent working in the
// project cannot write one for itself.
func GrantsPath(project string) (string, error) {
	d, err := account.Dir()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(project))
	return filepath.Join(d, "remote-grants", hex.EncodeToString(sum[:8])+".json"), nil
}

// RecordPeer notes text that one agent wrote to another, by its hash, so the
// checks never read it as something the person asked for. It lives here, beside
// the approvals, and not in the project, where an agent could edit it.
func RecordPeer(text string) error {
	d, err := account.Dir()
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(d, intent.PeerFile), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(intent.PeerHash(text) + "\n")
	return err
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
		s.Keys, s.RP, s.Password = nil, nil, nil
		s.Projects = map[string]Enabled{}
		s.All, s.Off, s.OffRoots = false, nil, nil
		return nil
	})
}
