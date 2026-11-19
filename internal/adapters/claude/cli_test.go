package claude

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Crocmagnon/wallabag-tagger/internal/domain/tagging"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// fakeBin writes an executable shell script and returns its path.
func fakeBin(t *testing.T, script string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestParseOutputOK(t *testing.T) {
	r, err := parseOutput(readFixture(t, "ok.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(r.Answer.Tags, []string{"bdd"}) || r.CostUSD != 0.0084 || r.Duration != 3117*time.Millisecond {
		t.Fatalf("got %+v", r)
	}
}

func TestParseOutputIsError(t *testing.T) {
	_, err := parseOutput(readFixture(t, "auth_error.json"))
	if !errors.Is(err, tagging.ErrClassifierUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

func TestParseOutputMissingStructured(t *testing.T) {
	_, err := parseOutput([]byte(`{"type":"result","is_error":false,"result":"sorry"}`))
	if err == nil || errors.Is(err, tagging.ErrClassifierUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

func TestClassifyNonZeroExit(t *testing.T) {
	cli := CLI{Bin: fakeBin(t, "echo boom >&2\nexit 1\n")}
	_, err := cli.Classify(context.Background(), "sys", "user", "{}")
	if !errors.Is(err, tagging.ErrClassifierUnavailable) || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err = %v", err)
	}
}

func TestClassifyPassesFlagsAndStdin(t *testing.T) {
	dir := t.TempDir()
	ok, err := filepath.Abs(filepath.Join("testdata", "ok.json"))
	if err != nil {
		t.Fatal(err)
	}
	bin := fakeBin(t, `for a in "$@"; do printf '%s\n' "[$a]"; done > `+dir+`/args
cat > `+dir+`/stdin
cat `+ok+"\n")
	cli := CLI{Bin: bin, Model: "opus", Effort: "low"}
	if _, err := cli.Classify(context.Background(), "the system", "the user prompt", `{"type":"object"}`); err != nil {
		t.Fatal(err)
	}
	args, _ := os.ReadFile(filepath.Join(dir, "args"))
	for _, want := range []string{"[--tools]\n[]\n", "[--strict-mcp-config]\n", "[--setting-sources]\n[]\n", "[--json-schema]\n[{\"type\":\"object\"}]\n", "[--system-prompt]\n[the system]\n", "[-p]\n"} {
		if !strings.Contains(string(args), want) {
			t.Errorf("args missing %q in:\n%s", want, args)
		}
	}
	stdin, _ := os.ReadFile(filepath.Join(dir, "stdin"))
	if string(stdin) != "the user prompt" {
		t.Errorf("stdin = %q", stdin)
	}
}

func TestParseOutputArticleErrorIsNotUnavailable(t *testing.T) {
	r, err := parseOutput(readFixture(t, "article_error.json"))
	if err == nil || errors.Is(err, tagging.ErrClassifierUnavailable) || !strings.Contains(err.Error(), "schema") {
		t.Fatalf("err = %v", err)
	}
	if r.CostUSD != 0.02 {
		t.Fatalf("cost of the failed call lost: %+v", r)
	}
}

func TestClassifyNonZeroExitWithJSONKeepsCause(t *testing.T) {
	auth, err := filepath.Abs(filepath.Join("testdata", "auth_error.json"))
	if err != nil {
		t.Fatal(err)
	}
	cli := CLI{Bin: fakeBin(t, "cat "+auth+"\nexit 1\n")}
	_, err = cli.Classify(context.Background(), "sys", "user", "{}")
	if !errors.Is(err, tagging.ErrClassifierUnavailable) || !strings.Contains(err.Error(), "Invalid bearer token") {
		t.Fatalf("err = %v", err)
	}
}

func TestClassifyTimesOut(t *testing.T) {
	cli := CLI{Bin: fakeBin(t, "sleep 10\n"), Timeout: 200 * time.Millisecond}
	start := time.Now()
	_, err := cli.Classify(context.Background(), "sys", "user", "{}")
	if !errors.Is(err, tagging.ErrClassifierUnavailable) || time.Since(start) > 5*time.Second {
		t.Fatalf("err = %v after %s", err, time.Since(start))
	}
}
