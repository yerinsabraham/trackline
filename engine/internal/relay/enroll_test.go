package relay

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yerinsabraham/trackline/engine/internal/cloud/account"
)

func fixedGuard(string) string { return "g" }

func state() *State {
	return &State{Projects: map[string]Enabled{}, Seen: map[string]int64{}, All: true, Keys: []Key{{ID: "k"}}}
}

func TestEnrollTurnsOnEveryConnectedProject(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	s := state()
	added := s.Enroll(map[string]account.Project{a: {ID: "a"}, b: {ID: "b"}}, fixedGuard, time.Now())
	if len(added) != 2 || s.Projects["a"].Root != a || s.Projects["b"].Guard != "g" {
		t.Fatalf("added %v, projects %v", added, s.Projects)
	}
}

// The consent is pairing at this laptop, so nothing may be added without it.
func TestEnrollAddsNothingWithoutAPairedPasskey(t *testing.T) {
	s := state()
	s.Keys = nil
	if added := s.Enroll(map[string]account.Project{t.TempDir(): {ID: "a"}}, fixedGuard, time.Now()); len(added) != 0 {
		t.Fatalf("enrolled without a passkey: %v", added)
	}
}

func TestEnrollAddsNothingUnlessAllIsOn(t *testing.T) {
	s := state()
	s.All = false
	if added := s.Enroll(map[string]account.Project{t.TempDir(): {ID: "a"}}, fixedGuard, time.Now()); len(added) != 0 {
		t.Fatalf("enrolled with All off: %v", added)
	}
}

func TestATurnedOffProjectStaysOffUntilTurnedOn(t *testing.T) {
	root := t.TempDir()
	s := state()
	connected := map[string]account.Project{root: {ID: "a"}}
	s.Enroll(connected, fixedGuard, time.Now())
	s.TurnOff("a", root)
	if added := s.Enroll(connected, fixedGuard, time.Now()); len(added) != 0 {
		t.Fatalf("a project turned off came back: %v", added)
	}
	s.Enable("a", root, "a", "g", time.Now())
	if _, on := s.Projects["a"]; !on || len(s.Off) != 0 {
		t.Fatalf("turning it on by hand did not stick: %v %v", s.Projects, s.Off)
	}
}

func TestEnrollSkipsAFolderThatHasGone(t *testing.T) {
	s := state()
	gone := filepath.Join(t.TempDir(), "gone")
	if added := s.Enroll(map[string]account.Project{gone: {ID: "a"}}, fixedGuard, time.Now()); len(added) != 0 {
		t.Fatalf("enrolled a missing folder: %v", added)
	}
}

func TestEnrollKeepsWhatIsAlreadyOn(t *testing.T) {
	root := t.TempDir()
	s := state()
	s.Enable("a", root, "a", "old", time.Now())
	s.Ran("a", "session-1")
	s.Enroll(map[string]account.Project{root: {ID: "a"}}, fixedGuard, time.Now())
	if p := s.Projects["a"]; p.Guard != "old" || len(p.Sessions) != 1 {
		t.Fatalf("enrolling rewrote a project already on: %+v", p)
	}
}

func TestEnablingAgainKeepsConversations(t *testing.T) {
	s := state()
	s.Enable("a", "/x", "a", "g", time.Now())
	s.Ran("a", "session-1")
	s.Enable("a", "/x", "a", "g2", time.Now())
	if len(s.Projects["a"].Sessions) != 1 {
		t.Fatal("enabling again lost the project's conversations")
	}
}

// The same folder under a new name gets a new id. It must not walk past a
// turning off, or past a changed guard by being taken as it stands.
func TestAProjectConnectedAgainByAnotherNameIsNotEnrolled(t *testing.T) {
	root, elsewhere := t.TempDir(), t.TempDir()
	link := filepath.Join(elsewhere, "alias")
	if err := os.Symlink(root, link); err != nil {
		t.Skip(err)
	}
	sub := filepath.Join(root, "sub")
	os.Mkdir(sub, 0o755)

	off := state()
	off.TurnOff("a", root)
	if added := off.Enroll(map[string]account.Project{link: {ID: "b"}, sub: {ID: "c"}}, fixedGuard, time.Now()); len(added) != 0 {
		t.Fatalf("a turned off project came back as: %v", added)
	}

	on := state()
	on.Enable("a", root, "a", "old", time.Now())
	if added := on.Enroll(map[string]account.Project{link: {ID: "b"}, sub: {ID: "c"}}, fixedGuard, time.Now()); len(added) != 0 {
		t.Fatalf("a project already on was enrolled again as: %v", added)
	}
}

func TestWithin(t *testing.T) {
	root := t.TempDir()
	for path, want := range map[string]bool{root: true, root + "/a/b": true, root + "/..bin/claude": true, root + "x": false, filepath.Dir(root): false} {
		if got := Within(path, root); got != want {
			t.Errorf("Within(%s) = %v", path, got)
		}
	}
}

func TestForgetClearsAll(t *testing.T) {
	t.Setenv("TRACKLINE_CONFIG_DIR", t.TempDir())
	if err := Update(func(s *State) error { s.All, s.Off = true, []string{"a"}; return nil }); err != nil {
		t.Fatal(err)
	}
	if err := Forget(); err != nil {
		t.Fatal(err)
	}
	s, err := Load()
	if err != nil || s.All || len(s.Off) != 0 {
		t.Fatalf("still on after the kill switch: %+v, %v", s, err)
	}
}

func TestLocalUseKeepsAProjectFromGoingIdle(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".trackline")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "events.jsonl"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !usedRecently(Enabled{Root: root, EnabledAt: time.Now().Add(-90 * 24 * time.Hour)}, time.Now()) {
		t.Fatal("a project used at the desk today was treated as idle")
	}
	if usedRecently(Enabled{Root: t.TempDir(), EnabledAt: time.Now().Add(-90 * 24 * time.Hour)}, time.Now()) {
		t.Fatal("a project unused for 90 days was treated as in use")
	}
	// An agent can set this file's time, and must not switch the rule off.
	future := time.Now().Add(10 * 365 * 24 * time.Hour)
	os.Chtimes(filepath.Join(dir, "events.jsonl"), future, future)
	if usedRecently(Enabled{Root: root, EnabledAt: time.Now().Add(-90 * 24 * time.Hour)}, time.Now()) {
		t.Fatal("a time in the future kept a project from pausing")
	}
}

func TestTurningOffAProjectNotOnYetKeepsItOut(t *testing.T) {
	root := t.TempDir()
	s := state()
	s.TurnOff("a", root)
	if added := s.Enroll(map[string]account.Project{root: {ID: "a"}}, fixedGuard, time.Now()); len(added) != 0 {
		t.Fatalf("it came on with the rest: %v", added)
	}
}
