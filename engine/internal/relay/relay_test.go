package relay_test

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/yerinsabraham/trackline/engine/internal/relay"
	"github.com/yerinsabraham/trackline/engine/internal/relay/relaytest"
)

const machine = "dev_laptop"

var now = time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)

type fixture struct {
	state *relay.State
	phone *relaytest.Phone
	root  string
}

func setup(t *testing.T) fixture {
	t.Helper()
	phone := relaytest.New("iPhone")
	s := &relay.State{Projects: map[string]relay.Enabled{}, Seen: map[string]int64{}}
	s.AddKey(phone.Trusted())
	root := t.TempDir()
	s.Enable("proj_enabled", root, "app", now.Add(-time.Hour))
	return fixture{s, phone, root}
}

func job(mod ...func(*relay.Job)) relay.Job {
	j := relay.Job{
		V: 1, ID: "job_1", Machine: machine, Project: "proj_enabled", Kind: "test",
		Text: "say hello", IssuedAt: now.UnixMilli(), ExpiresAt: now.Add(3 * time.Minute).UnixMilli(),
	}
	for _, m := range mod {
		m(&j)
	}
	return j
}

func code(err error) string {
	var r *relay.Refusal
	if errors.As(err, &r) {
		return r.Code
	}
	if err != nil {
		return "error: " + err.Error()
	}
	return "accepted"
}

func TestSignedJobForAnEnabledProjectIsAccepted(t *testing.T) {
	f := setup(t)
	got, err := relay.Check(f.state, f.phone.Send(job()), machine, now)
	if err != nil {
		t.Fatalf("refused: %v", err)
	}
	if got.Root != f.root || got.Job.Text != "say hello" {
		t.Fatalf("accepted the wrong thing: %+v", got)
	}
	if f.state.Projects["proj_enabled"].LastJobAt != now {
		t.Fatal("the project's idle clock was not restarted")
	}
}

