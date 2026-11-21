package tagging

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
)

// MaxAttempts is how many failed attempts an entry gets before it is skipped.
const MaxAttempts = 3

// Summary counts what a run did.
type Summary struct {
	Examined, Tagged, Failed, Skipped int
	CostUSD                           float64
}

// ErrIncomplete means some entries were not tagged in this run.
var ErrIncomplete = errors.New("some entries were not tagged")

// Run tags every unread entry that has no tag yet.
func Run(ctx context.Context, articles Articles, cl Classifier, f Failures, log *slog.Logger) (s Summary, err error) {
	serviceTags, err := articles.Tags(ctx)
	if err != nil {
		return s, fmt.Errorf("list tags: %w", err)
	}
	entries, err := articles.UnreadEntries(ctx)
	if err != nil {
		return s, fmt.Errorf("list unread entries: %w", err)
	}
	existing := Existing(serviceTags)

	defer func() {
		if saveErr := f.Save(); saveErr != nil {
			err = errors.Join(err, fmt.Errorf("save failures: %w", saveErr))
		}
		if s.Examined > 0 || s.Skipped > 0 {
			log.Info("summary", "examined", s.Examined, "tagged", s.Tagged, "failed", s.Failed,
				"skipped", s.Skipped, "cost_usd", s.CostUSD)
		}
		if err == nil && s.Failed+s.Skipped > 0 {
			err = ErrIncomplete
		}
	}()

	for _, e := range entries {
		if len(e.Tags) > 0 {
			continue
		}
		if f.Count(e.ID) >= MaxAttempts {
			s.Skipped++
			log.Warn("skipped", "entry_id", e.ID, "title", e.Title, "attempts", f.Count(e.ID))
			continue
		}
		s.Examined++
		res, err := cl.Classify(ctx, SystemPrompt(existing), UserPrompt(e), Schema(existing))
		if errors.Is(err, ErrClassifierUnavailable) {
			log.Error("classifier unavailable, stopping", "entry_id", e.ID, "error", err)
			return s, err
		}
		s.CostUSD += res.CostUSD
		var d Decision
		if err == nil {
			d, err = res.Answer.Validate(existing)
		}
		if err == nil {
			err = articles.AddTags(ctx, e.ID, d)
		}
		if err != nil {
			s.Failed++
			f.Record(e.ID)
			log.Error("failed", "entry_id", e.ID, "title", e.Title, "error", err)
			continue
		}
		s.Tagged++
		f.Clear(e.ID)
		log.Info("tagged", "entry_id", e.ID, "title", e.Title, "tags", d.Tags(),
			"duration", res.Duration, "cost_usd", res.CostUSD)
		if t, ok := d.Created(); ok {
			log.Info("NEW TAG "+string(t), "entry_id", e.ID, "title", e.Title)
			existing = Existing(append(existing, t))
		}
	}
	return s, nil
}
