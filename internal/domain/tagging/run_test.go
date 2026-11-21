package tagging

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"testing"
)

type fakeArticles struct {
	tags    []Tag
	entries []Entry
	added   map[EntryID][]Tag
}

func (f *fakeArticles) Tags(context.Context) ([]Tag, error)            { return f.tags, nil }
func (f *fakeArticles) UnreadEntries(context.Context) ([]Entry, error) { return f.entries, nil }
func (f *fakeArticles) AddTags(_ context.Context, id EntryID, d Decision) error {
	if f.added == nil {
		f.added = map[EntryID][]Tag{}
	}
	f.added[id] = d.Tags()
	return nil
}

// fakeClassifier returns its answers in order and records the schemas it saw.
type fakeClassifier struct {
	answers []Answer
	errs    []error
	schemas []string
}

func (f *fakeClassifier) Classify(_ context.Context, _, _, schema string) (Result, error) {
	i := len(f.schemas)
	f.schemas = append(f.schemas, schema)
	if i < len(f.errs) && f.errs[i] != nil {
		return Result{}, f.errs[i]
	}
	return Result{Answer: f.answers[i], CostUSD: 0.01}, nil
}

type fakeFailures struct {
	counts map[EntryID]int
	saved  bool
}

func newFailures() *fakeFailures             { return &fakeFailures{counts: map[EntryID]int{}} }
func (f *fakeFailures) Count(id EntryID) int { return f.counts[id] }
func (f *fakeFailures) Record(id EntryID)    { f.counts[id]++ }
func (f *fakeFailures) Clear(id EntryID)     { delete(f.counts, id) }
func (f *fakeFailures) Save() error          { f.saved = true; return nil }

func run(t *testing.T, a *fakeArticles, c *fakeClassifier, f *fakeFailures) (Summary, error, string) {
	t.Helper()
	var buf bytes.Buffer
	s, err := Run(context.Background(), a, c, f, slog.New(slog.NewTextHandler(&buf, nil)))
	return s, err, buf.String()
}

func TestRunTagsOnlyUntagged(t *testing.T) {
	a := &fakeArticles{entries: []Entry{{ID: "1", Title: "a"}, {ID: "2", Title: "b", Tags: []Tag{"go"}}}}
	c := &fakeClassifier{answers: []Answer{{Tags: []string{"bdd"}}}}
	s, err, _ := run(t, a, c, newFailures())
	if err != nil {
		t.Fatal(err)
	}
	if len(c.schemas) != 1 || !slices.Equal(a.added["1"], []Tag{"bdd"}) || len(a.added) != 1 {
		t.Fatalf("calls %d, added %v", len(c.schemas), a.added)
	}
	if s.Examined != 1 || s.Tagged != 1 {
		t.Fatalf("summary %+v", s)
	}
}

func TestRunLogsNewTagAndReusesIt(t *testing.T) {
	a := &fakeArticles{entries: []Entry{{ID: "1"}, {ID: "2"}}}
	c := &fakeClassifier{answers: []Answer{{NewTag: "electronique"}, {Tags: []string{"electronique"}}}}
	_, err, log := run(t, a, c, newFailures())
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(log, "NEW TAG electronique"); n != 1 {
		t.Fatalf("NEW TAG logged %d times:\n%s", n, log)
	}
	if !strings.Contains(c.schemas[1], `"electronique"`) || !slices.Equal(a.added["2"], []Tag{"electronique"}) {
		t.Fatalf("second call did not see the new tag: %s / %v", c.schemas[1], a.added)
	}
}

func TestRunInvalidAnswerCountsFailure(t *testing.T) {
	a := &fakeArticles{entries: []Entry{{ID: "1"}}}
	f := newFailures()
	s, err, _ := run(t, a, &fakeClassifier{answers: []Answer{{Tags: []string{"IA"}}}}, f)
	if len(a.added) != 0 || s.Failed != 1 || f.counts["1"] != 1 || !errors.Is(err, ErrIncomplete) {
		t.Fatalf("added %v, summary %+v, count %d, err %v", a.added, s, f.counts["1"], err)
	}
}

func TestRunStopsWhenClassifierUnavailable(t *testing.T) {
	a := &fakeArticles{entries: []Entry{{ID: "1"}, {ID: "2"}, {ID: "3"}}}
	c := &fakeClassifier{errs: []error{fmt.Errorf("x: %w", ErrClassifierUnavailable)}}
	f := newFailures()
	_, err, _ := run(t, a, c, f)
	if len(c.schemas) != 1 || len(f.counts) != 0 || !errors.Is(err, ErrClassifierUnavailable) || !f.saved {
		t.Fatalf("calls %d, counts %v, err %v, saved %v", len(c.schemas), f.counts, err, f.saved)
	}
}

func TestRunSkipsExhausted(t *testing.T) {
	a := &fakeArticles{entries: []Entry{{ID: "5"}}}
	f := newFailures()
	f.counts["5"] = MaxAttempts
	c := &fakeClassifier{}
	s, err, log := run(t, a, c, f)
	if len(c.schemas) != 0 || s.Skipped != 1 || err == nil || !strings.Contains(log, "skipped") || !strings.Contains(log, "entry_id=5") {
		t.Fatalf("calls %d, summary %+v, err %v, log:\n%s", len(c.schemas), s, err, log)
	}
}

func TestRunIdleLogsNothing(t *testing.T) {
	a := &fakeArticles{entries: []Entry{{ID: "1", Tags: []Tag{"go"}}}}
	_, err, log := run(t, a, &fakeClassifier{}, newFailures())
	if err != nil || log != "" {
		t.Fatalf("err %v, log %q", err, log)
	}
}

func TestRunClearsFailureOnSuccess(t *testing.T) {
	a := &fakeArticles{entries: []Entry{{ID: "1"}}}
	f := newFailures()
	f.counts["1"] = 2
	_, err, _ := run(t, a, &fakeClassifier{answers: []Answer{{Tags: []string{"go"}}}}, f)
	if err != nil || f.counts["1"] != 0 || !f.saved {
		t.Fatalf("err %v, count %d, saved %v", err, f.counts["1"], f.saved)
	}
}
