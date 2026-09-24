// Package production watches a deployed agent through its traces.
//
// The bet from the start of the build plan is tested here: if the core was
// designed right, production is an adapter and not a rewrite. So this package
// turns spans into the same events and intent a hook produces, and hands them
// to the same engine. The engine, the event model, intent and verdicts are not
// changed by it.
//
// What production adds is a second kind of check that has no place in the
// hook: things that are only visible across many runs. A tool failing once is
// a tool call; failing six conversations in a row is an outage. Those live in
// Monitor, beside the engine, because the engine judges one action at a time
// and that is the right shape for everything else.
package production

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/yerinsabraham/trackline/engine/internal/adapter/otel"
	"github.com/yerinsabraham/trackline/engine/internal/config"
	"github.com/yerinsabraham/trackline/engine/internal/engine"
	"github.com/yerinsabraham/trackline/engine/internal/event"
	"github.com/yerinsabraham/trackline/engine/internal/intent"
	"github.com/yerinsabraham/trackline/engine/internal/signal"
	"github.com/yerinsabraham/trackline/engine/internal/signal/toolpolicy"
	"github.com/yerinsabraham/trackline/engine/internal/verdict"
)

// Conversation is one agent run, reassembled from its trace.
type Conversation struct {
	TraceID string `json:"traceId"`
	ID      string `json:"id"`
	Service string `json:"service,omitempty"`

	// Requests are what the user said, in order, each once. Every model call
	// repeats the whole history in its input, so they are deduplicated.
	Requests []string `json:"requests,omitempty"`
	// Rules are the system prompt's constraints, as stated.
	Rules []string `json:"rules,omitempty"`

	// Actions are the tools the agent ran. execute_tool spans are what
	// actually ran; a model's tool_call is only what it asked for. Taking both
	// would count every call twice, so the model's requests are used only when
	// a trace has no execute_tool spans at all.
	Actions []event.Event `json:"actions"`
	// ToolErrors is how many of those actions failed.
	ToolErrors []string `json:"toolErrors,omitempty"`

	ModelCalls   int           `json:"modelCalls"`
	ModelLatency time.Duration `json:"modelLatency"` // median across the run's model calls
	Tokens       int           `json:"tokens"`
	Start        time.Time     `json:"start"`

	// ContentAvailable is false when the trace carried no message content, in
	// which case Requests and Rules are empty because they were never
	// recorded, not because there were none. Reason says which.
	ContentAvailable bool   `json:"contentAvailable"`
	Reason           string `json:"reason,omitempty"`
}

// Assemble groups spans into conversations, oldest first.
func Assemble(spans []otel.Span) []Conversation {
	byTrace := map[string][]otel.Span{}
	var order []string
	for _, s := range spans {
		if _, seen := byTrace[s.TraceID]; !seen {
			order = append(order, s.TraceID)
		}
		byTrace[s.TraceID] = append(byTrace[s.TraceID], s)
	}

	var out []Conversation
	for _, id := range order {
		out = append(out, assemble(id, byTrace[id]))
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Start.Before(out[j].Start) })
	return out
}

func assemble(traceID string, spans []otel.Span) Conversation {
	sort.SliceStable(spans, func(i, j int) bool { return spans[i].Start.Before(spans[j].Start) })
	c := Conversation{TraceID: traceID, ID: traceID}

	seenReq := map[string]bool{}
	seenRule := map[string]bool{}
	var asked []event.Event
	var latencies []time.Duration
	anyContent, corrupt := false, false
	var reason string

	for i, s := range spans {
		if i == 0 || s.Start.Before(c.Start) {
			c.Start = s.Start
		}
		if c.Service == "" {
			c.Service = s.Service
		}
		if id := s.Attributes["gen_ai.conversation.id"]; id != "" {
			c.ID = id
		}

		switch s.Attributes["gen_ai.operation.name"] {
		case "execute_tool":
			ev := event.Event{
				ID: s.Attributes["gen_ai.tool.call.id"], SessionID: traceID, TurnID: traceID,
				At: s.Start, Host: event.HostOTel, Phase: event.PhasePostTool,
				Action: event.Action{Type: event.ActionCallTool, ToolName: s.Attributes["gen_ai.tool.name"], PathsUnknown: true},
			}
			// The arguments are the body: what the call carried. Loops are
			// found by the same tool with the same body.
			ev.Action.SetBody(s.Attributes["gen_ai.tool.call.arguments"])
			if ev.Action.ToolName == "" {
				ev.Action.ToolNameUnknown = true
			}
			c.Actions = append(c.Actions, ev)
			if s.Status == "error" {
				c.ToolErrors = append(c.ToolErrors, ev.Action.ToolName)
			}

		case "chat", "text_completion", "generate_content":
			c.ModelCalls++
			if !s.End.IsZero() {
				latencies = append(latencies, s.End.Sub(s.Start))
			}
			c.Tokens += atoi(s.Attributes["gen_ai.usage.input_tokens"]) + atoi(s.Attributes["gen_ai.usage.output_tokens"])

			p := otel.Parse(s, s.Start)
			anyContent = anyContent || p.ContentAvailable
			corrupt = corrupt || p.ContentCorrupt
			if !p.ContentAvailable && reason == "" {
				reason = p.Reason
			}
			for _, u := range p.UserMessages {
				if !seenReq[u] {
					seenReq[u] = true
					c.Requests = append(c.Requests, u)
				}
			}
			for _, r := range p.SystemRules {
				if !seenRule[r] {
					seenRule[r] = true
					c.Rules = append(c.Rules, r)
				}
			}
			for _, ev := range p.Events {
				ev.SessionID, ev.TurnID = traceID, traceID
				asked = append(asked, ev)
			}
		}
	}

	if len(c.Actions) == 0 {
		c.Actions = asked
	}
	c.ContentAvailable = anyContent
	if !anyContent {
		c.Reason = reason
		if corrupt && reason == "" {
			c.Reason = "the trace carries message content that could not be parsed"
		}
	}
	c.ModelLatency = median(latencies)
	return c
}