// Each of these must be refused, and refused for its own reason: a forged
// job reported as merely expired would hide the attack.
func TestRefusals(t *testing.T) {
	other := relaytest.New("attacker")

	cases := []struct {
		name string
		want string
		env  func(f fixture) relay.Envelope
		now  time.Time
	}{
		{"signed by a key that claims a paired key's id", "bad-signature", func(f fixture) relay.Envelope {
			other.ID = f.phone.ID
			return other.Send(job())
		}, now},
		{"signed by a key never paired", "unknown-key", func(f fixture) relay.Envelope {
			return relaytest.New("stranger").Send(job())
		}, now},
		{"altered after signing", "bad-signature", func(f fixture) relay.Envelope {
			env := f.phone.Send(job())
			raw, _ := json.Marshal(job(func(j *relay.Job) { j.Text = "rm -rf ~" }))
			env.Job = base64.RawURLEncoding.EncodeToString(raw)
			return env
		}, now},
		{"signature from the sign-in step-up, over the raw job hash", "bad-signature", func(f fixture) relay.Envelope {
			raw, _ := json.Marshal(job())
			h := sha256.Sum256(raw)
			env := f.phone.SendRaw(raw)
			env.Assertion = f.phone.Sign(h[:])
			return env
		}, now},
		{"a pairing signature presented as a job", "bad-signature", func(f fixture) relay.Envelope {
			env := f.phone.Send(job())
			env.Assertion = f.phone.Pair(machine, "ABCD1234").Assertion
			return env
		}, now},
		{"signed on another site's page", "bad-signature", func(f fixture) relay.Envelope {
			f.phone.Origin = "https://trackline.dev.evil.example"
			return f.phone.Send(job())
		}, now},
		{"signed inside a frame on another site", "bad-signature", func(f fixture) relay.Envelope {
			f.phone.CrossOrigin = true
			return f.phone.Send(job())
		}, now},
		{"a passkey for another site", "bad-signature", func(f fixture) relay.Envelope {
			f.phone.RPID = "evil.example"
			return f.phone.Send(job())
		}, now},
		{"a registration presented as a signature", "bad-signature", func(f fixture) relay.Envelope {
			f.phone.Type = "webauthn.create"
			return f.phone.Send(job())
		}, now},
		{"signed without Face ID or a PIN", "bad-signature", func(f fixture) relay.Envelope {
			f.phone.Flags = 0x01
			return f.phone.Send(job())
		}, now},
		{"for another machine", "wrong-machine", func(f fixture) relay.Envelope {
			return f.phone.Send(job(func(j *relay.Job) { j.Machine = "dev_other" }))
		}, now},
		{"expired", "expired", func(f fixture) relay.Envelope { return f.phone.Send(job()) }, now.Add(6 * time.Minute)},
		{"dated in the future", "not-yet-valid", func(f fixture) relay.Envelope { return f.phone.Send(job()) }, now.Add(-3 * time.Minute)},
		{"valid for a day", "too-long-lived", func(f fixture) relay.Envelope {
			return f.phone.Send(job(func(j *relay.Job) { j.ExpiresAt = now.Add(24 * time.Hour).UnixMilli() }))
		}, now},
		{"for a folder not enabled", "project-not-enabled", func(f fixture) relay.Envelope {
			return f.phone.Send(job(func(j *relay.Job) { j.Project = "proj_other" }))
		}, now},
		{"asking for full access", "unsupported", func(f fixture) relay.Envelope {
			return f.phone.SendRaw(withField(job(), "mode", "danger-full-access"))
		}, now},
		{"naming a folder", "unsupported", func(f fixture) relay.Envelope {
			return f.phone.SendRaw(withField(job(), "cwd", "/"))
		}, now},
		{"a kind of job this runner does not do", "unsupported", func(f fixture) relay.Envelope {
			return f.phone.Send(job(func(j *relay.Job) { j.Kind = "shell" }))
		}, now},
		{"a prompt naming no agent", "unsupported", func(f fixture) relay.Envelope {
			return f.phone.Send(job(func(j *relay.Job) { j.Kind = "prompt" }))
		}, now},
		{"a prompt with nothing to do", "unsupported", func(f fixture) relay.Envelope {
			return f.phone.Send(job(func(j *relay.Job) { j.Kind, j.Agent, j.Text = "prompt", "claude", "  " }))
		}, now},
		{"an agent that is not one", "unsupported", func(f fixture) relay.Envelope {
			return f.phone.Send(job(func(j *relay.Job) { j.Agent = "bash" }))
		}, now},
		{"not base64", "bad-envelope", func(f fixture) relay.Envelope {
			env := f.phone.Send(job())
			env.Job = "not base64!"
			return env
		}, now},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := setup(t)
			_, err := relay.Check(f.state, c.env(f), machine, c.now)
			if got := code(err); got != c.want {
				t.Fatalf("got %s, want %s (%v)", got, c.want, err)
			}
		})
	}
}

func withField(j relay.Job, k, v string) []byte {
	var m map[string]any
	raw, _ := json.Marshal(j)
	json.Unmarshal(raw, &m)
	m[k] = v
	out, _ := json.Marshal(m)
	return out
}

func TestTheSameJobIsAcceptedOnce(t *testing.T) {
	f := setup(t)
	env := f.phone.Send(job())
	if _, err := relay.Check(f.state, env, machine, now); err != nil {
		t.Fatal(err)
	}
	if got := code(func() error { _, err := relay.Check(f.state, env, machine, now.Add(time.Second)); return err }()); got != "replayed" {
		t.Fatalf("second delivery: %s", got)
	}
}

// Refused for a project that is not enabled, a job must not become
// acceptable by enabling the project and sending it again.
func TestARefusedJobStaysRefused(t *testing.T) {
	f := setup(t)
	env := f.phone.Send(job(func(j *relay.Job) { j.Project = "proj_later" }))
	relay.Check(f.state, env, machine, now)
	f.state.Enable("proj_later", t.TempDir(), "later", now)
	if got := code(func() error { _, err := relay.Check(f.state, env, machine, now); return err }()); got != "replayed" {
		t.Fatalf("got %s", got)
	}
}

