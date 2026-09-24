// Package remote talks to the trackline account API: connecting a machine,
// asking who it is connected as, and uploading.
//
// Kept apart from package account so that the hook, which only reads files,
// never links the network stack.
package remote

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/yerinsabraham/trackline/engine/internal/cloud/payload"
)

// DefaultBase is where accounts live unless TRACKLINE_API says otherwise.
const DefaultBase = "https://api.creovine.com/trackline/v1"

// Base returns the API base: the flag if given, then TRACKLINE_API, then the default.
func Base(flag string) string {
	for _, v := range []string{flag, os.Getenv("TRACKLINE_API"), DefaultBase} {
		if v != "" {
			return strings.TrimRight(v, "/")
		}
	}
	return DefaultBase
}

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

// HTTPStatus is what the outbox reads to decide between retrying, setting a
// batch aside, and giving up on a revoked machine.
func (e *Error) HTTPStatus() int { return e.Status }

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

// Ingest uploads one batch.
func (c Client) Ingest(b payload.Batch) error { return c.do("POST", "/ingest", b, nil) }
