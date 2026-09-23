package judge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// CLI asks whichever coding-agent CLI is already installed.
//
// Worth having as a first-class option rather than a hack. Someone running
// trackline is by definition working with a coding agent, so the binary is
// already there and already authenticated: the judge costs them no API key, no
// account and no extra spend.
//
// The obvious objection is that this judges an agent using the same model that
// produced the work, and self-judging inflates every number it touches. That is
// why it is not the default, and why the provider is named in every verdict, so
// a reader can see what produced it.
type CLI struct {
	// Binary is the command to run, e.g. "claude" or "codex".
	Binary string
	// Timeout bounds one call.
	Timeout time.Duration
}

func (c CLI) Name() string { return "cli:" + c.Binary }

func (c CLI) Ask(ctx context.Context, system, user string) (string, error) {
	timeout := c.Timeout
	if timeout == 0 {
		timeout = 90 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// A single prompt, because these CLIs take one. The system half is folded
	// in above the user half and marked, which is the best available given the
	// interface.
	prompt := system + "\n\n---\n\n" + user

	args, lastMessage, err := invocation(c.Binary, prompt)
	if err != nil {
		return "", err
	}
	if lastMessage != "" {
		defer os.Remove(lastMessage)
	}
	cmd := exec.CommandContext(ctx, c.Binary, args...)
	// The judge must not be able to touch anything. A CLI given no working
	// directory of consequence and no tools has nothing to reach for, and an
	// empty stdin stops it waiting on input that will never arrive.
	cmd.Dir = os.TempDir()
	cmd.Stdin = strings.NewReader("")

	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s failed: %w (%.200s)", c.Binary, err, errb.String())
	}
	if lastMessage != "" {
		// Codex prints its progress to stdout; the answer alone is in the file.
		b, err := os.ReadFile(lastMessage)
		if err != nil {
			return "", fmt.Errorf("%s gave no final answer: %w", c.Binary, err)
		}
		return string(b), nil
	}
	return out.String(), nil
}

// HTTP asks any endpoint that speaks the OpenAI chat-completions shape.
//
// One implementation covers OpenAI, Ollama, vLLM, LM Studio, OpenRouter and
// most things people actually run, including entirely local models. That
// matters for a tool whose whole subject is what an agent is allowed to do:
// someone unwilling to send their code to a third party can point this at
// localhost and lose nothing.
type HTTP struct {
	BaseURL string
	Model   string
	APIKey  string
	Timeout time.Duration
	Client  *http.Client
}

func (h HTTP) Name() string {
	host := h.BaseURL
	if host == "" {
		host = "openai"
	}
	return fmt.Sprintf("%s:%s", host, h.Model)
}

func (h HTTP) Ask(ctx context.Context, system, user string) (string, error) {
	base := h.BaseURL
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	timeout := h.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	client := h.Client
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}

	body, err := json.Marshal(map[string]any{
		"model": h.Model,
		// Deterministic as far as the API allows. A judge that answers
		// differently on a rerun makes every comparison unreadable.
		"temperature": 0,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
	})
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimSuffix(base, "/")+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("content-type", "application/json")
	if h.APIKey != "" {
		req.Header.Set("authorization", "Bearer "+h.APIKey)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", fmt.Errorf("could not read the reply from %s: %w", base, err)
	}
	if parsed.Error.Message != "" {
		return "", fmt.Errorf("%s: %s", base, parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("%s returned no answer", base)
	}
	return parsed.Choices[0].Message.Content, nil
}

// invocation says how to ask each CLI one question and get one answer.
//
// They do not share a convention. `claude -p` is print mode; `codex -p` is a
// config profile, so for as long as this passed -p to every binary, the
// documented `--binary codex` never worked. Each is now spelled out, and an
// unknown binary is refused rather than guessed at.
//
// lastMessage, when set, is a file the CLI writes its final answer to.
func invocation(binary, prompt string) (args []string, lastMessage string, err error) {
	switch filepath.Base(binary) {
	case "claude":
		return []string{"-p", prompt}, "", nil
	case "codex":
		f, err := os.CreateTemp("", "trackline-judge-*.txt")
		if err != nil {
			return nil, "", err
		}
		f.Close()
		// Read-only and ephemeral: a judge that can write, or that leaves a
		// session behind in the user's history, is not observing.
		return []string{"exec", "--skip-git-repo-check", "--sandbox", "read-only",
			"--ephemeral", "--color", "never", "-o", f.Name(), prompt}, f.Name(), nil
	case "cursor-agent", "agent":
		// Plan mode reads and answers; it does not edit.
		return []string{"-p", "--mode", "plan", "--output-format", "text", prompt}, "", nil
	}
	return nil, "", fmt.Errorf("do not know how to ask %q for one answer; supported: claude, codex, cursor-agent", binary)
}
