// Command readeck-tagger tags unlabeled unread Readeck bookmarks by theme with Claude.
package main

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Crocmagnon/readlist-tagger/internal/adapters/claude"
	"github.com/Crocmagnon/readlist-tagger/internal/adapters/failurestore"
	"github.com/Crocmagnon/readlist-tagger/internal/adapters/readeck"
	"github.com/Crocmagnon/readlist-tagger/internal/domain/tagging"
)

func main() {
	os.Exit(run())
}

func run() int {
	env := map[string]string{}
	var missing []string
	for _, k := range []string{"READECK_URL", "READECK_TOKEN"} {
		if env[k] = os.Getenv(k); env[k] == "" {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		fmt.Fprintln(os.Stderr, "missing environment variables:", strings.Join(missing, ", "))
		return 2
	}

	// journald timestamps every line already.
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if len(groups) == 0 && a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	}))

	ctx := context.Background()
	rd := readeck.New(env["READECK_URL"], env["READECK_TOKEN"], &http.Client{Timeout: 60 * time.Second})

	stateDir := cmp.Or(os.Getenv("STATE_DIRECTORY"), ".")
	failures, err := failurestore.Load(filepath.Join(stateDir, "failures.json"))
	if err != nil {
		log.Error("load failures", "error", err)
		return 1
	}

	cl := claude.CLI{Bin: cmp.Or(os.Getenv("CLAUDE_BIN"), "claude"), Model: "opus", Effort: "low", Timeout: 3 * time.Minute}
	if _, err := tagging.Run(ctx, rd, cl, failures, log); err != nil {
		log.Error("run", "error", err)
		return 1
	}
	return 0
}
