package relay

import (
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
)

// A job is signed with a passkey: a WebAuthn assertion whose challenge is a
// hash of the job. The laptop verifies it itself, against keys it learned at
// pairing, so the server that relays the job cannot make one up.
//
// Only ES256 (P-256) is accepted. It is what iPhone, Mac, Android and
// Windows passkeys produce, and one algorithm is less to get wrong.

// Assertion is what the browser's navigator.credentials.get returns, as
// base64url strings.
type Assertion struct {
	AuthenticatorData string `json:"authenticatorData"`
	ClientDataJSON    string `json:"clientDataJSON"`
	Signature         string `json:"signature"`
}

// Relying party: the site the passkeys belong to.
type RP struct {
	ID     string `json:"rpId"`
	Origin string `json:"origin"`
}

// DefaultRP is the product site. A passkey made for any other site does not
// sign for trackline, whatever the server says.
var DefaultRP = RP{ID: "trackline.dev", Origin: "https://trackline.dev"}

var b64 = base64.RawURLEncoding

// decodeB64 accepts base64url with or without padding: browsers' helpers
// differ.
func decodeB64(s string) ([]byte, error) {
	for len(s)%4 != 0 && len(s) > 0 && s[len(s)-1] == '=' {
		s = s[:len(s)-1]
	}
	return b64.DecodeString(s)
}

// Challenges are prefixed per purpose, so a signature made for one purpose
// can never be replayed as another. Without it, a server that wanted a job
// signed could offer that job's hash as a sign-in challenge.
const (
	jobPurpose  = "trackline job v1\n"
	pairPurpose = "trackline pair v1\n"
)

func challenge(purpose string, parts ...[]byte) []byte {
	h := sha256.New()
	h.Write([]byte(purpose))
	for i, p := range parts {
		if i > 0 {
			h.Write([]byte{'\n'})
		}
		h.Write(p)
	}
	return h.Sum(nil)
}

// JobChallenge is what the phone asks its passkey to sign for a job.
func JobChallenge(job []byte) []byte { return challenge(jobPurpose, job) }

// PairChallenge is what the phone signs to prove it saw the code on this
// laptop's screen.
func PairChallenge(deviceID, code string) []byte {
	return challenge(pairPurpose, []byte(deviceID), []byte(code))
}

const (
	flagUserPresent  = 0x01
	flagUserVerified = 0x04
)

// verifyAssertion checks that key signed want as a WebAuthn assertion for rp,
// with the person verified (Face ID, fingerprint or PIN), not merely present.
func verifyAssertion(key *ecdsa.PublicKey, rp RP, a Assertion, want []byte) error {
	authData, err := decodeB64(a.AuthenticatorData)
	if err != nil || len(authData) < 37 {
		return errors.New("authenticator data is malformed")
	}
	clientJSON, err := decodeB64(a.ClientDataJSON)
	if err != nil {
		return errors.New("client data is malformed")
	}
	sig, err := decodeB64(a.Signature)
	if err != nil {
		return errors.New("signature is malformed")
	}

	var client struct {
		Type        string `json:"type"`
		Challenge   string `json:"challenge"`
		Origin      string `json:"origin"`
		CrossOrigin bool   `json:"crossOrigin"`
	}
	if err := json.Unmarshal(clientJSON, &client); err != nil {
		return errors.New("client data is malformed")
	}
	if client.Type != "webauthn.get" {
		return fmt.Errorf("client data is a %q, not an assertion", client.Type)
	}
	// Signed inside a frame on another site, the origin would still read as
	// ours; crossOrigin is the browser saying so.
	if client.Origin != rp.Origin || client.CrossOrigin {
		return fmt.Errorf("signed on %s, not %s", client.Origin, rp.Origin)
	}
	got, err := decodeB64(client.Challenge)
	if err != nil || subtle.ConstantTimeCompare(got, want) != 1 {
		return errors.New("signed something else")
	}

	rpHash := sha256.Sum256([]byte(rp.ID))
	if subtle.ConstantTimeCompare(authData[:32], rpHash[:]) != 1 {
		return errors.New("the passkey belongs to another site")
	}
	flags := authData[32]
	if flags&flagUserPresent == 0 || flags&flagUserVerified == 0 {
		return errors.New("signed without the person verifying it")
	}

	clientHash := sha256.Sum256(clientJSON)
	signed := sha256.Sum256(append(append([]byte{}, authData...), clientHash[:]...))
	if !ecdsa.VerifyASN1(key, signed[:], sig) {
		return errors.New("the signature does not match")
	}
	return nil
}

