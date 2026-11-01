package main

import (
	"testing"

	"github.com/yerinsabraham/trackline/engine/internal/cloud/payload"
)

func TestParseLive(t *testing.T) {
	for name, c := range map[string]struct {
		raw  string
		ok   bool
		kind string
		turn string
	}{
		"a message":       {`{"hook_event_name":"UserPromptSubmit","session_id":"s","prompt_id":"p","prompt":"hi"}`, true, payload.LivePrompt, "p"},
		"codex message":   {`{"hook_event_name":"UserPromptSubmit","session_id":"s","turn_id":"t","prompt":"hi"}`, true, payload.LivePrompt, "t"},
		"a prompt":        {`{"hook_event_name":"Notification","session_id":"s","notification_type":"permission_prompt","message":"Claude needs your permission"}`, true, payload.LiveWaiting, ""},
		"an idle note":    {`{"hook_event_name":"Notification","session_id":"s","notification_type":"idle_prompt"}`, false, "", ""},
		"codex approval":  {`{"hook_event_name":"PermissionRequest","session_id":"s","tool_name":"Bash"}`, true, payload.LiveWaiting, ""},
		"carried on":      {`{"hook_event_name":"PostToolUse","session_id":"s"}`, true, payload.LiveResumed, ""},
		"an action":       {`{"hook_event_name":"PreToolUse","session_id":"s"}`, false, "", ""},
		"the end of turn": {`{"hook_event_name":"Stop","session_id":"s"}`, false, "", ""},
		"not json":        {`nope`, false, "", ""},
	} {
		l, ok := parseLive([]byte(c.raw))
		if ok != c.ok || l.kind != c.kind || l.turn != c.turn {
			t.Errorf("%s: got %+v %v", name, l, ok)
		}
	}
}
