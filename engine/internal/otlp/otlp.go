// Package otlp reads OpenTelemetry trace exports into spans.
//
// Both encodings OTLP/HTTP allows: protobuf, which is what every SDK sends by
// default and the only one the Python exporter can send, and JSON, which a
// Collector can forward. A receiver that took JSON alone would miss most real
// deployments without an error, because the SDK would simply be configured
// for something else.
//
// The protobuf side is decoded by hand rather than through the generated
// types. The engine carries no dependencies, and the part of the trace schema
// read here is a few dozen fields of a wire format that has not changed since
// OTLP 1.0. Field numbers are from opentelemetry-proto, and the tests run
// against bytes the real Python exporter produced.
package otlp

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/yerinsabraham/trackline/engine/internal/adapter/otel"
)

// Decode reads one export request. contentType is the request's header;
// anything mentioning json is read as JSON and everything else as protobuf,
// which is the OTLP default.
func Decode(body []byte, contentType string) ([]otel.Span, error) {
	if strings.Contains(strings.ToLower(contentType), "json") {
		return decodeJSON(body)
	}
	return decodeProto(body)
}

// ---- protobuf ----

type field struct {
	num   int
	wire  int
	u64   uint64 // varint, fixed64, fixed32
	bytes []byte // length-delimited
}

var errTruncated = errors.New("truncated protobuf")

// fields walks one message's top level. Unknown fields are returned like any
// other and ignored by the caller, which is what makes this tolerant of a
// newer schema adding fields.
func fields(b []byte) ([]field, error) {
	var out []field
	for len(b) > 0 {
		key, n := binary.Uvarint(b)
		if n <= 0 {
			return nil, errTruncated
		}
		b = b[n:]
		f := field{num: int(key >> 3), wire: int(key & 7)}
		switch f.wire {
		case 0:
			v, n := binary.Uvarint(b)
			if n <= 0 {
				return nil, errTruncated
			}
			f.u64, b = v, b[n:]
		case 1:
			if len(b) < 8 {
				return nil, errTruncated
			}
			f.u64, b = binary.LittleEndian.Uint64(b), b[8:]
		case 2:
			l, n := binary.Uvarint(b)
			if n <= 0 || uint64(len(b)-n) < l {
				return nil, errTruncated
			}
			f.bytes, b = b[n:n+int(l)], b[n+int(l):]
		case 5:
			if len(b) < 4 {
				return nil, errTruncated
			}
			f.u64, b = uint64(binary.LittleEndian.Uint32(b)), b[4:]
		default:
			return nil, fmt.Errorf("unsupported protobuf wire type %d", f.wire)
		}
		out = append(out, f)
	}
	return out, nil
}

func decodeProto(body []byte) ([]otel.Span, error) {
	req, err := fields(body)
	if err != nil {
		return nil, err
	}
	var out []otel.Span
	for _, rs := range req {
		if rs.num != 1 || rs.wire != 2 { // resource_spans
			continue
		}
		rsf, err := fields(rs.bytes)
		if err != nil {
			return nil, err
		}
		service := ""
		for _, f := range rsf {
			if f.num == 1 && f.wire == 2 { // resource
				attrs, err := protoAttributes(f.bytes, 1)
				if err != nil {
					return nil, err
				}
				service = attrs["service.name"]
			}
		}
		for _, f := range rsf {
			if f.num != 2 || f.wire != 2 { // scope_spans
				continue
			}
			ssf, err := fields(f.bytes)
			if err != nil {
				return nil, err
			}
			scope := ""
			for _, s := range ssf {
				if s.num == 1 && s.wire == 2 {
					sf, _ := fields(s.bytes)
					for _, x := range sf {
						if x.num == 1 && x.wire == 2 {
							scope = string(x.bytes)
						}
					}
				}
			}
			for _, s := range ssf {
				if s.num != 2 || s.wire != 2 { // spans
					continue
				}
				span, err := protoSpan(s.bytes)
				if err != nil {
					return nil, err
				}
				span.Service, span.Scope = service, scope
				out = append(out, span)
			}
		}
	}
	return out, nil
}

func protoSpan(b []byte) (otel.Span, error) {
	fs, err := fields(b)
	if err != nil {
		return otel.Span{}, err
	}
	s := otel.Span{Attributes: map[string]string{}}
	for _, f := range fs {
		switch {
		case f.num == 1 && f.wire == 2:
			s.TraceID = hex.EncodeToString(f.bytes)
		case f.num == 2 && f.wire == 2:
			s.SpanID = hex.EncodeToString(f.bytes)
		case f.num == 4 && f.wire == 2:
			s.ParentSpanID = hex.EncodeToString(f.bytes)
		case f.num == 5 && f.wire == 2:
			s.Name = string(f.bytes)
		case f.num == 7 && f.wire == 1:
			s.Start = nanos(f.u64)
		case f.num == 8 && f.wire == 1:
			s.End = nanos(f.u64)
		case f.num == 9 && f.wire == 2:
			k, v, err := protoKeyValue(f.bytes)
			if err != nil {
				return s, err
			}
			s.Attributes[k] = v
		case f.num == 15 && f.wire == 2:
			st, _ := fields(f.bytes)
			for _, x := range st {
				switch {
				case x.num == 2 && x.wire == 2:
					s.StatusMessage = string(x.bytes)
				case x.num == 3 && x.wire == 0:
					s.Status = statusName(int(x.u64))
				}
			}
		}
	}
	return s, nil
}

// protoAttributes reads the KeyValue list at field num of a message.
func protoAttributes(b []byte, num int) (map[string]string, error) {
	fs, err := fields(b)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, f := range fs {
		if f.num == num && f.wire == 2 {
			k, v, err := protoKeyValue(f.bytes)
			if err != nil {
				return nil, err
			}
			out[k] = v
		}
	}
	return out, nil
}

