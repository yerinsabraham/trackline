package payload

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// What a deployed agent's results look like on the way to the account. The
// contract is docs/contract/production-v1.schema.json.
//
// Production is a different privacy problem from a developer's own laptop:
// the conversations are the customers', not the person uploading. So this is
// its own contract with its own, narrower field list, and nothing in it can
// carry what a customer or the model said. The builder lives in the production
// package, which knows what a result is; these types only say what may leave.

// ProdBatch is one upload of checked conversations.
type ProdBatch struct {
	V             int                `json:"v"`
	Conversations []ProdConversation `json:"conversations"`
}

// ProdConversation is what trackline concluded about one conversation.
type ProdConversation struct {
	ID               string           `json:"id"`
	At               string           `json:"at"`
	Service          string           `json:"service"`
	Conversation     string           `json:"conversation"`
	Tools            []ProdTool       `json:"tools"`
	Findings         []ProdFinding    `json:"findings"`
	Incidents        []ProdIncident   `json:"incidents"`
	ModelCalls       int              `json:"modelCalls"`
	Tokens           int              `json:"tokens"`
	ModelLatencyMs   int64            `json:"modelLatencyMs,omitempty"`
	ContentAvailable bool             `json:"contentAvailable"`
	Unmeasured       []ProdUnmeasured `json:"unmeasured,omitempty"`
}

// ProdTool is a tool by name, with how its calls went. Never its arguments.
type ProdTool struct {
	Name   string `json:"name"`
	Calls  int    `json:"calls"`
	Errors int    `json:"errors"`
}

// ProdFinding is a policy finding, in trackline's own words.
type ProdFinding struct {
	Check    string `json:"check"`
	Severity string `json:"severity"`
	Summary  string `json:"summary"`
	Tool     string `json:"tool,omitempty"`
}

// ProdIncident is an operational failure: an outage, a loop, a slowdown.
type ProdIncident struct {
	Kind    string `json:"kind"`
	Summary string `json:"summary"`
}

// ProdUnmeasured is a check that could not run, and why.
type ProdUnmeasured struct {
	Check  string `json:"check"`
	Reason string `json:"reason"`
}

// ProdID names one checked conversation, the same way every time it is
// sent, so a retried batch is stored once.
func ProdID(service, traceID string) string {
	sum := sha256.Sum256([]byte(service + "\x00" + traceID))
	return "pc_" + hex.EncodeToString(sum[:16])
}

// ProdTime is a time as the contract carries it.
func ProdTime(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

// Clean is free text trackline wrote, made fit to leave: token shapes
// removed and cut to n bytes. Summaries name tools and counts, but a tool
// name or a conversation id is whatever the agent's developer chose, and a
// key pasted into one should not travel.
func Clean(s string, n int) string { return cut(redact(s), n) }
