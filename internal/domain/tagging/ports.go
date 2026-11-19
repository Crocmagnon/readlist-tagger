// Package tagging holds the business rules of wallabag-tagger: which articles to tag, which tags
// are allowed, and the ports it needs from the outside world.
package tagging

import (
	"context"
	"errors"
	"time"
)

// Entry is an unread article as seen by the tagger.
type Entry struct {
	ID      int
	Title   string
	URL     string
	Domain  string
	Content string // HTML as stored by Wallabag
	Tags    []string
}

// Articles is the read-it-later service holding the entries.
type Articles interface {
	Tags(ctx context.Context) ([]string, error)
	UnreadEntries(ctx context.Context) ([]Entry, error)
	AddTags(ctx context.Context, id int, tags []string) error
}

// Answer is the model's choice for one article.
type Answer struct {
	Tags   []string `json:"tags"`
	NewTag string   `json:"new_tag,omitempty"`
}

// Result is a classification and what it cost.
type Result struct {
	Answer   Answer
	CostUSD  float64
	Duration time.Duration
}

// Classifier asks a model to pick tags.
type Classifier interface {
	Classify(ctx context.Context, system, user, schema string) (Result, error)
}

// ErrClassifierUnavailable is wrapped by a Classifier when the model cannot be reached at all
// (auth, usage limit, CLI crash); Run stops on it.
var ErrClassifierUnavailable = errors.New("classifier unavailable")

// Failures counts failed attempts per entry id.
type Failures interface {
	Count(id int) int
	Record(id int)
	Clear(id int)
	Save() error
}