// Result is what trackline concluded about one conversation.
type Result struct {
	Conversation Conversation     `json:"conversation"`
	Actions      []ActionResult   `json:"actions"`
	Incidents    []Incident       `json:"incidents,omitempty"`
	Unmeasured   []verdict.Result `json:"unmeasured,omitempty"`
}

// ActionResult is the engine's report on one action.
type ActionResult struct {
	Tool    string           `json:"tool"`
	Results []verdict.Result `json:"results"`
}

// Findings flattens every finding in the result.
func (r Result) Findings() []verdict.Verdict {
	var out []verdict.Verdict
	for _, a := range r.Actions {
		for _, res := range a.Results {
			if res.Outcome == verdict.OutcomeFinding {
				out = append(out, res.Verdicts...)
			}
		}
	}
	return out
}

// Evaluate runs each of a conversation's actions through the engine, with the
// conversation's own requests as intent and its system prompt as rules.
func Evaluate(c Conversation, cfg config.Config) Result {
	var in intent.Intent
	for _, r := range c.Requests {
		in.Add(c.TraceID, c.Start, r)
	}
	unavailable := ""
	if !c.ContentAvailable {
		unavailable = c.Reason
	}
	var rules []signal.Rule
	for i, r := range c.Rules {
		rules = append(rules, signal.Rule{ID: "system-" + strconv.Itoa(i), Text: r, Source: "system prompt"})
	}

	// History is this conversation's earlier actions. It is built as the loop
	// walks forward, so a check sees exactly what had happened before.
	var done []event.Event
	history := func(event.Event) []event.Event { return done }

	var sigs []signal.Signal
	if !cfg.IsDisabled(toolpolicy.Name) {
		sigs = append(sigs, toolpolicy.New(cfg.Tools, history))
	}
	e := engine.New(sigs...)

	res := Result{Conversation: c}
	for _, ev := range c.Actions {
		rep := e.Run(ev, in, rules, unavailable)
		res.Actions = append(res.Actions, ActionResult{Tool: ev.Action.ToolName, Results: rep.Results})
		done = append(done, ev)
	}
	if !c.ContentAvailable {
		res.Unmeasured = append(res.Unmeasured, verdict.CannotMeasure("content", c.Reason))
	}
	return res
}

func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

func median(ds []time.Duration) time.Duration {
	if len(ds) == 0 {
		return 0
	}
	s := append([]time.Duration(nil), ds...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	return s[len(s)/2]
}

// argsKey identifies a call by tool and arguments, normalising JSON spacing so
// the same call serialised twice still matches.
func argsKey(ev event.Event) string {
	body := ev.Action.Body
	var v any
	if json.Unmarshal([]byte(body), &v) == nil {
		if b, err := json.Marshal(v); err == nil {
			body = string(b)
		}
	}
	return ev.Action.ToolName + "\x00" + body
}

// Process evaluates conversations in arrival order, one monitor across all of
// them, since an outage is only visible as a sequence.
func Process(convs []Conversation, cfg config.Config, m *Monitor) []Result {
	var out []Result
	for _, c := range convs {
		r := Evaluate(c, cfg)
		r.Incidents = m.Observe(c)
		out = append(out, r)
	}
	return out
}
