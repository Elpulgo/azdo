package metrics

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBackfillAlreadyDone_MissingFileReturnsFalse(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".metrics-backfill-done")

	done, err := BackfillAlreadyDone(path)
	if err != nil {
		t.Fatalf("BackfillAlreadyDone(missing) returned err: %v", err)
	}
	if done {
		t.Error("BackfillAlreadyDone(missing) = true, want false")
	}
}

func TestBackfillAlreadyDone_PresentFileReturnsTrue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".metrics-backfill-done")
	if err := os.WriteFile(path, []byte(""), 0644); err != nil {
		t.Fatalf("seed marker: %v", err)
	}

	done, err := BackfillAlreadyDone(path)
	if err != nil {
		t.Fatalf("BackfillAlreadyDone(present) returned err: %v", err)
	}
	if !done {
		t.Error("BackfillAlreadyDone(present) = false, want true")
	}
}

func TestMarkBackfillDone_CreatesMarker(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".metrics-backfill-done")

	if err := MarkBackfillDone(path); err != nil {
		t.Fatalf("MarkBackfillDone: %v", err)
	}

	done, _ := BackfillAlreadyDone(path)
	if !done {
		t.Error("after MarkBackfillDone, BackfillAlreadyDone = false; want true")
	}
}

func TestMarkBackfillDone_IsIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".metrics-backfill-done")

	if err := MarkBackfillDone(path); err != nil {
		t.Fatalf("first MarkBackfillDone: %v", err)
	}
	if err := MarkBackfillDone(path); err != nil {
		t.Fatalf("second MarkBackfillDone: %v", err)
	}
}

func TestMarkBackfillDone_CreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "sub", ".metrics-backfill-done")

	if err := MarkBackfillDone(path); err != nil {
		t.Fatalf("MarkBackfillDone with missing parent: %v", err)
	}

	done, _ := BackfillAlreadyDone(path)
	if !done {
		t.Error("after MarkBackfillDone, marker not found at nested path")
	}
}

func TestDefaultBackfillMarkerPath_UsesAzdoTuiConfig(t *testing.T) {
	path, err := DefaultBackfillMarkerPath()
	if err != nil {
		t.Fatalf("DefaultBackfillMarkerPath: %v", err)
	}
	if filepath.Base(path) != ".metrics-backfill-done" {
		t.Errorf("base = %q, want .metrics-backfill-done", filepath.Base(path))
	}
	parent := filepath.Base(filepath.Dir(path))
	if parent != "azdo-tui" {
		t.Errorf("parent dir = %q, want azdo-tui", parent)
	}
}
