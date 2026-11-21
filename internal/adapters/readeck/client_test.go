package readeck

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Crocmagnon/readlist-tagger/internal/domain/tagging"
)

// fakeServer checks the bearer token on every request, then delegates to api.
func fakeServer(t *testing.T, api http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("Authorization = %q", got)
		}
		api(w, r)
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL+"/", "tok", srv.Client())
}

func TestTagsListsLabelNames(t *testing.T) {
	c := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/bookmarks/labels" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
		}
		fmt.Fprint(w, `[{"name":"go","count":3},{"name":"ia","count":1}]`)
	})
	tags, err := c.Tags(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(tags, []tagging.Tag{"go", "ia"}) {
		t.Fatalf("got %v", tags)
	}
}

func TestUnreadEntriesSendsFilters(t *testing.T) {
	c := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		for k, want := range map[string]string{"is_archived": "false", "has_labels": "false", "is_loaded": "true"} {
			if got := q.Get(k); got != want {
				t.Errorf("%s = %q, want %q", k, got, want)
			}
		}
		fmt.Fprint(w, `[]`)
	})
	if _, err := c.UnreadEntries(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestUnreadEntriesPaginates(t *testing.T) {
	all := []string{"a", "b", "c"}
	c := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/bookmarks" {
			t.Errorf("unexpected request %s", r.URL)
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		page := []map[string]any{}
		for i := offset; i < min(offset+limit, len(all)); i++ {
			page = append(page, map[string]any{"id": all[i], "has_article": false, "labels": []string{}})
		}
		w.Header().Set("Total-Count", strconv.Itoa(len(all)))
		_ = json.NewEncoder(w).Encode(page)
	})
	c.pageSize = 2
	entries, err := c.UnreadEntries(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var ids []tagging.EntryID
	for _, e := range entries {
		ids = append(ids, e.ID)
	}
	if !slices.Equal(ids, []tagging.EntryID{"a", "b", "c"}) {
		t.Fatalf("got ids %v", ids)
	}
}

func TestUnreadEntriesFetchesArticleOnlyWhenPresent(t *testing.T) {
	c := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/bookmarks":
			fmt.Fprint(w, `[
				{"id":"a","title":"A","url":"https://e.x/a","site":"e.x","description":"desc a","has_article":true,"labels":[]},
				{"id":"b","title":"B","url":"https://v.x/b","site":"v.x","description":"a video","has_article":false,"labels":[]}]`)
		case "/api/bookmarks/a/article":
			fmt.Fprint(w, `<p>body</p>`)
		default:
			t.Errorf("unexpected request %s", r.URL)
			http.Error(w, "boom", http.StatusInternalServerError)
		}
	})
	entries, err := c.UnreadEntries(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []tagging.Entry{
		{ID: "a", Title: "A", URL: "https://e.x/a", Domain: "e.x", Content: "<p>body</p>"},
		{ID: "b", Title: "B", URL: "https://v.x/b", Domain: "v.x", Content: "a video"},
	}
	if len(entries) != len(want) {
		t.Fatalf("got %+v", entries)
	}
	for i, e := range entries {
		w := want[i]
		if e.ID != w.ID || e.Title != w.Title || e.URL != w.URL || e.Domain != w.Domain || e.Content != w.Content || len(e.Tags) != 0 {
			t.Errorf("entry %d = %+v, want %+v", i, e, w)
		}
	}
}

func TestUnreadEntriesMapsLabels(t *testing.T) {
	c := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"id":"a","has_article":false,"labels":["go"]}]`)
	})
	entries, err := c.UnreadEntries(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || !slices.Equal(entries[0].Tags, []tagging.Tag{"go"}) {
		t.Fatalf("got %+v", entries)
	}
}

func TestUnreadEntriesArticleErrorFails(t *testing.T) {
	c := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/bookmarks" {
			fmt.Fprint(w, `[{"id":"a","has_article":true,"labels":[]}]`)
			return
		}
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	if _, err := c.UnreadEntries(context.Background()); err == nil {
		t.Fatal("want an error")
	}
}

func TestNon2xxIsError(t *testing.T) {
	c := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
	_, err := c.Tags(context.Background())
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("err = %v", err)
	}
}

func TestAddTagsPatchesAddLabels(t *testing.T) {
	c := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.Method != http.MethodPatch || r.URL.Path != "/api/bookmarks/a" ||
			r.Header.Get("Content-Type") != "application/json" || string(body) != `{"add_labels":["go","ia"]}` {
			t.Errorf("unexpected request %s %s %q %s", r.Method, r.URL, r.Header.Get("Content-Type"), body)
		}
		fmt.Fprint(w, `{}`)
	})
	d, err := tagging.Answer{Tags: []string{"go", "ia"}}.Validate([]tagging.Tag{"go", "ia"})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.AddTags(context.Background(), "a", d); err != nil {
		t.Fatal(err)
	}
}

func TestAddTagsRejectsEmptyDecision(t *testing.T) {
	c := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request %s", r.URL)
	})
	if err := c.AddTags(context.Background(), "a", tagging.Decision{}); err == nil {
		t.Fatal("want an error")
	}
}

// A bookmark created during the listing shifts offset pages by one: the last bookmark of a page
// comes back at the top of the next one. It must be listed (and tagged) once.
func TestUnreadEntriesSkipsRepeatsAcrossPages(t *testing.T) {
	pages := map[string]string{
		"0": `[{"id":"a","has_article":false},{"id":"b","has_article":false}]`,
		"2": `[{"id":"b","has_article":false},{"id":"c","has_article":false}]`,
		"4": `[]`,
	}
	c := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, pages[r.URL.Query().Get("offset")])
	})
	c.pageSize = 2
	entries, err := c.UnreadEntries(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var ids []tagging.EntryID
	for _, e := range entries {
		ids = append(ids, e.ID)
	}
	if !slices.Equal(ids, []tagging.EntryID{"a", "b", "c"}) {
		t.Fatalf("got ids %v", ids)
	}
}
