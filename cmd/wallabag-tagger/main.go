// Command wallabag-tagger tags untagged unread Wallabag articles by theme with Claude.
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
	"github.com/Crocmagnon/readlist-tagger/internal/adapters/wallabag"
	"github.com/Crocmagnon/readlist-tagger/internal/domain/tagging"
)

func main() {
	os.Exit(run())
}

func run() int {
	env := map[string]string{}
	var missing []string
	for _, k := range []string{"WALLABAG_URL", "WALLABAG_CLIENT_ID", "WALLABAG_CLIENT_SECRET", "WALLABAG_USERNAME", "WALLABAG_PASSWORD"} {
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
	wb := wallabag.New(env["WALLABAG_URL"], wallabag.Credentials{
		ClientID: env["WALLABAG_CLIENT_ID"], ClientSecret: env["WALLABAG_CLIENT_SECRET"],
		Username: env["WALLABAG_USERNAME"], Password: env["WALLABAG_PASSWORD"],
	}, &http.Client{Timeout: 60 * time.Second})
	if err := wb.Login(ctx); err != nil {
		log.Error("wallabag login", "error", err)
		return 1
	}

	stateDir := cmp.Or(os.Getenv("STATE_DIRECTORY"), ".")
	failures, err := failurestore.Load(filepath.Join(stateDir, "failures.json"))
	if err != nil {
		log.Error("load failures", "error", err)
		return 1
	}

	cl := claude.CLI{Bin: cmp.Or(os.Getenv("CLAUDE_BIN"), "claude"), Model: "opus", Effort: "low", Timeout: 3 * time.Minute}
	if _, err := tagging.Run(ctx, wb, cl, failures, log); err != nil {
		log.Error("run", "error", err)
		return 1
	}
	return 0
}
