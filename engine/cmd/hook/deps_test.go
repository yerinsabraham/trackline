package main

import (
	"os/exec"
	"strings"
	"testing"
)

// The hook starts before every tool call. Linking the network stack into it
// added 3.4ms to every start while it never made a request, so it must not
// come back by way of some import (docs/experiments/08).
func TestTheHookLinksNoNetworkCode(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", ".").Output()
	if err != nil {
		t.Skipf("go list unavailable: %v", err)
	}
	for _, pkg := range strings.Fields(string(out)) {
		if pkg == "net" || strings.HasPrefix(pkg, "net/") || pkg == "crypto/tls" {
			t.Errorf("the hook links %s", pkg)
		}
	}
}
