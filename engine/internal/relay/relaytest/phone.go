// Package relaytest is a phone in software: a passkey that signs jobs and
// pairing codes exactly as a browser would, for tests and for sending a test
// job to a runner without a real phone.
package relaytest

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"

	"github.com/yerinsabraham/trackline/engine/internal/relay"
)

var b64 = base64.RawURLEncoding

// Phone holds one passkey.
type Phone struct {
	ID   string
	Name string
	// Key is an *ecdsa.PrivateKey (ES256), ed25519.PrivateKey (EdDSA) or
	// *rsa.PrivateKey (RS256): the three kinds of passkey there are.
	Key crypto.Signer
	RP  relay.RP
	// Origin overrides where the browser says it signed, and Flags the
	// authenticator's flags, so tests can play a phishing page or a
	// passkey used without Face ID.
	Origin      string
	CrossOrigin bool
	Flags       byte
	RPID        string
	Type        string
}

// New makes a phone with a fresh P-256 passkey for the product site.
func New(name string) *Phone {
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	return with(name, k)
}

// NewEd25519 makes a phone whose passkey is EdDSA, as security keys often are.
func NewEd25519(name string) *Phone {
	_, k, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}
	return with(name, k)
}

// NewRSA makes a phone whose passkey is RS256, as some Windows Hello ones are.
func NewRSA(name string) *Phone {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	return with(name, k)
}

func with(name string, k crypto.Signer) *Phone {
	id := make([]byte, 16)
	rand.Read(id)
	return &Phone{ID: b64.EncodeToString(id), Name: name, Key: k, RP: relay.DefaultRP, Flags: 0x05}
}

// PublicKey is the passkey's public key as the server stores it.
func (p *Phone) PublicKey() string { return relay.EncodeKey(p.Key.Public()) }

// Sign makes a WebAuthn assertion over challenge.
func (p *Phone) Sign(challenge []byte) relay.Assertion {
	origin := p.RP.Origin
	if p.Origin != "" {
		origin = p.Origin
	}
	rpID := p.RP.ID
	if p.RPID != "" {
		rpID = p.RPID
	}
	typ := "webauthn.get"
	if p.Type != "" {
		typ = p.Type
	}
	client, _ := json.Marshal(map[string]any{
		"type":        typ,
		"challenge":   b64.EncodeToString(challenge),
		"origin":      origin,
		"crossOrigin": p.CrossOrigin,
	})
	rpHash := sha256.Sum256([]byte(rpID))
	auth := append(rpHash[:], p.Flags, 0, 0, 0, 0)
	clientHash := sha256.Sum256(client)
	signed := append(append([]byte{}, auth...), clientHash[:]...)
	digest := sha256.Sum256(signed)
	var sig []byte
	var err error
	switch k := p.Key.(type) {
	case *ecdsa.PrivateKey:
		sig, err = ecdsa.SignASN1(rand.Reader, k, digest[:])
	case ed25519.PrivateKey:
		sig = ed25519.Sign(k, signed)
	case *rsa.PrivateKey:
		sig, err = rsa.SignPKCS1v15(rand.Reader, k, crypto.SHA256, digest[:])
	}
	if err != nil {
		panic(err)
	}
	return relay.Assertion{
		AuthenticatorData: b64.EncodeToString(auth),
		ClientDataJSON:    b64.EncodeToString(client),
		Signature:         b64.EncodeToString(sig),
	}
}

// Send signs a job, as the composer on the site will.
func (p *Phone) Send(j relay.Job) relay.Envelope {
	raw, _ := json.Marshal(j)
	return p.SendRaw(raw)
}

// SendRaw signs job bytes exactly as given, for jobs no honest site would
// write.
func (p *Phone) SendRaw(raw []byte) relay.Envelope {
	return relay.Envelope{Job: b64.EncodeToString(raw), Key: p.ID, Assertion: p.Sign(relay.JobChallenge(raw))}
}

// Pair answers a laptop's pairing code.
func (p *Phone) Pair(deviceID, code string) relay.PairAnswer {
	var a relay.PairAnswer
	a.Key.ID, a.Key.PublicKey, a.Key.Name = p.ID, p.PublicKey(), p.Name
	a.Assertion = p.Sign(relay.PairChallenge(deviceID, relay.NormalizeCode(code)))
	return a
}

// Trusted is the key a laptop keeps after pairing with this phone.
func (p *Phone) Trusted() relay.Key {
	return relay.Key{ID: p.ID, PublicKey: p.PublicKey(), Name: p.Name}
}