// ParseKey reads a passkey's public key as the server stores it: a COSE key,
// base64url, as the registration returned it.
func ParseKey(cose string) (*ecdsa.PublicKey, error) {
	raw, err := decodeB64(cose)
	if err != nil {
		return nil, errors.New("public key is not base64url")
	}
	m, err := coseMap(raw)
	if err != nil {
		return nil, err
	}
	const (
		kty, alg, crv, x, y = 1, 3, -1, -2, -3
		ec2, es256, p256    = 2, -7, 1
	)
	if m.ints[kty] != ec2 || m.ints[alg] != es256 || m.ints[crv] != p256 {
		return nil, errors.New("only P-256 (ES256) passkeys can sign jobs")
	}
	xb, yb := m.bytes[x], m.bytes[y]
	if len(xb) != 32 || len(yb) != 32 {
		return nil, errors.New("public key has the wrong length")
	}
	// ecdh rejects points not on the curve, which ecdsa would not.
	point := append(append([]byte{4}, xb...), yb...)
	if _, err := ecdh.P256().NewPublicKey(point); err != nil {
		return nil, errors.New("public key is not a point on P-256")
	}
	return &ecdsa.PublicKey{Curve: elliptic.P256(), X: new(big.Int).SetBytes(xb), Y: new(big.Int).SetBytes(yb)}, nil
}

// EncodeKey writes a P-256 key as a COSE key, base64url: the inverse of
// ParseKey, for tests and the test sender.
func EncodeKey(k *ecdsa.PublicKey) string {
	var x, y [32]byte
	k.X.FillBytes(x[:])
	k.Y.FillBytes(y[:])
	out := []byte{0xa5, 0x01, 0x02, 0x03, 0x26, 0x20, 0x01, 0x21, 0x58, 0x20}
	out = append(out, x[:]...)
	out = append(out, 0x22, 0x58, 0x20)
	out = append(out, y[:]...)
	return b64.EncodeToString(out)
}

// A COSE key is a small CBOR map of integer keys to integers or byte strings.
// This reads exactly that and refuses anything else, rather than bringing in
// a general CBOR library for five fields.
type cose struct {
	ints  map[int64]int64
	bytes map[int64][]byte
}

func coseMap(b []byte) (cose, error) {
	m := cose{ints: map[int64]int64{}, bytes: map[int64][]byte{}}
	bad := errors.New("public key is not a COSE key")
	major, n, rest, err := cborHead(b)
	if err != nil || major != 5 || n > 16 {
		return m, bad
	}
	for i := uint64(0); i < n; i++ {
		var km, vm byte
		var kv, vv uint64
		if km, kv, rest, err = cborHead(rest); err != nil {
			return m, bad
		}
		key, ok := cborInt(km, kv)
		if !ok {
			return m, bad
		}
		if vm, vv, rest, err = cborHead(rest); err != nil {
			return m, bad
		}
		switch vm {
		case 0, 1:
			v, _ := cborInt(vm, vv)
			m.ints[key] = v
		case 2:
			if vv > uint64(len(rest)) {
				return m, bad
			}
			m.bytes[key], rest = rest[:vv], rest[vv:]
		default:
			return m, bad
		}
	}
	if len(rest) != 0 {
		return m, bad
	}
	return m, nil
}

func cborHead(b []byte) (major byte, n uint64, rest []byte, err error) {
	if len(b) == 0 {
		return 0, 0, nil, errors.New("short")
	}
	major, info := b[0]>>5, b[0]&0x1f
	b = b[1:]
	switch {
	case info < 24:
		return major, uint64(info), b, nil
	case info == 24 && len(b) >= 1:
		return major, uint64(b[0]), b[1:], nil
	case info == 25 && len(b) >= 2:
		return major, uint64(binary.BigEndian.Uint16(b)), b[2:], nil
	case info == 26 && len(b) >= 4:
		return major, uint64(binary.BigEndian.Uint32(b)), b[4:], nil
	}
	return 0, 0, nil, errors.New("unsupported")
}

func cborInt(major byte, v uint64) (int64, bool) {
	if v > 1<<31 {
		return 0, false
	}
	switch major {
	case 0:
		return int64(v), true
	case 1:
		return -1 - int64(v), true
	}
	return 0, false
}
