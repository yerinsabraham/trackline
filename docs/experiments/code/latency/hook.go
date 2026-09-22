package main

import (
	"encoding/json"
	"io"
	"os"
	"strings"
)

type ev struct {
	ToolName  string                 `json:"tool_name"`
	ToolInput map[string]interface{} `json:"tool_input"`
}

func main() {
	raw, _ := io.ReadAll(os.Stdin)
	var e ev
	if json.Unmarshal(raw, &e) != nil {
		os.Exit(0)
	}
	blob, _ := json.Marshal(e.ToolInput)
	if strings.Contains(string(blob), "secrets.env") {
		os.Stderr.WriteString("BLOCKED: secrets.env is off limits. Use config.local.json instead.")
		os.Exit(2)
	}
	os.Exit(0)
}
