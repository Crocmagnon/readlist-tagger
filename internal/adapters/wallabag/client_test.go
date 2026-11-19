package wallabag

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/Crocmagnon/wallabag-tagger/internal/domain/tagging"
)

// fakeServer serves the token endpoint and delegates /api/ requests to api after checking the bearer.
func fakeServer(t *testing.T, api http.HandlerFunc) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /oauth/v2/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		want := map[string]string{
			"grant_type": "password", "client_id": "cid", "client_secret": "csecret",
			"username": "user", "password": "pass",
		}
		for k, v := range want {
			if got := r.PostForm.Get(k); got != v {
				t.Errorf("token form %s = %q, want %q", k, got, v)
			}
		}
		fmt.Fprint(w, `{"access_token":"tok","expires_in":3600}`)
	})
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("Authorization = %q", got)
		}
		api(w, r)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func login(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	c := New(srv.URL, Credentials{ClientID: "cid", ClientSecret: "csecret", Username: "user", Password: "pass"}, srv.Client())
	if err := c.Login(context.Background()); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestLoginSendsPasswordGrant(t *testing.T) {
	srv := fakeServer(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `[]`) })
	c := login(t, srv)
	if _, err := c.Tags(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestUnreadEntriesFetchesAllPages(t *testing.T) {
	srv := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/entries.json" || r.URL.Query().Get("archive") != "0" {
			t.Errorf("unexpected request %s", r.URL)
		}
		n := 100
		if r.URL.Query().Get("page") == "2" {
			n = 1
		}
		items := make([]map[string]any, n)
		for i := range items {
			items[i] = map[string]any{"id": i, "title": "t", "tags": []any{}}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"pages": 2, "_embedded": map[string]any{"items": items}})
	})
	entries, err := login(t, srv).UnreadEntries(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 101 {
		t.Fatalf("got %d entries, want 101", len(entries))
	}
}

func TestUnreadEntriesMapsFields(t *testing.T) {
	srv := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"pages":1,"_embedded":{"items":[{"id":7,"title":"T","url":"https://e.x/a",
			"domain_name":"e.x","content":"<p>c</p>","tags":[{"id":1,"label":"go","slug":"go"}]}]}}`)
	})
	entries, err := login(t, srv).UnreadEntries(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := tagging.Entry{ID: 7, Title: "T", URL: "https://e.x/a", Domain: "e.x", Content: "<p>c</p>", Tags: []tagging.Tag{"go"}}
	if len(entries) != 1 || entries[0].ID != want.ID || entries[0].Title != want.Title || entries[0].URL != want.URL ||
		entries[0].Domain != want.Domain || entries[0].Content != want.Content || !slices.Equal(entries[0].Tags, want.Tags) {
		t.Fatalf("got %+v, want %+v", entries, want)
	}
}

func TestTagsReturnsLabels(t *testing.T) {
	srv := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"id":1,"label":"ia","slug":"ia"}]`)
	})
	tags, err := login(t, srv).Tags(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(tags, []tagging.Tag{"ia"}) {
		t.Fatalf("got %v", tags)
	}
}

func TestAddTagsPostsCommaJoined(t *testing.T) {
	called := false
	srv := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
		_ = r.ParseForm()
		if r.Method != http.MethodPost || r.URL.Path != "/api/entries/42/tags.json" || r.PostForm.Get("tags") != "go,bdd" {
			t.Errorf("got %s %s tags=%q", r.Method, r.URL.Path, r.PostForm.Get("tags"))
		}
		fmt.Fprint(w, `{}`)
	})
	d, err := tagging.Answer{Tags: []string{"go", "bdd"}}.Validate(tagging.Existing(nil))
	if err != nil {
		t.Fatal(err)
	}
	if err := login(t, srv).AddTags(context.Background(), 42, d); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("API not called")
	}
}

func TestNon2xxIsError(t *testing.T) {
	srv := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	_, err := login(t, srv).Tags(context.Background())
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("err = %v, want mention of 500", err)
	}
}

func TestAddTagsRejectsEmptyDecision(t *testing.T) {
	srv := fakeServer(t, func(w http.ResponseWriter, r *http.Request) { t.Error("API must not be called") })
	if err := login(t, srv).AddTags(context.Background(), 42, tagging.Decision{}); err == nil {
		t.Fatal("empty decision accepted")
	}
}
