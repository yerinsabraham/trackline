package account_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

func TestAPIPrecedence(t *testing.T) {
	t.Setenv("TRACKLINE_API", "")
	if account.API("") != account.DefaultAPI {
		t.Error("default")
	}
	t.Setenv("TRACKLINE_API", "http://localhost:4100/trackline/v1/")
	if account.API("") != "http://localhost:4100/trackline/v1" {
		t.Error("env, trailing slash trimmed")
	}
	if account.API("http://flag") != "http://flag" {
		t.Error("the flag wins")
	}
}

// The client sends the device credential as a bearer token, and surfaces the
// API's own error message.
func TestClientAgainstAStandIn(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/devices/code":
			json.NewEncoder(w).Encode(map[string]any{"deviceCode": "poll", "userCode": "BCDF-GHJK", "verificationUriComplete": "http://site/connect?code=BCDF-GHJK", "expiresIn": 600, "interval": 5})
		case "/devices/me":
			if r.Header.Get("authorization") != "Bearer tl_dev_good" {
				w.WriteHeader(401)
				json.NewEncoder(w).Encode(map[string]string{"error": "This machine was disconnected."})
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"device": map[string]string{"id": "d1", "name": "mac"}, "user": map[string]string{"email": "ada@example.com"}})
		}
	}))
	defer srv.Close()

	code, err := (account.Client{Base: srv.URL}).StartConnect("mac")
	if err != nil || code.UserCode != "BCDF-GHJK" {
		t.Fatalf("start: %+v %v", code, err)
	}
	me, err := (account.Client{Base: srv.URL, Token: "tl_dev_good"}).Me()
	if err != nil || me.User.Email != "ada@example.com" {
		t.Fatalf("me: %+v %v", me, err)
	}
	_, err = (account.Client{Base: srv.URL, Token: "tl_dev_bad"}).Me()
	var apiErr *account.Error
	if err == nil || !strings.Contains(err.Error(), "disconnected") || !asErr(err, &apiErr) || apiErr.Status != 401 {
		t.Errorf("err = %v", err)
	}
}

func asErr(err error, target **account.Error) bool {
	e, ok := err.(*account.Error)
	if ok {
		*target = e
	}
	return ok
}
