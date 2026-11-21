// Package readeck talks to the Readeck HTTP API.
package readeck

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/Crocmagnon/readlist-tagger/internal/domain/tagging"
)

// Client is a Readeck API client authenticated with an API token.
type Client struct {
	baseURL  string
	token    string
	hc       *http.Client
	pageSize int
}

var _ tagging.Articles = (*Client)(nil)

func New(baseURL, token string, hc *http.Client) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), token: token, hc: hc, pageSize: 100}
}

func (c *Client) Tags(ctx context.Context) ([]tagging.Tag, error) {
	var labels []struct {
		Name string `json:"name"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/api/bookmarks/labels", nil, &labels); err != nil {
		return nil, err
	}
	tags := make([]tagging.Tag, len(labels))
	for i, l := range labels {
		tags[i] = tagging.Tag(l.Name)
	}
	return tags, nil
}

type bookmark struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	URL         string   `json:"url"`
	Site        string   `json:"site"`
	Description string   `json:"description"`
	HasArticle  bool     `json:"has_article"`
	Labels      []string `json:"labels"`
}

// UnreadEntries lists unarchived, unlabeled bookmarks whose extraction is done. Bookmarks still
// loading are left for a later run.
func (c *Client) UnreadEntries(ctx context.Context) ([]tagging.Entry, error) {
	var entries []tagging.Entry
	// Readeck sorts by creation date, newest first: a bookmark created while we page through
	// shifts the pages by one and repeats a bookmark across two of them.
	seen := map[string]bool{}
	for offset := 0; ; offset += c.pageSize {
		q := url.Values{
			"is_archived": {"false"}, "has_labels": {"false"}, "is_loaded": {"true"},
			"limit": {strconv.Itoa(c.pageSize)}, "offset": {strconv.Itoa(offset)},
		}
		var page []bookmark
		if err := c.doJSON(ctx, http.MethodGet, "/api/bookmarks?"+q.Encode(), nil, &page); err != nil {
			return nil, err
		}
		for _, b := range page {
			if seen[b.ID] {
				continue
			}
			seen[b.ID] = true
			e, err := c.entry(ctx, b)
			if err != nil {
				return nil, err
			}
			entries = append(entries, e)
		}
		if len(page) < c.pageSize {
			return entries, nil
		}
	}
}

func (c *Client) entry(ctx context.Context, b bookmark) (tagging.Entry, error) {
	e := tagging.Entry{ID: tagging.EntryID(b.ID), Title: b.Title, URL: b.URL, Domain: b.Site, Content: b.Description}
	for _, l := range b.Labels {
		e.Tags = append(e.Tags, tagging.Tag(l))
	}
	// Without an article, Readeck 0.22 answers /article with a 500: never ask for it.
	if b.HasArticle {
		body, err := c.do(ctx, http.MethodGet, "/api/bookmarks/"+url.PathEscape(b.ID)+"/article", nil)
		if err != nil {
			return tagging.Entry{}, err
		}
		e.Content = string(body)
	}
	return e, nil
}

func (c *Client) AddTags(ctx context.Context, id tagging.EntryID, d tagging.Decision) error {
	tags := d.Tags()
	if len(tags) == 0 {
		return errors.New("add tags: empty decision")
	}
	labels := make([]string, len(tags))
	for i, t := range tags {
		labels[i] = string(t)
	}
	return c.doJSON(ctx, http.MethodPatch, "/api/bookmarks/"+url.PathEscape(string(id)),
		map[string][]string{"add_labels": labels}, nil)
}

// doJSON sends in as JSON (if not nil) and decodes the JSON response into out (if not nil).
func (c *Client) doJSON(ctx context.Context, method, path string, in, out any) error {
	var body []byte
	if in != nil {
		var err error
		if body, err = json.Marshal(in); err != nil {
			return err
		}
	}
	resp, err := c.do(ctx, method, path, body)
	if err != nil || out == nil {
		return err
	}
	if err := json.Unmarshal(resp, out); err != nil {
		return fmt.Errorf("%s %s: decode: %w", method, path, err)
	}
	return nil
}

// do sends body (as JSON, if not nil) and returns the response body of a 2xx answer.
func (c *Client) do(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, r)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		return nil, fmt.Errorf("%s %s: status %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", method, path, err)
	}
	return b, nil
}
