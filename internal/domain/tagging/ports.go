// Package tagging holds the business rules of readlist-tagger: which articles to tag, which tags
// are allowed, and the ports it needs from the outside world.
package tagging

import (
	"context"
	"errors"
	"time"
)

// EntryID identifies an article.
type EntryID string

// Tag is a tag label as stored by the service.
type Tag string

// Entry is an unread article as seen by the tagger.
type Entry struct {
	ID      EntryID
	Title   string
	URL     string
	Domain  string
	Content string // article HTML as stored by the service
	Tags    []Tag
}

// Articles is the read-it-later service holding the entries.
type Articles interface {
	Tags(ctx context.Context) ([]Tag, error)
	UnreadEntries(ctx context.Context) ([]Entry, error)
	AddTags(ctx context.Context, id EntryID, d Decision) error
}

// Answer is the model's raw, unvalidated choice for one article; Validate turns it into a Decision.
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
	Count(id EntryID) int
	Record(id EntryID)
	Clear(id EntryID)
	Save() error
}
