// Package claude classifies articles with the Claude Code CLI in print mode.
package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/Crocmagnon/readlist-tagger/internal/domain/tagging"
)

// CLI runs `claude -p` with no tools, no MCP server, no settings, and a JSON Schema for the
// output. Timeout bounds one call (0: none).
type CLI struct {
	Bin, Model, Effort string
	Timeout            time.Duration
}

var _ tagging.Classifier = CLI{}

func (c CLI) Classify(ctx context.Context, system, user, schema string) (tagging.Result, error) {
	if c.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.Timeout)
		defer cancel()
	}
	args := []string{"-p", "--tools", "", "--strict-mcp-config", "--setting-sources", "",
		"--output-format", "json", "--json-schema", schema, "--system-prompt", system,
		"--no-session-persistence", "--disable-slash-commands"}
	if c.Model != "" {
		args = append(args, "--model", c.Model)
	}
	if c.Effort != "" {
		args = append(args, "--effort", c.Effort)
	}
	cmd := exec.CommandContext(ctx, c.Bin, args...)
	// Kill the whole process group on timeout: wrappers may not exec into claude.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 5 * time.Second
	cmd.Env = allowedEnv(os.Environ())
	cmd.Stdin = strings.NewReader(user)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	runErr := cmd.Run()
	// claude reports most failures as JSON on stdout, even with a non-zero exit.
	res, err := parseOutput(stdout.Bytes())
	if runErr != nil && (err == nil || !errors.Is(err, tagging.ErrClassifierUnavailable)) {
		return res, fmt.Errorf("%w: %v: %s", tagging.ErrClassifierUnavailable, runErr,
			truncate(stderr.String()+" "+resultText(stdout.Bytes()), 500))
	}
	return res, err
}

// envAllowlist is all claude gets from our environment: never the service credentials.
var envAllowlist = []string{
	"HOME", "PATH", "TMPDIR", "LANG", "LC_ALL", "TZ", "TZDIR", "LOCALE_ARCHIVE",
	"SSL_CERT_FILE", "NIX_SSL_CERT_FILE", "CLAUDE_CODE_OAUTH_TOKEN",
}

func allowedEnv(environ []string) []string {
	var env []string
	for _, kv := range environ {
		k, _, _ := strings.Cut(kv, "=")
		if slices.Contains(envAllowlist, k) {
			env = append(env, kv)
		}
	}
	return env
}

// output is the part of `claude -p --output-format json` we use.
type output struct {
	IsError          bool            `json:"is_error"`
	Subtype          string          `json:"subtype"`
	APIErrorStatus   *int            `json:"api_error_status"`
	Result           string          `json:"result"`
	StructuredOutput *tagging.Answer `json:"structured_output"`
	TotalCostUSD     float64         `json:"total_cost_usd"`
	DurationMS       int64           `json:"duration_ms"`
}

func parseOutput(out []byte) (tagging.Result, error) {
	var o output
	if err := json.Unmarshal(out, &o); err != nil {
		return tagging.Result{}, fmt.Errorf("decode claude output: %w", err)
	}
	res := tagging.Result{CostUSD: o.TotalCostUSD, Duration: time.Duration(o.DurationMS) * time.Millisecond}
	if o.IsError {
		// An API error (auth, quota, outage) affects every article; anything else is this article's.
		if o.APIErrorStatus != nil {
			return res, fmt.Errorf("%w: API status %d: %s", tagging.ErrClassifierUnavailable, *o.APIErrorStatus, truncate(o.Result, 500))
		}
		return res, fmt.Errorf("claude error (%s): %s", o.Subtype, truncate(o.Result, 500))
	}
	if o.StructuredOutput == nil {
		return res, errors.New("claude output has no structured_output: " + truncate(o.Result, 200))
	}
	res.Answer = *o.StructuredOutput
	return res, nil
}

// resultText returns the "result" field of a JSON output, if any.
func resultText(out []byte) string {
	var o output
	_ = json.Unmarshal(out, &o)
	return o.Result
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
