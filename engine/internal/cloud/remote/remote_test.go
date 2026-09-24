package remote_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yerinsabraham/trackline/engine/internal/cloud/remote"
)

func TestBasePrecedence(t *testing.T) {
	t.Setenv("TRACKLINE_API", "")
	if remote.Base("") != remote.DefaultBase {
		t.Error("default")
	}
	t.Setenv("TRACKLINE_API", "http://localhost:4100/trackline/v1/")
	if remote.Base("") != "http://localhost:4100/trackline/v1" {
		t.Error("env, trailing slash trimmed")
	}
	if remote.Base("http://flag") != "http://flag" {
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

	code, err := (remote.Client{Base: srv.URL}).StartConnect("mac")
	if err != nil || code.UserCode != "BCDF-GHJK" {
		t.Fatalf("start: %+v %v", code, err)
	}
	me, err := (remote.Client{Base: srv.URL, Token: "tl_dev_good"}).Me()
	if err != nil || me.User.Email != "ada@example.com" {
		t.Fatalf("me: %+v %v", me, err)
	}
	_, err = (remote.Client{Base: srv.URL, Token: "tl_dev_bad"}).Me()
	var apiErr *remote.Error
	if err == nil || !strings.Contains(err.Error(), "disconnected") || !asErr(err, &apiErr) || apiErr.Status != 401 {
		t.Errorf("err = %v", err)
	}
}

func asErr(err error, target **remote.Error) bool {
	e, ok := err.(*remote.Error)
	if ok {
		*target = e
	}
	return ok
}
