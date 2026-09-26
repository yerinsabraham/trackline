package override_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yerinsabraham/trackline/engine/internal/override"
)

func TestOnceOnlyHoldsWithinItsRequest(t *testing.T) {
	s := override.NewStore(t.TempDir())
	s.Add(override.Grant{Signal: "scope", Target: "src/payments/charge.ts",
		Scope: override.ScopeOnce, Session: "s1", Turn: "t1"})

	if _, ok := s.Allows("scope", "src/payments/charge.ts", "s1", "t1"); !ok {
		t.Error("the approval should hold in the request it was given in")
	}
	// A new request is a new decision. Someone approving one thing has
	// approved one thing.
	if _, ok := s.Allows("scope", "src/payments/charge.ts", "s1", "t2"); ok {
		t.Error("a one-off approval must not carry into the next request")
	}
	if _, ok := s.Allows("scope", "src/payments/charge.ts", "s2", "t1"); ok {
		t.Error("a one-off approval must not carry into another session")
	}
}

func TestProjectScopePersists(t *testing.T) {
	s := override.NewStore(t.TempDir())
	s.Add(override.Grant{Signal: "scope", Target: "src/payments/charge.ts", Scope: override.ScopeProject})
	if _, ok := s.Allows("scope", "src/payments/charge.ts", "other", "other"); !ok {
		t.Error("a project approval should hold anywhere in the project")
	}
}

// Prefix matching would quietly approve more than anyone meant.
func TestApprovalIsExactUnlessItSaysOtherwise(t *testing.T) {
	s := override.NewStore(t.TempDir())
	s.Add(override.Grant{Signal: "scope", Target: "src/auth", Scope: override.ScopeProject})

	if _, ok := s.Allows("scope", "src/authority/thing.ts", "s", "t"); ok {
		t.Error("approving src/auth must not approve src/authority")
	}
	if _, ok := s.Allows("scope", "src/auth/login.ts", "s", "t"); ok {
		t.Error("approving a path must not approve everything under it unless asked")
	}

	s.Add(override.Grant{Signal: "scope", Target: "src/auth/**", Scope: override.ScopeProject})
	if _, ok := s.Allows("scope", "src/auth/login.ts", "s", "t"); !ok {
		t.Error("an explicit /** should cover what is beneath it")
	}
	if _, ok := s.Allows("scope", "src/authority/thing.ts", "s", "t"); ok {
		t.Error("/** must still respect the directory boundary")
	}
}

func TestApprovalDoesNotLeakAcrossChecks(t *testing.T) {
	s := override.NewStore(t.TempDir())
	s.Add(override.Grant{Signal: "scope", Target: "/w/.env", Scope: override.ScopeProject})
	if _, ok := s.Allows("off-limits", "/w/.env", "s", "t"); ok {
		t.Error("approving one check's finding must not silence another's")
	}
}

func TestSurvivesARestart(t *testing.T) {
	dir := t.TempDir()
	first := override.NewStore(dir)
	first.Add(override.Grant{Signal: "scope", Target: "a.ts", Scope: override.ScopeProject})

	// A fresh process, which is what the hook always is.
	if _, ok := override.NewStore(dir).Allows("scope", "a.ts", "s", "t"); !ok {
		t.Error("an approval must outlive the process that recorded it")
	}
}

// A corrupt file must fail toward asking, never toward allowing.
func TestCorruptStoreApprovesNothing(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".trackline"), 0o755)
	os.WriteFile(filepath.Join(dir, ".trackline", "overrides.json"), []byte("{not json"), 0o600)

	if _, ok := override.NewStore(dir).Allows("scope", "a.ts", "s", "t"); ok {
		t.Error("an unreadable overrides file must not approve anything")
	}
}

func TestRemove(t *testing.T) {
	dir := t.TempDir()
	s := override.NewStore(dir)
	s.Add(override.Grant{Signal: "scope", Target: "a.ts", Scope: override.ScopeProject})
	s.Add(override.Grant{Signal: "scope", Target: "b.ts", Scope: override.ScopeProject})

	n, err := s.Remove("scope", "a.ts")
	if err != nil || n != 1 {
		t.Fatalf("removed %d, err %v", n, err)
	}
	if _, ok := s.Allows("scope", "a.ts", "s", "t"); ok {
		t.Error("a removed approval must stop applying")
	}
	if _, ok := s.Allows("scope", "b.ts", "s", "t"); !ok {
		t.Error("removing one must not remove the others")
	}
}

// "Allow once" from the phone holds for the next matching action in its
// session, in whatever request that comes, and never anywhere else.
func TestNextHoldsInItsSessionAcrossRequests(t *testing.T) {
	root := t.TempDir()
	s := override.NewStore(root)
	s.Add(override.Grant{Signal: "off-limits", Target: ".env", Scope: override.ScopeNext, Session: "s1"})

	// The dashboard shows paths relative to the project; the check sees them whole.
	g, ok := s.Allows("off-limits", filepath.Join(root, ".env"), "s1", "a-later-request")
	if !ok {
		t.Fatal("the approval should hold for the retry in a later request")
	}
	if _, ok := s.Allows("off-limits", filepath.Join(root, ".env"), "s2", "t"); ok {
		t.Error("an approval for one session must not hold in another")
	}
	if _, ok := s.Allows("off-limits", filepath.Join(root, "config", ".env"), "s1", "t"); ok {
		t.Error("an approval for one file must not hold for another")
	}
	if err := s.Consume(g); err != nil {
		t.Fatal(err)
	}
	if _, ok := override.NewStore(root).Allows("off-limits", filepath.Join(root, ".env"), "s1", "t"); ok {
		t.Error("allowed once must mean once")
	}
}

func TestNextLapsesUnused(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, ".trackline"), 0o755)
	old := time.Now().Add(-override.NextLife - time.Minute).UTC().Format(time.RFC3339)
	os.WriteFile(filepath.Join(root, ".trackline", "overrides.json"),
		[]byte(`[{"signal":"off-limits","target":".env","scope":"next","session":"s1","at":"`+old+`"}]`), 0o600)
	if _, ok := override.NewStore(root).Allows("off-limits", ".env", "s1", "t"); ok {
		t.Fatal("an approval unused for over an hour still held")
	}
}
