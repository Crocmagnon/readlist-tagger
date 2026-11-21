// Package wallabag talks to the Wallabag HTTP API.
package wallabag

import (
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

// Credentials authenticate against the Wallabag API (password grant).
type Credentials struct{ ClientID, ClientSecret, Username, Password string }

// Client is an authenticated Wallabag API client. Call Login before anything else.
type Client struct {
	baseURL string
	creds   Credentials
	hc      *http.Client
	token   string
}

var _ tagging.Articles = (*Client)(nil)

func New(baseURL string, creds Credentials, hc *http.Client) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), creds: creds, hc: hc}
}

// Login gets an access token with the password grant.
func (c *Client) Login(ctx context.Context) error {
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	err := c.do(ctx, http.MethodPost, "/oauth/v2/token", url.Values{
		"grant_type": {"password"}, "client_id": {c.creds.ClientID}, "client_secret": {c.creds.ClientSecret},
		"username": {c.creds.Username}, "password": {c.creds.Password},
	}, &tok)
	if err != nil {
		return err
	}
	c.token = tok.AccessToken
	return nil
}

func (c *Client) Tags(ctx context.Context) ([]tagging.Tag, error) {
	var tags []struct {
		Label string `json:"label"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/tags.json", nil, &tags); err != nil {
		return nil, err
	}
	labels := make([]tagging.Tag, len(tags))
	for i, t := range tags {
		labels[i] = tagging.Tag(t.Label)
	}
	return labels, nil
}

func (c *Client) UnreadEntries(ctx context.Context) ([]tagging.Entry, error) {
	var entries []tagging.Entry
	for page, pages := 1, 1; page <= pages; page++ {
		var resp struct {
			Pages    int `json:"pages"`
			Embedded struct {
				Items []struct {
					ID      int    `json:"id"`
					Title   string `json:"title"`
					URL     string `json:"url"`
					Domain  string `json:"domain_name"`
					Content string `json:"content"`
					Tags    []struct {
						Label string `json:"label"`
					} `json:"tags"`
				} `json:"items"`
			} `json:"_embedded"`
		}
		path := "/api/entries.json?archive=0&perPage=100&page=" + strconv.Itoa(page)
		if err := c.do(ctx, http.MethodGet, path, nil, &resp); err != nil {
			return nil, err
		}
		pages = resp.Pages
		for _, it := range resp.Embedded.Items {
			e := tagging.Entry{ID: tagging.EntryID(strconv.Itoa(it.ID)), Title: it.Title, URL: it.URL, Domain: it.Domain, Content: it.Content}
			for _, t := range it.Tags {
				e.Tags = append(e.Tags, tagging.Tag(t.Label))
			}
			entries = append(entries, e)
		}
	}
	return entries, nil
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
	path := fmt.Sprintf("/api/entries/%s/tags.json", id)
	return c.do(ctx, http.MethodPost, path, url.Values{"tags": {strings.Join(labels, ",")}}, nil)
}

// do sends form (if any) and decodes the JSON response into out (if any).
func (c *Client) do(ctx context.Context, method, path string, form url.Values, out any) error {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return err
	}
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		return fmt.Errorf("%s %s: status %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("%s %s: decode: %w", method, path, err)
	}
	return nil
}
