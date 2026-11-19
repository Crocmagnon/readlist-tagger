package tagging

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
)

// Seed is a tag defined in code, with the description that guides the model.
type Seed struct {
	Name        Tag
	Description string
}

// Seeds were derived by hand from the existing articles; see the design doc.
var Seeds = []Seed{
	{"ia", "LLMs, agents, AI, and criticism of the AI hype"},
	{"go", "The Go language, its internals, concurrency, testing in Go"},
	{"bdd", "Postgres, SQL, data storage"},
	{"methodes", "Management, leadership, feedback, design docs/RFCs, estimation, agile, career"},
	{"ingenierie", "Language-agnostic software craft: architecture, testing, distributed systems, tech choices"},
	{"langages", "Other languages: Rust, Python/Django, Zig, C, JS/CSS"},
	{"systeme", "Linux, Nix, git, terminal, filesystems, self-hosting"},
	{"reseau", "The Internet, DNS, email, network governance"},
	{"securite", "Security, cryptography, privacy"},
	{"societe", "Tech and society: digital sovereignty, enshittification, environmental footprint"},
	{"histoire", "History and culture of computing"},
	{"hors-tech", "Personal finance, cooking, health, fiction, literature"},
	{"pas-a-lire", "Not long-form reading material: code repositories, videos, podcasts, tool or product homepages, tweets"},
}

const newTagPattern = `^[a-z0-9-]{2,20}$`

var newTagRe = regexp.MustCompile(newTagPattern)

// Decision is a validated, non-empty set of tags for one article. Only Answer.Validate builds one.
type Decision struct {
	tags    []Tag
	created Tag
}

// Tags returns the tags to apply.
func (d Decision) Tags() []Tag { return slices.Clone(d.tags) }

// Created returns the tag this decision creates, if any.
func (d Decision) Created() (Tag, bool) { return d.created, d.created != "" }

// Existing returns the seed tags and the given tags, deduplicated and sorted.
func Existing(wallabagTags []Tag) []Tag {
	all := slices.Clone(wallabagTags)
	for _, s := range Seeds {
		all = append(all, s.Name)
	}
	slices.Sort(all)
	return slices.Compact(all)
}

// Validate checks the answer against the existing tags. A new tag equal to an existing one is
// folded into the existing tags; duplicates are removed.
func (a Answer) Validate(existing []Tag) (Decision, error) {
	var d Decision
	for _, s := range a.Tags {
		t := Tag(s)
		if !slices.Contains(existing, t) {
			return Decision{}, fmt.Errorf("unknown tag %q", s)
		}
		if !slices.Contains(d.tags, t) {
			d.tags = append(d.tags, t)
		}
	}
	if a.NewTag != "" {
		t := Tag(a.NewTag)
		switch {
		case slices.Contains(existing, t):
			if !slices.Contains(d.tags, t) {
				d.tags = append(d.tags, t)
			}
		case !newTagRe.MatchString(a.NewTag):
			return Decision{}, fmt.Errorf("invalid new tag %q", a.NewTag)
		default:
			d.tags = append(d.tags, t)
			d.created = t
		}
	}
	if len(d.tags) == 0 {
		return Decision{}, errors.New("no tag")
	}
	return d, nil
}

// Schema returns the JSON Schema the classifier output must follow.
func Schema(existing []Tag) string {
	s := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"tags":    map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": existing}},
			"new_tag": map[string]any{"type": "string", "pattern": newTagPattern},
		},
		"required":             []string{"tags"},
		"additionalProperties": false,
	}
	b, err := json.Marshal(s)
	if err != nil {
		panic(err) // only maps, strings and slices: cannot fail
	}
	return string(b)
}
