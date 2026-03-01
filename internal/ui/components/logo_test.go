package components

import (
	"strings"
	"testing"

	"github.com/Elpulgo/azdo/internal/ui/styles"
)

func TestLogo_New(t *testing.T) {
	logo := NewLogo(styles.DefaultStyles())

	if logo == nil {
		t.Fatal("expected non-nil Logo")
	}
}

func TestLogo_Height(t *testing.T) {
	logo := NewLogo(styles.DefaultStyles())

	height := logo.Height()
	if height != 3 {
		t.Errorf("expected height 3, got %d", height)
	}
}

func TestLogo_View_ContainsArt(t *testing.T) {
	logo := NewLogo(styles.DefaultStyles())

	view := logo.View()

	if view == "" {
		t.Fatal("expected non-empty view")
	}

	// The view should contain multiple lines (ASCII art)
	lines := strings.Split(view, "\n")
	if len(lines) < 3 {
		t.Errorf("expected 3-line ASCII art, got %d lines", len(lines))
	}
}

func TestLogo_View_ContainsLogoChars(t *testing.T) {
	logo := NewLogo(styles.DefaultStyles())

	view := logo.View()

	// Should contain the box-drawing characters from the logo
	if !strings.Contains(view, "╔═╗") {
		t.Error("view should contain logo art characters")
	}
}
