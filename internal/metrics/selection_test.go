package metrics

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadSelection_MissingFile(t *testing.T) {
	rows, err := LoadSelection(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Errorf("expected no error for missing file, got %v", err)
	}
	if rows != nil {
		t.Errorf("expected nil result, got %v", rows)
	}
}

func TestSaveLoadSelection_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sel.json")
	in := []string{"sprint-40", "sprint-41", "sprint-42"}
	if err := SaveSelection(path, in); err != nil {
		t.Fatalf("save: %v", err)
	}
	out, err := LoadSelection(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !reflect.DeepEqual(out, in) {
		t.Errorf("round-trip mismatch: got %v, want %v", out, in)
	}
}

func TestSaveSelection_AtomicLeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sel.json")
	if err := SaveSelection(path, []string{"a"}); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}

func TestFilterAvailable_DropsUnknown(t *testing.T) {
	saved := []string{"sprint-40", "sprint-41", "stale-tag", "sprint-42"}
	available := []string{"sprint-40", "sprint-42", "sprint-43"}
	got := FilterAvailable(saved, available)
	want := []string{"sprint-40", "sprint-42"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestFilterAvailable_EmptySaved(t *testing.T) {
	if got := FilterAvailable(nil, []string{"a"}); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}
