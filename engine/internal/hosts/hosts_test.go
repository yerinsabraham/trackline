package hosts_test

import (
	"strings"
	"testing"

	"github.com/yerinsabraham/trackline/engine/internal/hosts"
	"github.com/yerinsabraham/trackline/engine/internal/install"
	"github.com/yerinsabraham/trackline/engine/internal/runner"
)

// Every host trackline can be installed into, or can read payloads from, has
// an entry. A host added without one would be supported with its limits
// unstated, which is how the Codex transcript gap went unnoticed.
func TestEveryHostHasItsLimitsStated(t *testing.T) {
	for _, h := range []string{string(install.Claude), string(install.Codex), string(install.Cursor),
		runner.HostClaude, runner.HostCodex, runner.HostCursor, "mcp"} {
		c, ok := hosts.For(h)
		if !ok {
			t.Errorf("%s has no capabilities entry", h)
			continue
		}
		if c.Evidence == "" || c.Intent == "" {
			t.Errorf("%s: every claim needs the evidence it rests on", h)
		}
	}
}

func TestUnknownHostIsNotGuessed(t *testing.T) {
	if _, ok := hosts.For("windsurf"); ok {
		t.Error("a host nobody has captured must not get default capabilities")
	}
}

// The one host that cannot block must say so in words, not only in a field a
// person never reads.
func TestAdvisoryHostSaysItCannotStopAnything(t *testing.T) {
	c, _ := hosts.For("mcp")
	d := c.Describe()
	if !strings.Contains(d, "can stop an action:       no") || !strings.Contains(d, "cannot stop anything") {
		t.Errorf("MCP must say plainly that it only advises:\n%s", d)
	}
	for _, name := range hosts.Names() {
		if _, ok := hosts.For(name); !ok {
			t.Errorf("Names lists %s with no entry", name)
		}
	}
}
