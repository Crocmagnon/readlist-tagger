// Package failurestore keeps the per-entry failure counters in a JSON file.
package failurestore

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Crocmagnon/readlist-tagger/internal/domain/tagging"
)

// File is a failure counter backed by a JSON object {"<entry id>": count}.
type File struct {
	path    string
	counts  map[tagging.EntryID]int
	changed bool
}

var _ tagging.Failures = (*File)(nil)

// Load reads path; a missing file is an empty counter.
func Load(path string) (*File, error) {
	f := &File{path: path, counts: map[tagging.EntryID]int{}}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return f, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &f.counts); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return f, nil
}

func (f *File) Count(id tagging.EntryID) int { return f.counts[id] }
func (f *File) Record(id tagging.EntryID) {
	f.counts[id]++
	f.changed = true
}

func (f *File) Clear(id tagging.EntryID) {
	if _, ok := f.counts[id]; ok {
		delete(f.counts, id)
		f.changed = true
	}
}

// Save writes the counters atomically, only if they changed since Load.
func (f *File) Save() error {
	if !f.changed {
		return nil
	}
	b, err := json.Marshal(f.counts)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(f.path), ".failures-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), f.path); err != nil {
		return err
	}
	f.changed = false
	return nil
}
