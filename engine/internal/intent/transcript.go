package intent

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"strings"
	"time"
)

// Reading a transcript is an incremental operation, not a parse.
//
// A real session transcript measured 3.2 MB across 1371 entries, and the hook
// that reads it has a budget under 10 milliseconds because it runs before every
// tool call. Re-reading the whole file each time is not available.
//
// So a Reader remembers where it stopped and only consumes what has been
// appended since. The file is append-only in normal operation; if it shrinks,
// it has been rotated or rewritten and the Reader starts over rather than
// reading from a meaningless offset.

// Reader consumes a host transcript incrementally.
type Reader struct {
	Path string

	offset int64
	size   int64

	sawEntries   int
	sawAssistant int
	sawKnown     int

	// codexTurn is the turn a Codex request belongs to. Codex writes it on a
	// task_started line just before the request, not on the request itself.
	codexTurn string
}

// Unrecognised reports a transcript with lines in it, none of which were in a
// shape this reader knows.
//
// ReadAnything alone missed exactly this. It flagged a conversation only when
// it spotted an assistant turn, and a format it could not read has no
// assistant turns it can spot. Codex sessions went through that gap from the
// start: every one read as "nothing asked yet", and scope reported it had
// nothing to compare against rather than that it could not see.
func (r *Reader) Unrecognised() bool {
	return r.sawEntries > 0 && r.sawKnown == 0
}

// ReadAnything reports whether the last Read saw a conversation at all.
//
// It exists to separate two states that look identical from the outside: a
// session where the user has not said anything yet, and a session whose format
// we failed to recognise. The first is ordinary. The second means every check
// that depends on intent is silently blind, and must say so rather than
// reporting that it found nothing to complain about.
func (r *Reader) ReadAnything() (entries, assistantTurns int) {
	return r.sawEntries, r.sawAssistant
}

