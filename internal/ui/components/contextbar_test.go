package components

import (
	"strings"
	"testing"

	"github.com/Elpulgo/azdo/internal/ui/styles"
)

func TestContextBar_New(t *testing.T) {
	cb := NewContextBar(styles.DefaultStyles())

	if cb == nil {
		t.Fatal("expected non-nil ContextBar")
	}
}

func TestContextBar_SetWidth(t *testing.T) {
	cb := NewContextBar(styles.DefaultStyles())
	cb.SetWidth(100)

	if cb.width != 100 {
		t.Errorf("expected width 100, got %d", cb.width)
	}
}

func TestContextBar_SetItems(t *testing.T) {
	cb := NewContextBar(styles.DefaultStyles())
	items := []ContextItem{
		{Key: "↑↓", Description: "navigate"},
		{Key: "enter", Description: "select"},
	}
	cb.SetItems(items)

	if len(cb.items) != 2 {
		t.Errorf("expected 2 items, got %d", len(cb.items))
	}
}

func TestContextBar_AddItem(t *testing.T) {
	cb := NewContextBar(styles.DefaultStyles())
	cb.AddItem("esc", "back")

	if len(cb.items) != 1 {
		t.Errorf("expected 1 item, got %d", len(cb.items))
	}
	if cb.items[0].Key != "esc" || cb.items[0].Description != "back" {
		t.Error("item not added correctly")
	}
}

func TestContextBar_SetStatus(t *testing.T) {
	cb := NewContextBar(styles.DefaultStyles())
	cb.SetStatus("Loading...")

	if cb.status != "Loading..." {
		t.Errorf("expected status 'Loading...', got '%s'", cb.status)
	}
}

func TestContextBar_SetScrollPercent(t *testing.T) {
	cb := NewContextBar(styles.DefaultStyles())
	cb.SetScrollPercent(50.5)

	if cb.scrollPercent != 50.5 {
		t.Errorf("expected scroll percent 50.5, got %f", cb.scrollPercent)
	}
}

func TestContextBar_ShowScrollPercent(t *testing.T) {
	cb := NewContextBar(styles.DefaultStyles())

	// Default should be false
	if cb.showScroll {
		t.Error("showScroll should be false by default")
	}

	cb.ShowScrollPercent(true)
	if !cb.showScroll {
		t.Error("showScroll should be true after ShowScrollPercent(true)")
	}
}

func TestContextBar_View_Empty(t *testing.T) {
	cb := NewContextBar(styles.DefaultStyles())
	cb.SetWidth(80)

	view := cb.View()

	// Should still render a box even if empty
	if view == "" {
		t.Error("view should not be empty")
	}
}

func TestContextBar_View_WithItems(t *testing.T) {
	cb := NewContextBar(styles.DefaultStyles())
	cb.SetWidth(100)
	cb.AddItem("↑↓", "navigate")
	cb.AddItem("enter", "select")

	view := cb.View()

	if !strings.Contains(view, "navigate") {
		t.Error("view should contain 'navigate'")
	}
	if !strings.Contains(view, "select") {
		t.Error("view should contain 'select'")
	}
}

func TestContextBar_View_WithStatus(t *testing.T) {
	cb := NewContextBar(styles.DefaultStyles())
	cb.SetWidth(100)
	cb.SetStatus("Stage has no logs")

	view := cb.View()

	if !strings.Contains(view, "Stage has no logs") {
		t.Error("view should contain status message")
	}
}

func TestContextBar_View_WithScrollPercent(t *testing.T) {
	cb := NewContextBar(styles.DefaultStyles())
	cb.SetWidth(100)
	cb.ShowScrollPercent(true)
	cb.SetScrollPercent(75.0)

	view := cb.View()

	if !strings.Contains(view, "75%") {
		t.Error("view should contain scroll percentage")
	}
}

func TestContextBar_View_SeparatorsBetweenItems(t *testing.T) {
	cb := NewContextBar(styles.DefaultStyles())
	cb.SetWidth(100)
	cb.AddItem("a", "first")
	cb.AddItem("b", "second")

	view := cb.View()

	// Should have separator between items
	if !strings.Contains(view, "•") && !strings.Contains(view, "│") {
		t.Error("view should contain separator between items")
	}
}

func TestContextBar_Clear(t *testing.T) {
	cb := NewContextBar(styles.DefaultStyles())
	cb.AddItem("a", "test")
	cb.SetStatus("status")
	cb.SetScrollPercent(50)

	cb.Clear()

	if len(cb.items) != 0 {
		t.Error("items should be cleared")
	}
	if cb.status != "" {
		t.Error("status should be cleared")
	}
	if cb.scrollPercent != 0 {
		t.Error("scroll percent should be cleared")
	}
}

func TestContextBar_View_HasBorder(t *testing.T) {
	cb := NewContextBar(styles.DefaultStyles())
	cb.SetWidth(80)
	cb.AddItem("test", "item")

	view := cb.View()

	// Should have box border characters
	if !strings.Contains(view, "─") && !strings.Contains(view, "│") && !strings.Contains(view, "╭") {
		t.Error("view should have border characters")
	}
}

func TestContextItem_String(t *testing.T) {
	item := ContextItem{Key: "enter", Description: "select"}

	// Just verify it doesn't panic
	_ = item.Key
	_ = item.Description
}
