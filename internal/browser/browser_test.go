package browser

import (
	"fmt"
	"testing"
)

func TestOpen_CallsOpener(t *testing.T) {
	orig := Opener
	t.Cleanup(func() { Opener = orig })

	var called string
	Opener = func(url string) error {
		called = url
		return nil
	}

	if err := Open("https://dev.azure.com/org/proj/_git/repo/pullrequest/1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "https://dev.azure.com/org/proj/_git/repo/pullrequest/1"
	if called != want {
		t.Errorf("Opener called with %q, want %q", called, want)
	}
}

func TestOpen_ReturnsOpenerError(t *testing.T) {
	orig := Opener
	t.Cleanup(func() { Opener = orig })

	Opener = func(url string) error {
		return fmt.Errorf("browser not found")
	}

	err := Open("https://example.com")
	if err == nil {
		t.Error("expected error from Opener, got nil")
	}
}
