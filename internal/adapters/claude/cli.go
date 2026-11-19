// Package claude classifies articles with the Claude Code CLI in print mode.
package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/Crocmagnon/wallabag-tagger/internal/domain/tagging"
)

// CLI runs `claude -p` with no tools and a JSON Schema for the output.
type CLI struct{ Bin, Model, Effort string }

var _ tagging.Classifier = CLI{}

func (c CLI) Classify(ctx context.Context, system, user, schema string) (tagging.Result, error) {
	args := []string{"-p", "--tools", "", "--output-format", "json", "--json-schema", schema,
		"--system-prompt", system, "--no-session-persistence", "--disable-slash-commands"}
	if c.Model != "" {
		args = append(args, "--model", c.Model)
	}
	if c.Effort != "" {
		args = append(args, "--effort", c.Effort)
	}
	cmd := exec.CommandContext(ctx, c.Bin, args...)
	cmd.Stdin = strings.NewReader(user)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return tagging.Result{}, fmt.Errorf("%w: %v: %s", tagging.ErrClassifierUnavailable, err, truncate(stderr.String(), 500))
	}
	return parseOutput(stdout.Bytes())
}

func parseOutput(out []byte) (tagging.Result, error) {
	var o struct {
		IsError          bool            `json:"is_error"`
		Result           string          `json:"result"`
		StructuredOutput *tagging.Answer `json:"structured_output"`
		TotalCostUSD     float64         `json:"total_cost_usd"`
		DurationMS       int64           `json:"duration_ms"`
	}
	if err := json.Unmarshal(out, &o); err != nil {
		return tagging.Result{}, fmt.Errorf("decode claude output: %w", err)
	}
	if o.IsError {
		return tagging.Result{}, fmt.Errorf("%w: %s", tagging.ErrClassifierUnavailable, truncate(o.Result, 500))
	}
	if o.StructuredOutput == nil {
		return tagging.Result{}, errors.New("claude output has no structured_output: " + truncate(o.Result, 200))
	}
	return tagging.Result{
		Answer:   *o.StructuredOutput,
		CostUSD:  o.TotalCostUSD,
		Duration: time.Duration(o.DurationMS) * time.Millisecond,
	}, nil
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