// claudeEntry is the subset of a Claude Code transcript line that matters.
//
// Of 259 `user` entries in a measured session, 229 were tool results and only
// 31 were genuine human turns. TurnOrigin and Origin.Kind separate them
// cleanly, so no heuristic is needed for this part.
type claudeEntry struct {
	Type string `json:"type"`
	// Role is Cursor's field. Its transcript lines are {role, message} with no
	// type, timestamp or id, and share the message.content shape with Claude,
	// so one struct reads both.
	Role       string `json:"role"`
	UserType   string `json:"userType"`
	TurnOrigin string `json:"turnOrigin"`
	Origin     struct {
		Kind string `json:"kind"`
	} `json:"origin"`
	PromptID  string `json:"promptId"`
	Timestamp string `json:"timestamp"`
	// Payload is Codex's. Its lines are {timestamp, type, payload}.
	Payload json.RawMessage `json:"payload"`
	Message struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

// humanOrigins are the turnOrigin values that mean a person asked for this.
//
// "human" is an interactive session. "sdk" is a headless one, where the prompt
// arrives through the SDK and carries no origin object at all.
//
// Missing "sdk" was a real bug: every headless session read as having no
// request in it, so the checks that depend on intent reported not-applicable
// and looked like they had nothing to say. They had nothing to *see*. That is
// the same mistake this project keeps finding, in another costume.
var humanOrigins = map[string]bool{"human": true, "sdk": true}

func (e claudeEntry) isHumanTurn() bool {
	if e.Type != "user" {
		return false
	}
	return humanOrigins[e.TurnOrigin] || e.Origin.Kind == "human"
}

func (e claudeEntry) isAssistant() bool {
	return e.Type == "assistant" || (e.Type == "" && e.Role == "assistant")
}

// codexLine reads the parts of a Codex rollout line that matter.
//
// The request is the event_msg of type user_message. Codex also writes
// response_item messages with role user, but those include the instructions and
// environment it injects itself; five real requests in a measured session sat
// beside seven role-user items.
type codexLine struct {
	Type    string `json:"type"`
	TurnID  string `json:"turn_id"`
	Message string `json:"message"`
	Role    string `json:"role"`
}

func (e claudeEntry) codex() (codexLine, bool) {
	if (e.Type != "event_msg" && e.Type != "response_item") || len(e.Payload) == 0 {
		return codexLine{}, false
	}
	var c codexLine
	if json.Unmarshal(e.Payload, &c) != nil {
		return codexLine{}, false
	}
	return c, true
}

// cursorRequest returns what the person typed, from a Cursor user line.
//
// Cursor wraps it: a <timestamp> block, then the request inside <user_query>.
// The hook's own beforeSubmitPrompt payload carried an empty prompt in the
// captured session, so this is the only place the request is recorded at all.
//
// A user line with no <user_query> is something Cursor injected rather than
// something a person asked for, and is skipped rather than guessed at.
func (e claudeEntry) cursorRequest() (string, bool) {
	if e.Type != "" || e.Role != "user" {
		return "", false
	}
	txt := e.text()
	const open, close = "<user_query>", "</user_query>"
	i := strings.Index(txt, open)
	if i < 0 {
		return "", false
	}
	rest := txt[i+len(open):]
	if j := strings.Index(rest, close); j >= 0 {
		rest = rest[:j]
	}
	rest = strings.TrimSpace(rest)
	return rest, rest != ""
}

// text pulls the human-written text out of a message body, which is either a
// plain string or a list of typed parts. Tool results live in the list form
// too, and carry no text part, so they fall out naturally.
func (e claudeEntry) text() string {
	if len(e.Message.Content) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(e.Message.Content, &s) == nil {
		return s
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(e.Message.Content, &parts) != nil {
		return ""
	}
	var out string
	for _, p := range parts {
		if p.Type == "text" {
			if out != "" {
				out += "\n"
			}
			out += p.Text
		}
	}
	return out
}

// Read consumes everything appended since the last call and adds any human
// turns to in.
//
// A transcript that cannot be opened is not an error worth failing a tool call
// over: the caller gets the error and decides, and the hook's own policy is to
// carry on with whatever intent it already had rather than block the user.
func (r *Reader) Read(in *Intent) error {
	f, err := os.Open(r.Path)
	if err != nil {
		return err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return err
	}

	// Shrunk means rotated or rewritten. An old offset into a new file points
	// at the middle of an unrelated line, so start again.
	if st.Size() < r.size {
		r.offset = 0
	}
	r.size = st.Size()

	if r.offset > 0 {
		if _, err := f.Seek(r.offset, io.SeekStart); err != nil {
			return err
		}
	}

	// Counted so the caller can tell "the user said nothing yet" from "we read a
	// whole conversation and recognised none of it", which need different
	// answers.
	r.sawEntries = 0
	r.sawAssistant = 0
	r.sawKnown = 0

	sc := bufio.NewScanner(f)
	// Transcript lines carry whole messages and can be large; the default
	// 64 KiB limit truncates them mid-JSON.
	sc.Buffer(make([]byte, 0, 1<<20), 8<<20)

	var consumed int64
	for sc.Scan() {
		line := sc.Bytes()
		consumed += int64(len(line)) + 1 // +1 for the newline the scanner strips

		var e claudeEntry
		if json.Unmarshal(line, &e) != nil {
			continue // a line we cannot read is skipped, not fatal
		}
		r.sawEntries++
		if e.isAssistant() {
			r.sawAssistant++
		}
		switch {
		case e.Type == "user" || e.Type == "assistant":
			r.sawKnown++
		case e.Type == "" && (e.Role == "user" || e.Role == "assistant"):
			r.sawKnown++
		}
		if c, ok := e.codex(); ok {
			r.sawKnown++
			switch {
			case e.Type == "event_msg" && c.Type == "task_started":
				r.codexTurn = c.TurnID
			case e.Type == "event_msg" && c.Type == "user_message" && strings.TrimSpace(c.Message) != "":
				at, _ := time.Parse(time.RFC3339, e.Timestamp)
				in.Add(r.codexTurn, at, strings.TrimSpace(c.Message))
			case e.Type == "response_item" && c.Type == "message" && c.Role == "assistant":
				r.sawAssistant++
			}
			continue
		}
		if req, ok := e.cursorRequest(); ok {
			// No id and no timestamp on the line. Turns stay in order, which is
			// what Anchor and Recent need; matching a turn to its actions by id
			// is not available for Cursor, and review says so.
			in.Add("", time.Time{}, req)
			continue
		}
		if !e.isHumanTurn() {
			continue
		}
		txt := e.text()
		if txt == "" {
			continue
		}
		at, err := time.Parse(time.RFC3339, e.Timestamp)
		if err != nil {
			at = time.Time{}
		}
		in.Add(e.PromptID, at, txt)
	}
	if err := sc.Err(); err != nil {
		// Advance past what was read successfully so the next call makes
		// progress instead of re-reading a line that will fail again.
		r.offset += consumed
		return err
	}

	r.offset += consumed
	return nil
}
