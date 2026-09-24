// Package hosts says what trackline can and cannot do in each agent.
//
// Not every host can block, not every host says where its transcript is, and
// the ways each one fails are different. A tool that behaves identically
// everywhere by pretending is worse than one that says where it is weaker,
// because the user then trusts it most exactly where it sees least.
//
// Every entry here was established by a capture or a live run, and says which.
// A limit discovered later belongs here, not in someone's memory: the Codex
// transcript gap sat unnoticed through three phases because nothing listed
// what each host actually provided.
package hosts

import "fmt"

// Capabilities describe one host, as measured.
type Capabilities struct {
	Name string

	// Blocks is true when trackline can stop an action before it happens,
	// whether or not the agent cooperates.
	Blocks bool

	// ReasonReachesAgent is true when the agent is told why it was stopped,
	// which is what lets it correct itself rather than retry.
	ReasonReachesAgent bool

	// Intent says where the user's request comes from.
	Intent string

	// Evidence says how the above was established.
	Evidence string

	// Limits are the ways it sees less, in plain words for the user.
	Limits []string
}

var all = map[string]Capabilities{
	"claude": {
		Name:               "Claude Code",
		Blocks:             true,
		ReasonReachesAgent: true,
		Intent:             "read from the session transcript",
		Evidence:           "live sessions; the agent corrected itself 11 times out of 11 when blocked",
		Limits: []string{
			"ask mode cannot prompt you directly: the agent is stopped and told to ask you",
		},
	},
	"codex": {
		Name:               "Codex",
		Blocks:             true,
		ReasonReachesAgent: true,
		Intent:             "read from the session transcript",
		Evidence:           "live sessions and captured payloads",
		Limits: []string{
			"Codex runs a project hook only after you trust it with /hooks; until then it silently never runs",
			"if the hook crashes or times out, Codex lets the action through",
			"ask mode cannot prompt you directly: the agent is stopped and told to ask you",
		},
	},
	"cursor": {
		Name:               "Cursor",
		Blocks:             true,
		ReasonReachesAgent: true,
		Intent:             "read from the session transcript",
		Evidence:           "captured from the Cursor CLI; one live run where the agent was blocked once and did not retry",
		Limits: []string{
			"it only sees the agent once the agent is inside this project; start the agent here",
			"captured from the Cursor CLI; the desktop app is expected to match but has not been checked",
		},
	},
	"production": {
		Name:               "production traces",
		Blocks:             false,
		ReasonReachesAgent: false,
		Intent:             "the user message in the trace, when message content capture is on",
		Evidence:           "the official OTel OpenAI instrumentation and OTLP exporter, captured and replayed; 45 pre-registered conversations",
		Limits: []string{
			"a trace is a record of what already happened, so trackline alerts and never stops anything",
			"without message content capture, which is opt-in, a trace does not say which tool was called; checks report that rather than pass it",
			"a permitted tool used for something nobody asked for is only caught with --judge",
		},
	},
	"mcp": {
		Name:               "any MCP client",
		Blocks:             false,
		ReasonReachesAgent: true,
		Intent:             "whatever the agent says it was asked, which it may get wrong",
		Evidence:           "tested with the official MCP Inspector",
		Limits: []string{
			"the agent chooses whether to ask; an agent that does not ask is not watched",
			"approvals given for a single request do not apply; only project-wide ones do",
			"some clients start the server outside the project; pass --root in the client's config",
		},
	},
}

// For returns a host's capabilities, and false for a host trackline does not
// know. An unknown host is never given a default: guessing its capabilities is
// the mistake this package exists to prevent.
func For(host string) (Capabilities, bool) {
	c, ok := all[host]
	return c, ok
}

// Names lists the known hosts in a stable order.
func Names() []string { return []string{"claude", "codex", "cursor", "mcp", "production"} }

// Describe renders a host's capabilities for a person.
func (c Capabilities) Describe() string {
	yes := func(b bool) string {
		if b {
			return "yes"
		}
		return "no"
	}
	s := fmt.Sprintf("In %s:\n  can stop an action:       %s\n  tells the agent why:      %s\n  knows what you asked:     %s\n  established by:           %s\n",
		c.Name, yes(c.Blocks), yes(c.ReasonReachesAgent), c.Intent, c.Evidence)
	if !c.Blocks {
		s += "  It advises. It cannot stop anything.\n"
	}
	if len(c.Limits) > 0 {
		s += "  Where it sees less:\n"
		for _, l := range c.Limits {
			s += "    - " + l + "\n"
		}
	}
	return s
}