func TestSeenJobsAreForgottenOnceTheyCouldNotPassAnyway(t *testing.T) {
	f := setup(t)
	relay.Check(f.state, f.phone.Send(job()), machine, now)
	relay.Check(f.state, f.phone.Send(job(func(j *relay.Job) {
		j.ID, j.IssuedAt, j.ExpiresAt = "job_2", now.Add(time.Hour).UnixMilli(), now.Add(time.Hour+time.Minute).UnixMilli()
	})), machine, now.Add(time.Hour))
	if _, ok := f.state.Seen["job_1"]; ok {
		t.Fatal("an expired job id is still kept")
	}
}

func TestAProjectUnusedForAMonthIsTurnedOff(t *testing.T) {
	f := setup(t)
	later := now.Add(31 * 24 * time.Hour)
	env := f.phone.Send(job(func(j *relay.Job) {
		j.IssuedAt, j.ExpiresAt = later.UnixMilli(), later.Add(time.Minute).UnixMilli()
	}))
	if got := code(func() error { _, err := relay.Check(f.state, env, machine, later); return err }()); got != "project-idle" {
		t.Fatalf("got %s", got)
	}
	if _, ok := f.state.Projects["proj_enabled"]; ok {
		t.Fatal("the idle project is still enabled")
	}
}

func TestAProjectWhoseFolderIsGoneIsRefused(t *testing.T) {
	f := setup(t)
	f.state.Enable("proj_enabled", f.root+"/gone", "app", now)
	if got := code(func() error { _, err := relay.Check(f.state, f.phone.Send(job()), machine, now); return err }()); got != "project-missing" {
		t.Fatalf("got %s", got)
	}
}

func TestPairing(t *testing.T) {
	phone := relaytest.New("iPhone")
	code, err := relay.NewCode()
	if err != nil || len(code) != relay.CodeLength {
		t.Fatalf("code %q: %v", code, err)
	}

	typed := strings.ToLower(relay.ShowCode(code))
	k, err := relay.VerifyPair(relay.DefaultRP, machine, code, phone.Pair(machine, typed), now)
	if err != nil {
		t.Fatalf("an honest answer was refused: %v", err)
	}
	if k.ID != phone.ID || k.Name != "iPhone" {
		t.Fatalf("trusted the wrong key: %+v", k)
	}

	if _, err := relay.VerifyPair(relay.DefaultRP, machine, code, phone.Pair(machine, "00000000"), now); err == nil {
		t.Fatal("accepted a signature over the wrong code")
	}
	if _, err := relay.VerifyPair(relay.DefaultRP, machine, code, phone.Pair("dev_other", code), now); err == nil {
		t.Fatal("accepted a pairing meant for another machine")
	}
	// The server swaps in its own key but cannot sign the code it never saw.
	swapped := phone.Pair(machine, code)
	swapped.Key.PublicKey = relaytest.New("server").PublicKey()
	if _, err := relay.VerifyPair(relay.DefaultRP, machine, code, swapped, now); err == nil {
		t.Fatal("accepted a substituted key")
	}
}

func TestNormalizeCode(t *testing.T) {
	if got := relay.NormalizeCode("k7qf-2mxo il"); got != "K7QF2MX011" {
		t.Fatalf("got %s", got)
	}
}

func TestParseKeyRefusesWhatItCannotUse(t *testing.T) {
	good := relaytest.New("x").PublicKey()
	if _, err := relay.ParseKey(good); err != nil {
		t.Fatalf("refused a real key: %v", err)
	}
	raw, _ := base64.RawURLEncoding.DecodeString(good)

	eddsa := append([]byte{}, raw...)
	eddsa[4] = 0x27 // alg -8, EdDSA
	offCurve := append([]byte{}, raw...)
	offCurve[len(offCurve)-1] ^= 1
	for name, b := range map[string][]byte{
		"empty":         {},
		"not a map":     {0x01},
		"truncated":     raw[:20],
		"trailing":      append(append([]byte{}, raw...), 0),
		"other alg":     eddsa,
		"off the curve": offCurve,
	} {
		if _, err := relay.ParseKey(base64.RawURLEncoding.EncodeToString(b)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
