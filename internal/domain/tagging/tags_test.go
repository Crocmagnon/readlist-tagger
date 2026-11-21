package tagging

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestExistingMergesSeedsAndServiceTags(t *testing.T) {
	got := Existing([]Tag{"electronique", "go"})
	for _, s := range Seeds {
		if !slices.Contains(got, s.Name) {
			t.Errorf("missing seed %q", s.Name)
		}
	}
	if !slices.Contains(got, Tag("electronique")) {
		t.Error("missing electronique")
	}
	n := 0
	for _, g := range got {
		if g == "go" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("go appears %d times", n)
	}
	if !slices.IsSorted(got) {
		t.Errorf("not sorted: %v", got)
	}
	if len(Seeds) != 13 {
		t.Errorf("len(Seeds) = %d, want 13", len(Seeds))
	}
}

func TestValidateAcceptsExisting(t *testing.T) {
	d, err := Answer{Tags: []string{"go", "bdd"}}.Validate(Existing(nil))
	created, ok := d.Created()
	if err != nil || !slices.Equal(d.Tags(), []Tag{"go", "bdd"}) || ok {
		t.Fatalf("got %v %q %v", d.Tags(), created, err)
	}
}

func TestValidateAcceptsNewTagAlone(t *testing.T) {
	d, err := Answer{NewTag: "electronique"}.Validate(Existing(nil))
	created, ok := d.Created()
	if err != nil || !slices.Equal(d.Tags(), []Tag{"electronique"}) || !ok || created != "electronique" {
		t.Fatalf("got %v %q %v", d.Tags(), created, err)
	}
}

func TestValidateFoldsNewTagEqualToExisting(t *testing.T) {
	d, err := Answer{Tags: []string{"go"}, NewTag: "go"}.Validate(Existing(nil))
	created, ok := d.Created()
	if err != nil || !slices.Equal(d.Tags(), []Tag{"go"}) || ok {
		t.Fatalf("got %v %q %v", d.Tags(), created, err)
	}
}

func TestValidateDedupes(t *testing.T) {
	d, err := Answer{Tags: []string{"go", "go"}}.Validate(Existing(nil))
	if err != nil || !slices.Equal(d.Tags(), []Tag{"go"}) {
		t.Fatalf("got %v %v", d.Tags(), err)
	}
}

func TestValidateRejectsUnknownTag(t *testing.T) {
	for _, tag := range []string{"IA", "méthodes"} {
		if _, err := (Answer{Tags: []string{tag}}).Validate(Existing(nil)); err == nil {
			t.Errorf("%q accepted", tag)
		}
	}
}

func TestValidateRejectsBadNewTag(t *testing.T) {
	for _, tag := range []string{"É", "a", "way-too-long-tag-name-x", "two words"} {
		if _, err := (Answer{NewTag: tag}).Validate(Existing(nil)); err == nil {
			t.Errorf("%q accepted", tag)
		}
	}
}

func TestValidateRejectsEmpty(t *testing.T) {
	if _, err := (Answer{}).Validate(Existing(nil)); err == nil {
		t.Fatal("empty answer accepted")
	}
}

func TestSchemaIsValidJSONWithEnum(t *testing.T) {
	var s struct {
		Properties struct {
			Tags struct {
				Items struct {
					Enum []string `json:"enum"`
				} `json:"items"`
			} `json:"tags"`
			NewTag struct {
				Pattern string `json:"pattern"`
			} `json:"new_tag"`
		} `json:"properties"`
		Required             []string `json:"required"`
		AdditionalProperties *bool    `json:"additionalProperties"`
	}
	if err := json.Unmarshal([]byte(Schema([]Tag{"bdd", "go"})), &s); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(s.Properties.Tags.Items.Enum, []string{"bdd", "go"}) {
		t.Errorf("enum = %v", s.Properties.Tags.Items.Enum)
	}
	if s.Properties.NewTag.Pattern != "^[a-z0-9-]{2,20}$" {
		t.Errorf("pattern = %q", s.Properties.NewTag.Pattern)
	}
	if !slices.Equal(s.Required, []string{"tags"}) {
		t.Errorf("required = %v", s.Required)
	}
	if s.AdditionalProperties == nil || *s.AdditionalProperties {
		t.Error("additionalProperties must be false")
	}
}
