package account_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yerinsabraham/trackline/engine/internal/cloud/account"
)

func isolate(t *testing.T) string {
	t.Helper()
	d := filepath.Join(t.TempDir(), "trackline")
	t.Setenv("TRACKLINE_CONFIG_DIR", d)
	return d
}

// A credential readable by other users of the machine is a shared credential.
func TestCredentialsAreOwnerReadableOnly(t *testing.T) {
	d := isolate(t)
	if err := account.SaveCredentials(account.Credentials{API: "http://x", Token: "tl_dev_abc"}); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(filepath.Join(d, "credentials.json"))
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("credentials.json mode %v, want 0600", fi.Mode().Perm())
	}
	di, _ := os.Stat(d)
	if di.Mode().Perm() != 0o700 {
		t.Errorf("directory mode %v, want 0700", di.Mode().Perm())
	}
	got, err := account.LoadCredentials()
	if err != nil || got.Token != "tl_dev_abc" {
		t.Errorf("round trip: %+v %v", got, err)
	}
}

// The account learns a project by a random id, never its path, and the id
// stays the same when the project is connected again.
func TestProjectIDsAreRandomAndStable(t *testing.T) {
	isolate(t)
	a, _ := account.ConnectProject("/work/one", true)
	b, _ := account.ConnectProject("/work/two", false)
	again, _ := account.ConnectProject("/work/one", false)
	if !strings.HasPrefix(a.ID, "proj_") || a.ID == b.ID {
		t.Errorf("ids %q %q", a.ID, b.ID)
	}
	if again.ID != a.ID || again.ShareRequests {
		t.Errorf("reconnecting kept id %v and updated sharing to false: %+v", again.ID == a.ID, again)
	}
	if strings.Contains(a.ID, "work") {
		t.Error("a project id must not be derived from its path")
	}
}

func TestForgetRemovesEverything(t *testing.T) {
	d := isolate(t)
	account.SaveCredentials(account.Credentials{Token: "x"})
	account.ConnectProject("/work/one", true)
	if err := account.Forget(); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"credentials.json", "projects.json"} {
		if _, err := os.Stat(filepath.Join(d, f)); !os.IsNotExist(err) {
			t.Errorf("%s still exists", f)
		}
	}
	if err := account.Forget(); err != nil {
		t.Error("forgetting twice must not fail")
	}
}

// Agents run from subfolders. Their actions belong to the connected project,
// and a folder that merely shares a prefix with it does not.
func TestProjectForFindsTheEnclosingProject(t *testing.T) {
	isolate(t)
	base := t.TempDir()
	app := filepath.Join(base, "app")
	inner := filepath.Join(app, "packages", "web")
	account.ConnectProject(app, true)
	account.ConnectProject(inner, false)

	cases := map[string]string{
		app:                             app,
		filepath.Join(app, "src"):       app,
		inner:                           inner,
		filepath.Join(inner, "src"):     inner,
		filepath.Join(base, "app-copy"): "",
		base:                            "",
	}
	for dir, want := range cases {
		root, _, ok := account.ProjectFor(dir)
		if root != want || ok != (want != "") {
			t.Errorf("ProjectFor(%s) = %q %v, want %q", dir, root, ok, want)
		}
	}
}

// Disconnecting leaves nothing behind to upload later.
func TestForgetClearsTheOutbox(t *testing.T) {
	d := isolate(t)
	account.SaveCredentials(account.Credentials{API: "http://x", Token: "t"})
	os.MkdirAll(filepath.Join(d, "outbox"), 0o700)
	os.WriteFile(filepath.Join(d, "outbox", "1-a.json"), []byte("{}"), 0o600)
	if err := account.Forget(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(d, "outbox")); !os.IsNotExist(err) {
		t.Error("the outbox survived disconnecting")
	}
}
