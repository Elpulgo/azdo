package pipelines

import (
	"strings"
	"testing"
)

func TestLogViewerModel_ViewportUsesFullAvailableHeight(t *testing.T) {
	// The height passed to SetSize is already the content area (after app-level
	// borders and footer are subtracted). The log viewer should only subtract
	// its own header lines (title + separator = 2 lines).
	model := NewLogViewerModel(nil, 123, 5, "npm install")

	height := 30
	model.SetSize(80, height)

	// Set enough content to fill viewport
	var lines []string
	for i := 0; i < 100; i++ {
		lines = append(lines, "Log line "+string(rune('A'+i%26)))
	}
	model.SetContent(strings.Join(lines, "\n"))

	view := model.View()
	outputLines := strings.Split(view, "\n")

	// Total output lines should equal the height passed in.
	if len(outputLines) != height {
		t.Errorf("Log viewer output has %d lines, want %d (height passed to SetSize). "+
			"Viewport is not using full available height.", len(outputLines), height)
	}
}

func TestLogViewerModel_SetContent(t *testing.T) {
	model := NewLogViewerModel(nil, 123, 5, "Test Task")

	content := "Line 1\nLine 2\nLine 3"
	model.SetContent(content)

	if model.GetContent() != content {
		t.Errorf("GetContent() = %q, want %q", model.GetContent(), content)
	}
}

func TestLogViewerModel_Title(t *testing.T) {
	model := NewLogViewerModel(nil, 123, 5, "npm install")

	if model.GetTitle() != "npm install" {
		t.Errorf("GetTitle() = %q, want %q", model.GetTitle(), "npm install")
	}
}

func TestLogViewerModel_BuildAndLogIDs(t *testing.T) {
	model := NewLogViewerModel(nil, 456, 10, "Build Task")

	if model.GetBuildID() != 456 {
		t.Errorf("GetBuildID() = %d, want 456", model.GetBuildID())
	}

	if model.GetLogID() != 10 {
		t.Errorf("GetLogID() = %d, want 10", model.GetLogID())
	}
}

func TestLogViewerModel_LoadingState(t *testing.T) {
	model := NewLogViewerModel(nil, 123, 5, "Test Task")

	// Initially loading should be true (until content is set)
	if !model.IsLoading() {
		t.Error("Expected IsLoading() to be true initially")
	}

	// After setting content, loading should be false
	model.SetContent("Some content")
	if model.IsLoading() {
		t.Error("Expected IsLoading() to be false after SetContent")
	}
}

func TestLogViewerModel_ErrorState(t *testing.T) {
	model := NewLogViewerModel(nil, 123, 5, "Test Task")

	// Initially no error
	if model.GetError() != nil {
		t.Error("Expected GetError() to be nil initially")
	}

	// Set an error
	model.SetError("Failed to fetch logs")
	if model.GetError() == nil {
		t.Error("Expected GetError() to be non-nil after SetError")
	}
	if model.GetError().Error() != "Failed to fetch logs" {
		t.Errorf("GetError() = %q, want %q", model.GetError().Error(), "Failed to fetch logs")
	}
}

func TestLogViewerModel_View(t *testing.T) {
	model := NewLogViewerModel(nil, 123, 5, "npm install")
	model.SetSize(80, 24)
	model.spinner.SetVisible(true)

	// Test loading view
	view := model.View()
	if !strings.Contains(view, "Loading") || !strings.Contains(view, "npm install") {
		t.Errorf("Loading view should contain 'Loading' and task name, got: %q", view)
	}

	// Test with content
	model.SetContent("Build output line 1\nBuild output line 2")
	view = model.View()
	if !strings.Contains(view, "npm install") {
		t.Errorf("Content view should contain task name in header, got: %q", view)
	}
}

func TestLogViewerModel_EmptyContent(t *testing.T) {
	model := NewLogViewerModel(nil, 123, 5, "Test Task")
	model.SetSize(80, 24)
	model.SetContent("")

	view := model.View()
	if !strings.Contains(view, "No log content") {
		t.Errorf("Empty content should show 'No log content', got: %q", view)
	}
}

func TestFormatLogLines(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantLen int
	}{
		{
			name:    "empty content",
			content: "",
			wantLen: 0,
		},
		{
			name:    "single line",
			content: "Hello world",
			wantLen: 1,
		},
		{
			name:    "multiple lines",
			content: "Line 1\nLine 2\nLine 3",
			wantLen: 3,
		},
		{
			name:    "lines with empty last line",
			content: "Line 1\nLine 2\n",
			wantLen: 2, // Trailing newline shouldn't create extra line
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := formatLogLines(tt.content)
			if len(lines) != tt.wantLen {
				t.Errorf("formatLogLines() returned %d lines, want %d", len(lines), tt.wantLen)
			}
		})
	}
}

func TestStripAnsiTimestamps(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no timestamp",
			input: "Hello world",
			want:  "Hello world",
		},
		{
			name:  "azure devops timestamp format",
			input: "2024-02-06T10:00:00.000Z Starting build...",
			want:  "Starting build...",
		},
		{
			name:  "timestamp with T separator",
			input: "2024-02-06T10:00:00.123456Z npm install",
			want:  "npm install",
		},
		{
			name:  "preserve line without timestamp",
			input: "  Added 1234 packages",
			want:  "  Added 1234 packages",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripTimestamp(tt.input)
			if got != tt.want {
				t.Errorf("stripTimestamp(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestLogViewerModel_GetContextItems(t *testing.T) {
	model := NewLogViewerModel(nil, 123, 5, "Test Task")

	items := model.GetContextItems()

	// Should have keybinding items
	if len(items) == 0 {
		t.Error("GetContextItems() should return items")
	}

	// Should include scroll keys
	found := false
	for _, item := range items {
		if strings.Contains(item.Key, "↑↓") || strings.Contains(item.Description, "scroll") {
			found = true
			break
		}
	}
	if !found {
		t.Error("GetContextItems() should include scroll keybinding")
	}

	// Should include g/G for top/bottom
	foundGG := false
	for _, item := range items {
		if strings.Contains(item.Key, "g/G") || strings.Contains(item.Description, "top/bottom") {
			foundGG = true
			break
		}
	}
	if !foundGG {
		t.Error("GetContextItems() should include g/G keybinding")
	}
}

func TestLogViewerModel_GetScrollPercent(t *testing.T) {
	model := NewLogViewerModel(nil, 123, 5, "Test Task")
	model.SetSize(80, 20)

	// Generate content with many lines
	var lines []string
	for i := 0; i < 100; i++ {
		lines = append(lines, "Log line "+string(rune('0'+i%10)))
	}
	model.SetContent(strings.Join(lines, "\n"))

	// Initially should be at top (0%)
	percent := model.GetScrollPercent()
	if percent < 0 || percent > 100 {
		t.Errorf("GetScrollPercent() = %f, should be between 0 and 100", percent)
	}
}
