package tagging

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSystemPromptListsSeedsAndOthers(t *testing.T) {
	p := SystemPrompt(Existing([]Tag{"electronique"}))
	for _, s := range Seeds {
		if !strings.Contains(p, string(s.Name)) || !strings.Contains(p, s.Description) {
			t.Errorf("missing seed %q", s.Name)
		}
	}
	if !strings.Contains(p, "electronique") {
		t.Error("missing electronique")
	}
}

func TestUserPromptStripsHTMLAndTruncates(t *testing.T) {
	p := UserPrompt(Entry{Title: "T", Content: "<p>Hello &amp; <b>bye</b></p>"})
	if !strings.Contains(p, "Hello & bye") || strings.Contains(p, "<p>") {
		t.Errorf("not stripped: %q", p)
	}
	long := UserPrompt(Entry{Content: strings.Repeat("é", 10000)})
	if n := strings.Count(long, "é"); n > ExcerptChars {
		t.Errorf("excerpt has %d chars, want at most %d", n, ExcerptChars)
	}
	if !utf8.ValidString(long) {
		t.Error("truncation broke UTF-8")
	}
}

func TestUserPromptEmptyContent(t *testing.T) {
	p := UserPrompt(Entry{Title: "uBlacklist", Domain: "github.com", URL: "https://github.com/x/ublacklist"})
	for _, want := range []string{"uBlacklist", "github.com", "https://github.com/x/ublacklist"} {
		if !strings.Contains(p, want) {
			t.Errorf("missing %q", want)
		}
	}
}
