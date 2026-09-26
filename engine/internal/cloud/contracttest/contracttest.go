// Package contracttest checks values against the upload contracts in
// docs/contract, for tests on both sides of the wire.
package contracttest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Load reads a contract by file name from docs/contract.
func Load(name string) (map[string]any, error) {
	_, here, _, _ := runtime.Caller(0)
	b, err := os.ReadFile(filepath.Join(filepath.Dir(here), "..", "..", "..", "..", "docs", "contract", name))
	if err != nil {
		return nil, err
	}
	var s map[string]any
	return s, json.Unmarshal(b, &s)
}

// validate checks a value against the subset of JSON Schema the contract uses:
// type, const, enum, closed objects, required, arrays and string lengths. Small
// enough to read, so nobody has to trust a dependency to know what "valid" means.
func Validate(root, s map[string]any, v any, at string) []string {
	if ref, ok := s["$ref"].(string); ok {
		def := root["$defs"].(map[string]any)[strings.TrimPrefix(ref, "#/$defs/")].(map[string]any)
		return Validate(root, def, v, at)
	}
	var errs []string
	if c, ok := s["const"]; ok && fmt.Sprint(c) != fmt.Sprint(v) {
		errs = append(errs, fmt.Sprintf("%s: %v is not %v", at, v, c))
	}
	if e, ok := s["enum"].([]any); ok {
		found := false
		for _, x := range e {
			if x == v {
				found = true
			}
		}
		if !found {
			errs = append(errs, fmt.Sprintf("%s: %v not in %v", at, v, e))
		}
	}
	switch x := v.(type) {
	case map[string]any:
		props, _ := s["properties"].(map[string]any)
		for k, val := range x {
			ps, ok := props[k].(map[string]any)
			if !ok {
				errs = append(errs, fmt.Sprintf("%s: field %q is not in the contract", at, k))
				continue
			}
			errs = append(errs, Validate(root, ps, val, at+"."+k)...)
		}
		if req, ok := s["required"].([]any); ok {
			for _, r := range req {
				if _, ok := x[r.(string)]; !ok {
					errs = append(errs, fmt.Sprintf("%s: missing %q", at, r))
				}
			}
		}
	case []any:
		if m, ok := s["maxItems"].(float64); ok && len(x) > int(m) {
			errs = append(errs, fmt.Sprintf("%s: %d items, limit %v", at, len(x), m))
		}
		items, _ := s["items"].(map[string]any)
		for i, it := range x {
			errs = append(errs, Validate(root, items, it, fmt.Sprintf("%s[%d]", at, i))...)
		}
	case string:
		if m, ok := s["maxLength"].(float64); ok && len(x) > int(m) {
			errs = append(errs, fmt.Sprintf("%s: %d bytes, limit %v", at, len(x), m))
		}
	}
	return errs
}
