package relay

import (
	"encoding/hex"
	"testing"
)

// The site computes the same challenge in the browser (site/lib/remote.ts,
// pairChallenge). If either side changes alone, every pairing fails, so both
// are pinned to this value.
func TestPairChallengeVector(t *testing.T) {
	got := hex.EncodeToString(PairChallenge("dev_example", "ABCD1234"))
	const want = "1d4f10326e9e095539d58eec0dadd00fac2b4ef1c3071a210e479452f89465b5"
	if got != want {
		t.Fatalf("PairChallenge = %s", got)
	}
}