func protoKeyValue(b []byte) (string, string, error) {
	fs, err := fields(b)
	if err != nil {
		return "", "", err
	}
	var key string
	var val any
	for _, f := range fs {
		switch {
		case f.num == 1 && f.wire == 2:
			key = string(f.bytes)
		case f.num == 2 && f.wire == 2:
			val, err = protoAnyValue(f.bytes)
			if err != nil {
				return "", "", err
			}
		}
	}
	return key, render(val), nil
}

func protoAnyValue(b []byte) (any, error) {
	fs, err := fields(b)
	if err != nil {
		return nil, err
	}
	for _, f := range fs {
		switch f.num {
		case 1:
			return string(f.bytes), nil
		case 2:
			return f.u64 != 0, nil
		case 3:
			return int64(f.u64), nil
		case 4:
			return math.Float64frombits(f.u64), nil
		case 5:
			vs, err := fields(f.bytes)
			if err != nil {
				return nil, err
			}
			var arr []any
			for _, v := range vs {
				if v.num == 1 && v.wire == 2 {
					x, err := protoAnyValue(v.bytes)
					if err != nil {
						return nil, err
					}
					arr = append(arr, x)
				}
			}
			return arr, nil
		case 7:
			return hex.EncodeToString(f.bytes), nil
		}
	}
	return nil, nil
}

// ---- JSON ----

type jsonRequest struct {
	ResourceSpans []struct {
		Resource struct {
			Attributes []jsonKV `json:"attributes"`
		} `json:"resource"`
		ScopeSpans []struct {
			Scope struct {
				Name string `json:"name"`
			} `json:"scope"`
			Spans []struct {
				TraceID      string          `json:"traceId"`
				SpanID       string          `json:"spanId"`
				ParentSpanID string          `json:"parentSpanId"`
				Name         string          `json:"name"`
				Start        json.RawMessage `json:"startTimeUnixNano"`
				End          json.RawMessage `json:"endTimeUnixNano"`
				Attributes   []jsonKV        `json:"attributes"`
				Status       struct {
					Code    json.RawMessage `json:"code"`
					Message string          `json:"message"`
				} `json:"status"`
			} `json:"spans"`
		} `json:"scopeSpans"`
	} `json:"resourceSpans"`
}

type jsonKV struct {
	Key   string    `json:"key"`
	Value jsonValue `json:"value"`
}

type jsonValue struct {
	StringValue *string         `json:"stringValue"`
	BoolValue   *bool           `json:"boolValue"`
	IntValue    json.RawMessage `json:"intValue"`
	DoubleValue *float64        `json:"doubleValue"`
	ArrayValue  *struct {
		Values []jsonValue `json:"values"`
	} `json:"arrayValue"`
}

func (v jsonValue) native() any {
	switch {
	case v.StringValue != nil:
		return *v.StringValue
	case v.BoolValue != nil:
		return *v.BoolValue
	case len(v.IntValue) > 0:
		// int64 is a string in the JSON mapping, but some producers send a number.
		n, _ := strconv.ParseInt(strings.Trim(string(v.IntValue), `"`), 10, 64)
		return n
	case v.DoubleValue != nil:
		return *v.DoubleValue
	case v.ArrayValue != nil:
		var arr []any
		for _, x := range v.ArrayValue.Values {
			arr = append(arr, x.native())
		}
		return arr
	}
	return nil
}

func decodeJSON(body []byte) ([]otel.Span, error) {
	var req jsonRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	var out []otel.Span
	for _, rs := range req.ResourceSpans {
		service := ""
		for _, kv := range rs.Resource.Attributes {
			if kv.Key == "service.name" {
				service = render(kv.Value.native())
			}
		}
		for _, ss := range rs.ScopeSpans {
			for _, sp := range ss.Spans {
				s := otel.Span{
					Name: sp.Name, TraceID: sp.TraceID, SpanID: sp.SpanID, ParentSpanID: sp.ParentSpanID,
					Start: nanos(jsonUint(sp.Start)), End: nanos(jsonUint(sp.End)),
					Service: service, Scope: ss.Scope.Name,
					StatusMessage: sp.Status.Message, Status: jsonStatus(sp.Status.Code),
					Attributes: map[string]string{},
				}
				for _, kv := range sp.Attributes {
					s.Attributes[kv.Key] = render(kv.Value.native())
				}
				out = append(out, s)
			}
		}
	}
	return out, nil
}

func jsonUint(raw json.RawMessage) uint64 {
	n, _ := strconv.ParseUint(strings.Trim(string(raw), `"`), 10, 64)
	return n
}

// jsonStatus accepts the code as a number or as its enum name, since both
// appear in the wild.
func jsonStatus(raw json.RawMessage) string {
	s := strings.Trim(string(raw), `"`)
	switch s {
	case "2", "STATUS_CODE_ERROR":
		return "error"
	case "1", "STATUS_CODE_OK":
		return "ok"
	}
	return ""
}

// ---- shared ----

func statusName(code int) string {
	switch code {
	case 2:
		return "error"
	case 1:
		return "ok"
	}
	return ""
}

func nanos(n uint64) time.Time {
	if n == 0 {
		return time.Time{}
	}
	return time.Unix(0, int64(n)).UTC()
}

// render flattens an attribute value to the string the adapter reads. Arrays
// become JSON, so ["tool_calls"] stays recognisable, where Phase 0's capture
// had the Python tuple repr ('tool_calls',).
func render(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		return strconv.FormatBool(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64)
	default:
		b, _ := json.Marshal(x)
		return string(b)
	}
}
