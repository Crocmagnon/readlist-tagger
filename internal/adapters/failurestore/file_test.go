package failurestore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingIsEmpty(t *testing.T) {
	f, err := Load(filepath.Join(t.TempDir(), "failures.json"))
	if err != nil {
		t.Fatal(err)
	}
	if f.Count(1) != 0 {
		t.Fatalf("Count = %d", f.Count(1))
	}
}

func TestRecordIncrements(t *testing.T) {
	f, _ := Load(filepath.Join(t.TempDir(), "failures.json"))
	for range 3 {
		f.Record(7)
	}
	if f.Count(7) != 3 {
		t.Fatalf("Count = %d", f.Count(7))
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "failures.json")
	f, _ := Load(p)
	f.Record(7)
	f.Record(7)
	f.Record(9)
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	g, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if g.Count(7) != 2 || g.Count(9) != 1 {
		t.Fatalf("got 7→%d 9→%d", g.Count(7), g.Count(9))
	}
	g.Clear(7)
	if err := g.Save(); err != nil {
		t.Fatal(err)
	}
	h, _ := Load(p)
	if h.Count(7) != 0 || h.Count(9) != 1 {
		t.Fatalf("got 7→%d 9→%d", h.Count(7), h.Count(9))
	}
}

func TestLoadCorruptIsError(t *testing.T) {
	p := filepath.Join(t.TempDir(), "failures.json")
	if err := os.WriteFile(p, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("corrupt file accepted")
	}
}

func TestSaveWithoutChangeDoesNotCreateFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "failures.json")
	f, _ := Load(p)
	f.Clear(3) // clearing an unknown id is not a change
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatalf("file written without change: %v", err)
	}
}

func TestSaveWithoutChangeDoesNotRewrite(t *testing.T) {
	p := filepath.Join(t.TempDir(), "failures.json")
	f, _ := Load(p)
	f.Record(1)
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, 0o400); err != nil { // a rewrite (temp file + rename) would replace the mode
		t.Fatal(err)
	}
	g, _ := Load(p)
	_ = g.Count(1)
	if err := g.Save(); err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Stat(p); st.Mode().Perm() != 0o400 {
		t.Fatalf("file rewritten without change (mode %v)", st.Mode().Perm())
	}
}
