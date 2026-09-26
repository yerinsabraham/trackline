package production_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/yerinsabraham/trackline/engine/internal/cloud/contracttest"
	"github.com/yerinsabraham/trackline/engine/internal/cloud/payload"
	"github.com/yerinsabraham/trackline/engine/internal/production"
)

// The whole recorded stream, as it would be uploaded: inside the contract,
// with the same findings and incidents trackline traces reports, and not one
// word a customer or the model said.
func TestTheStreamUploadsWithinTheContract(t *testing.T) {
	schema, err := contracttest.Load("production-v1.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	convs := stream(t)
	results := production.Process(convs, policy(), production.NewMonitor())

	var batch payload.ProdBatch
	batch.V = 1
	findings, incidents := 0, 0
	for _, r := range results {
		c := production.Upload(r)
		batch.Conversations = append(batch.Conversations, c)
		findings += len(c.Findings)
		incidents += len(c.Incidents)
	}
	b, _ := json.Marshal(batch)
	var v any
	json.Unmarshal(b, &v)
	if errs := contracttest.Validate(schema, schema, v, "$"); len(errs) > 0 {
		t.Fatalf("outside the contract: %v", errs)
	}
	// 8 incidents: two loops, the token spike each causes, three slow runs
	// and one outage. trackline traces over these files reports the same.
	if len(batch.Conversations) != 45 || findings != 3 || incidents != 8 {
		t.Errorf("uploaded %d conversations, %d findings, %d incidents; trackline traces reports 45, 3, 8",
			len(batch.Conversations), findings, incidents)
	}

	// Every request and system prompt rule in the stream, checked for.
	sent := string(b)
	for _, c := range convs {
		for _, text := range append(append([]string{}, c.Requests...), c.Rules...) {
			if len(text) > 12 && strings.Contains(sent, text) {
				t.Fatalf("customer or prompt text left in the upload: %q", text)
			}
		}
	}
}

// The same conversation sent twice has the same id, so it is stored once.
func TestAnUploadIDIsStable(t *testing.T) {
	r := production.Result{Conversation: production.Conversation{TraceID: "t1", ID: "c1", Service: "bank"}}
	if production.Upload(r).ID != production.Upload(r).ID {
		t.Fatal("ids differ")
	}
	r2 := r
	r2.Conversation.Service = "other"
	if production.Upload(r).ID == production.Upload(r2).ID {
		t.Fatal("two services share an id")
	}
}

type sender struct {
	fail    error
	batches [][]payload.ProdConversation
}

func (s *sender) Production(b payload.ProdBatch) error {
	if s.fail != nil {
		return s.fail
	}
	s.batches = append(s.batches, b.Conversations)
	return nil
}

type httpErr int

func (e httpErr) Error() string   { return "status" }
func (e httpErr) HTTPStatus() int { return int(e) }

func result(i int) production.Result {
	return production.Result{Conversation: production.Conversation{TraceID: string(rune('a' + i)), ID: "c", Service: "bank"}}
}

// An account that cannot be reached keeps what is waiting; once it is back,
// all of it goes, in order.
func TestAnOutageLosesNothing(t *testing.T) {
	s := &sender{fail: errors.New("offline")}
	u := production.NewUploader(s)
	for i := range 3 {
		u.Add(result(i))
	}
	if err := u.Flush(); err == nil {
		t.Fatal("a failed send reported success")
	}
	s.fail = nil
	if err := u.Flush(); err != nil {
		t.Fatal(err)
	}
	if sent, waiting, _, _ := u.Stats(); sent != 3 || waiting != 0 || len(s.batches) != 1 {
		t.Fatalf("sent %d, waiting %d, batches %d", sent, waiting, len(s.batches))
	}
}

// A batch the account refuses as malformed is set aside, not retried for
// ever in front of everything behind it.
func TestARefusedBatchDoesNotBlockTheQueue(t *testing.T) {
	s := &sender{fail: httpErr(400)}
	u := production.NewUploader(s)
	u.Add(result(0))
	u.Flush()
	if _, waiting, dropped, _ := u.Stats(); waiting != 0 || dropped != 1 {
		t.Fatalf("waiting %d, dropped %d", waiting, dropped)
	}
	// Signed out is not malformed: kept for when the machine is connected again.
	s.fail = httpErr(401)
	u.Add(result(1))
	u.Flush()
	if _, waiting, _, _ := u.Stats(); waiting != 1 {
		t.Fatalf("a 401 dropped the batch")
	}
}

// Down for a long time, the oldest are dropped and counted, never the memory.
func TestTheQueueIsBounded(t *testing.T) {
	u := production.NewUploader(&sender{fail: errors.New("offline")})
	u.Max = 2
	for i := range 5 {
		u.Add(result(i))
	}
	if _, waiting, dropped, _ := u.Stats(); waiting != 2 || dropped != 3 {
		t.Fatalf("waiting %d, dropped %d", waiting, dropped)
	}
}
