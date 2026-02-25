package diff

import (
	"testing"

	"github.com/Elpulgo/azdo/internal/azdevops"
)

func TestSplitLines(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    int // expected number of lines
	}{
		{name: "empty string", content: "", want: 0},
		{name: "single line no newline", content: "hello", want: 1},
		{name: "single line with newline", content: "hello\n", want: 1},
		{name: "two lines", content: "hello\nworld\n", want: 2},
		{name: "three lines no trailing newline", content: "a\nb\nc", want: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitLines(tt.content)
			if len(got) != tt.want {
				t.Errorf("splitLines(%q) = %d lines, want %d", tt.content, len(got), tt.want)
			}
		})
	}
}

func TestComputeDiff_BothEmpty(t *testing.T) {
	hunks := ComputeDiff("", "", 3)
	if len(hunks) != 0 {
		t.Errorf("Expected 0 hunks for empty files, got %d", len(hunks))
	}
}

func TestComputeDiff_IdenticalContent(t *testing.T) {
	content := "line1\nline2\nline3\n"
	hunks := ComputeDiff(content, content, 3)
	if len(hunks) != 0 {
		t.Errorf("Expected 0 hunks for identical files, got %d", len(hunks))
	}
}

func TestComputeDiff_AllAdded(t *testing.T) {
	hunks := ComputeDiff("", "line1\nline2\nline3\n", 3)
	if len(hunks) != 1 {
		t.Fatalf("Expected 1 hunk, got %d", len(hunks))
	}

	hunk := hunks[0]
	if len(hunk.Lines) != 3 {
		t.Fatalf("Expected 3 lines, got %d", len(hunk.Lines))
	}

	for i, line := range hunk.Lines {
		if line.Type != Added {
			t.Errorf("Line %d type = %d, want Added", i, line.Type)
		}
		if line.OldNum != 0 {
			t.Errorf("Line %d OldNum = %d, want 0 (added line)", i, line.OldNum)
		}
		if line.NewNum != i+1 {
			t.Errorf("Line %d NewNum = %d, want %d", i, line.NewNum, i+1)
		}
	}

	if hunk.OldCount != 0 {
		t.Errorf("OldCount = %d, want 0", hunk.OldCount)
	}
	if hunk.NewCount != 3 {
		t.Errorf("NewCount = %d, want 3", hunk.NewCount)
	}
}

func TestComputeDiff_AllRemoved(t *testing.T) {
	hunks := ComputeDiff("line1\nline2\nline3\n", "", 3)
	if len(hunks) != 1 {
		t.Fatalf("Expected 1 hunk, got %d", len(hunks))
	}

	hunk := hunks[0]
	if len(hunk.Lines) != 3 {
		t.Fatalf("Expected 3 lines, got %d", len(hunk.Lines))
	}

	for i, line := range hunk.Lines {
		if line.Type != Removed {
			t.Errorf("Line %d type = %d, want Removed", i, line.Type)
		}
		if line.OldNum != i+1 {
			t.Errorf("Line %d OldNum = %d, want %d", i, line.OldNum, i+1)
		}
		if line.NewNum != 0 {
			t.Errorf("Line %d NewNum = %d, want 0 (removed line)", i, line.NewNum)
		}
	}

	if hunk.OldCount != 3 {
		t.Errorf("OldCount = %d, want 3", hunk.OldCount)
	}
	if hunk.NewCount != 0 {
		t.Errorf("NewCount = %d, want 0", hunk.NewCount)
	}
}

func TestComputeDiff_SingleLineEdit(t *testing.T) {
	old := "line1\nline2\nline3\n"
	new := "line1\nmodified\nline3\n"

	hunks := ComputeDiff(old, new, 3)
	if len(hunks) != 1 {
		t.Fatalf("Expected 1 hunk, got %d", len(hunks))
	}

	hunk := hunks[0]
	// Should have: context(line1) + removed(line2) + added(modified) + context(line3)
	if len(hunk.Lines) != 4 {
		t.Fatalf("Expected 4 lines in hunk, got %d", len(hunk.Lines))
	}

	// Verify line types
	expectedTypes := []LineType{Context, Removed, Added, Context}
	for i, expected := range expectedTypes {
		if hunk.Lines[i].Type != expected {
			t.Errorf("Line %d type = %d, want %d", i, hunk.Lines[i].Type, expected)
		}
	}

	// Verify removed line content
	if hunk.Lines[1].Content != "line2" {
		t.Errorf("Removed line content = %q, want %q", hunk.Lines[1].Content, "line2")
	}
	// Verify added line content
	if hunk.Lines[2].Content != "modified" {
		t.Errorf("Added line content = %q, want %q", hunk.Lines[2].Content, "modified")
	}
}

func TestComputeDiff_ContextLinesLimit(t *testing.T) {
	// 10 unchanged lines, then a change, then 10 more unchanged lines
	old := "a\nb\nc\nd\ne\nf\ng\nh\ni\nj\noriginal\nk\nl\nm\nn\no\np\nq\nr\ns\n"
	new := "a\nb\nc\nd\ne\nf\ng\nh\ni\nj\nchanged\nk\nl\nm\nn\no\np\nq\nr\ns\n"

	hunks := ComputeDiff(old, new, 3)
	if len(hunks) != 1 {
		t.Fatalf("Expected 1 hunk, got %d", len(hunks))
	}

	hunk := hunks[0]
	// Should have 3 context before + removed + added + 3 context after = 8 lines
	if len(hunk.Lines) != 8 {
		t.Errorf("Expected 8 lines in hunk (3 ctx + removed + added + 3 ctx), got %d", len(hunk.Lines))
	}

	// First context line should be "h" (line 8, 3 before "original" which is line 11)
	if hunk.Lines[0].Content != "h" {
		t.Errorf("First context line = %q, want %q", hunk.Lines[0].Content, "h")
	}
	// Last context line should be "m" (line 13, 3 after "changed")
	if hunk.Lines[len(hunk.Lines)-1].Content != "m" {
		t.Errorf("Last context line = %q, want %q", hunk.Lines[len(hunk.Lines)-1].Content, "m")
	}
}

func TestComputeDiff_TwoSeparateHunks(t *testing.T) {
	// Changes far apart should produce separate hunks
	old := "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12\n13\n14\n15\n16\n17\n18\n19\n20\n"
	new := "1\nchanged2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12\n13\n14\n15\n16\n17\n18\nchanged19\n20\n"

	hunks := ComputeDiff(old, new, 2)
	if len(hunks) != 2 {
		t.Fatalf("Expected 2 hunks for distant changes with context=2, got %d", len(hunks))
	}

	// First hunk should cover the change at line 2
	foundChanged2 := false
	for _, line := range hunks[0].Lines {
		if line.Type == Added && line.Content == "changed2" {
			foundChanged2 = true
		}
	}
	if !foundChanged2 {
		t.Error("First hunk should contain 'changed2'")
	}

	// Second hunk should cover the change at line 19
	foundChanged19 := false
	for _, line := range hunks[1].Lines {
		if line.Type == Added && line.Content == "changed19" {
			foundChanged19 = true
		}
	}
	if !foundChanged19 {
		t.Error("Second hunk should contain 'changed19'")
	}
}

func TestComputeDiff_MergedHunks(t *testing.T) {
	// Changes close together should merge into one hunk
	old := "1\n2\n3\n4\n5\n6\n7\n8\n"
	new := "1\nchanged2\n3\n4\n5\nchanged6\n7\n8\n"

	hunks := ComputeDiff(old, new, 3)
	// With context=3, the context regions overlap, so should be 1 merged hunk
	if len(hunks) != 1 {
		t.Fatalf("Expected 1 merged hunk, got %d", len(hunks))
	}
}

func TestComputeDiff_AddedLines(t *testing.T) {
	old := "line1\nline3\n"
	new := "line1\nline2\nline3\n"

	hunks := ComputeDiff(old, new, 3)
	if len(hunks) != 1 {
		t.Fatalf("Expected 1 hunk, got %d", len(hunks))
	}

	// Find the added line
	foundAdded := false
	for _, line := range hunks[0].Lines {
		if line.Type == Added && line.Content == "line2" {
			foundAdded = true
			if line.OldNum != 0 {
				t.Errorf("Added line OldNum = %d, want 0", line.OldNum)
			}
		}
	}
	if !foundAdded {
		t.Error("Expected to find added line 'line2'")
	}
}

func TestComputeDiff_RemovedLines(t *testing.T) {
	old := "line1\nline2\nline3\n"
	new := "line1\nline3\n"

	hunks := ComputeDiff(old, new, 3)
	if len(hunks) != 1 {
		t.Fatalf("Expected 1 hunk, got %d", len(hunks))
	}

	// Find the removed line
	foundRemoved := false
	for _, line := range hunks[0].Lines {
		if line.Type == Removed && line.Content == "line2" {
			foundRemoved = true
			if line.NewNum != 0 {
				t.Errorf("Removed line NewNum = %d, want 0", line.NewNum)
			}
		}
	}
	if !foundRemoved {
		t.Error("Expected to find removed line 'line2'")
	}
}

func TestComputeDiff_LineNumbers(t *testing.T) {
	old := "a\nb\nc\n"
	new := "a\nx\nc\n"

	hunks := ComputeDiff(old, new, 5)
	if len(hunks) != 1 {
		t.Fatalf("Expected 1 hunk, got %d", len(hunks))
	}

	// Check line numbers for each line
	for _, line := range hunks[0].Lines {
		switch {
		case line.Content == "a" && line.Type == Context:
			if line.OldNum != 1 || line.NewNum != 1 {
				t.Errorf("Context 'a': OldNum=%d NewNum=%d, want 1,1", line.OldNum, line.NewNum)
			}
		case line.Content == "b" && line.Type == Removed:
			if line.OldNum != 2 || line.NewNum != 0 {
				t.Errorf("Removed 'b': OldNum=%d NewNum=%d, want 2,0", line.OldNum, line.NewNum)
			}
		case line.Content == "x" && line.Type == Added:
			if line.OldNum != 0 || line.NewNum != 2 {
				t.Errorf("Added 'x': OldNum=%d NewNum=%d, want 0,2", line.OldNum, line.NewNum)
			}
		case line.Content == "c" && line.Type == Context:
			if line.OldNum != 3 || line.NewNum != 3 {
				t.Errorf("Context 'c': OldNum=%d NewNum=%d, want 3,3", line.OldNum, line.NewNum)
			}
		}
	}
}

func TestComputeDiff_HunkHeaders(t *testing.T) {
	old := "a\nb\nc\n"
	new := "a\nx\nc\n"

	hunks := ComputeDiff(old, new, 5)
	if len(hunks) != 1 {
		t.Fatalf("Expected 1 hunk, got %d", len(hunks))
	}

	hunk := hunks[0]
	if hunk.OldStart != 1 {
		t.Errorf("OldStart = %d, want 1", hunk.OldStart)
	}
	if hunk.NewStart != 1 {
		t.Errorf("NewStart = %d, want 1", hunk.NewStart)
	}
	// OldCount: a(context) + b(removed) + c(context) = 3
	if hunk.OldCount != 3 {
		t.Errorf("OldCount = %d, want 3", hunk.OldCount)
	}
	// NewCount: a(context) + x(added) + c(context) = 3
	if hunk.NewCount != 3 {
		t.Errorf("NewCount = %d, want 3", hunk.NewCount)
	}
}

func TestComputeDiff_ContextZero(t *testing.T) {
	old := "a\nb\nc\n"
	new := "a\nx\nc\n"

	hunks := ComputeDiff(old, new, 0)
	if len(hunks) != 1 {
		t.Fatalf("Expected 1 hunk, got %d", len(hunks))
	}

	// With 0 context, should only have removed + added
	if len(hunks[0].Lines) != 2 {
		t.Errorf("Expected 2 lines with context=0, got %d", len(hunks[0].Lines))
	}
}

func TestComputeDiff_MultipleAdjacentChanges(t *testing.T) {
	old := "a\nb\nc\nd\n"
	new := "a\nX\nY\nd\n"

	hunks := ComputeDiff(old, new, 5)
	if len(hunks) != 1 {
		t.Fatalf("Expected 1 hunk, got %d", len(hunks))
	}

	// Count by type
	added, removed, context := 0, 0, 0
	for _, line := range hunks[0].Lines {
		switch line.Type {
		case Added:
			added++
		case Removed:
			removed++
		case Context:
			context++
		}
	}

	if removed != 2 {
		t.Errorf("Expected 2 removed lines, got %d", removed)
	}
	if added != 2 {
		t.Errorf("Expected 2 added lines, got %d", added)
	}
	if context != 2 {
		t.Errorf("Expected 2 context lines, got %d", context)
	}
}

func TestMapThreadsToLines(t *testing.T) {
	threads := []azdevops.Thread{
		{
			ID:     1,
			Status: "active",
			ThreadContext: &azdevops.ThreadContext{
				FilePath:       "/src/main.go",
				RightFileStart: &azdevops.FilePosition{Line: 10, Offset: 1},
			},
		},
		{
			ID:     2,
			Status: "fixed",
			ThreadContext: &azdevops.ThreadContext{
				FilePath:       "/src/main.go",
				RightFileStart: &azdevops.FilePosition{Line: 25, Offset: 1},
			},
		},
		{
			ID:     3,
			Status: "active",
			ThreadContext: &azdevops.ThreadContext{
				FilePath:       "/src/other.go",
				RightFileStart: &azdevops.FilePosition{Line: 5, Offset: 1},
			},
		},
		{
			ID:     4,
			Status: "active",
			ThreadContext: &azdevops.ThreadContext{
				FilePath:       "/src/main.go",
				RightFileStart: &azdevops.FilePosition{Line: 10, Offset: 1},
			},
		},
		{
			ID:            5,
			Status:        "active",
			ThreadContext: nil, // general comment, no file context
		},
		{
			ID:     6,
			Status: "active",
			ThreadContext: &azdevops.ThreadContext{
				FilePath:       "/src/main.go",
				RightFileStart: nil, // missing position
			},
		},
	}

	result := MapThreadsToLines(threads, "/src/main.go")

	// Line 10 should have 2 threads (IDs 1 and 4)
	if len(result[10]) != 2 {
		t.Errorf("Line 10: expected 2 threads, got %d", len(result[10]))
	}
	if result[10][0].ID != 1 || result[10][1].ID != 4 {
		t.Errorf("Line 10: expected thread IDs 1,4, got %d,%d", result[10][0].ID, result[10][1].ID)
	}

	// Line 25 should have 1 thread (ID 2)
	if len(result[25]) != 1 {
		t.Errorf("Line 25: expected 1 thread, got %d", len(result[25]))
	}
	if result[25][0].ID != 2 {
		t.Errorf("Line 25: expected thread ID 2, got %d", result[25][0].ID)
	}

	// Line 5 should not be present (different file)
	if len(result[5]) != 0 {
		t.Errorf("Line 5: expected 0 threads (different file), got %d", len(result[5]))
	}

	// Total keys should be 2 (lines 10 and 25)
	if len(result) != 2 {
		t.Errorf("Expected 2 line entries, got %d", len(result))
	}
}

func TestMapThreadsToLines_EmptyThreads(t *testing.T) {
	result := MapThreadsToLines(nil, "/src/main.go")
	if len(result) != 0 {
		t.Errorf("Expected empty map for nil threads, got %d entries", len(result))
	}
}

func TestMapThreadsToLines_NoMatchingFile(t *testing.T) {
	threads := []azdevops.Thread{
		{
			ID: 1,
			ThreadContext: &azdevops.ThreadContext{
				FilePath:       "/src/other.go",
				RightFileStart: &azdevops.FilePosition{Line: 10},
			},
		},
	}

	result := MapThreadsToLines(threads, "/src/main.go")
	if len(result) != 0 {
		t.Errorf("Expected empty map for non-matching file, got %d entries", len(result))
	}
}

func TestCountCommentsPerFile(t *testing.T) {
	threads := []azdevops.Thread{
		{
			ID: 1,
			ThreadContext: &azdevops.ThreadContext{
				FilePath:       "/src/main.go",
				RightFileStart: &azdevops.FilePosition{Line: 10},
			},
			Comments: []azdevops.Comment{
				{ID: 1, Content: "Fix this"},
				{ID: 2, Content: "Will do", ParentCommentID: 1},
			},
		},
		{
			ID: 2,
			ThreadContext: &azdevops.ThreadContext{
				FilePath:       "/src/main.go",
				RightFileStart: &azdevops.FilePosition{Line: 25},
			},
			Comments: []azdevops.Comment{
				{ID: 3, Content: "Nice"},
			},
		},
		{
			ID: 3,
			ThreadContext: &azdevops.ThreadContext{
				FilePath:       "/src/other.go",
				RightFileStart: &azdevops.FilePosition{Line: 5},
			},
			Comments: []azdevops.Comment{
				{ID: 4, Content: "Check this"},
			},
		},
		{
			ID:            4,
			ThreadContext: nil, // general comment, no file
			Comments: []azdevops.Comment{
				{ID: 5, Content: "Looks good overall"},
			},
		},
		{
			ID: 5,
			ThreadContext: &azdevops.ThreadContext{
				FilePath:       "/src/main.go",
				RightFileStart: nil, // missing position - should still count
			},
			Comments: []azdevops.Comment{
				{ID: 6, Content: "General file comment"},
			},
		},
	}

	result := CountCommentsPerFile(threads)

	// /src/main.go: threads 1 (2 comments) + 2 (1 comment) + 5 (1 comment) = 4 comments
	if result["/src/main.go"] != 4 {
		t.Errorf("/src/main.go: expected 4 comments, got %d", result["/src/main.go"])
	}

	// /src/other.go: thread 3 (1 comment) = 1 comment
	if result["/src/other.go"] != 1 {
		t.Errorf("/src/other.go: expected 1 comment, got %d", result["/src/other.go"])
	}

	// Should only have 2 file entries (general comments excluded)
	if len(result) != 2 {
		t.Errorf("Expected 2 file entries, got %d", len(result))
	}
}

func TestCountCommentsPerFile_Empty(t *testing.T) {
	result := CountCommentsPerFile(nil)
	if len(result) != 0 {
		t.Errorf("Expected empty map for nil threads, got %d entries", len(result))
	}
}

func TestCountCommentsPerFile_NoCodeComments(t *testing.T) {
	threads := []azdevops.Thread{
		{
			ID:            1,
			ThreadContext: nil,
			Comments: []azdevops.Comment{
				{ID: 1, Content: "General comment"},
			},
		},
	}

	result := CountCommentsPerFile(threads)
	if len(result) != 0 {
		t.Errorf("Expected empty map for threads without file context, got %d entries", len(result))
	}
}

func TestFilterGeneralThreads(t *testing.T) {
	threads := []azdevops.Thread{
		{
			ID:            1,
			ThreadContext: nil, // general comment
			Comments:      []azdevops.Comment{{ID: 1, Content: "Looks good"}},
		},
		{
			ID: 2,
			ThreadContext: &azdevops.ThreadContext{
				FilePath:       "/src/main.go",
				RightFileStart: &azdevops.FilePosition{Line: 10},
			},
			Comments: []azdevops.Comment{{ID: 2, Content: "Fix this"}},
		},
		{
			ID:            3,
			ThreadContext: nil, // another general comment
			Comments:      []azdevops.Comment{{ID: 3, Content: "Nice PR"}},
		},
		{
			ID: 4,
			ThreadContext: &azdevops.ThreadContext{
				FilePath: "", // empty path treated as general
			},
			Comments: []azdevops.Comment{{ID: 4, Content: "Empty path"}},
		},
	}

	result := FilterGeneralThreads(threads)

	if len(result) != 3 {
		t.Fatalf("Expected 3 general threads, got %d", len(result))
	}
	if result[0].ID != 1 {
		t.Errorf("First thread ID = %d, want 1", result[0].ID)
	}
	if result[1].ID != 3 {
		t.Errorf("Second thread ID = %d, want 3", result[1].ID)
	}
	if result[2].ID != 4 {
		t.Errorf("Third thread ID = %d, want 4", result[2].ID)
	}
}

func TestFilterGeneralThreads_Empty(t *testing.T) {
	result := FilterGeneralThreads(nil)
	if result != nil {
		t.Errorf("Expected nil for nil input, got %v", result)
	}
}

func TestFilterGeneralThreads_NoGeneralComments(t *testing.T) {
	threads := []azdevops.Thread{
		{
			ID: 1,
			ThreadContext: &azdevops.ThreadContext{
				FilePath:       "/src/main.go",
				RightFileStart: &azdevops.FilePosition{Line: 10},
			},
		},
	}

	result := FilterGeneralThreads(threads)
	if result != nil {
		t.Errorf("Expected nil when no general threads, got %d entries", len(result))
	}
}

func TestFilterGeneralThreads_AllGeneral(t *testing.T) {
	threads := []azdevops.Thread{
		{ID: 1, ThreadContext: nil},
		{ID: 2, ThreadContext: nil},
	}

	result := FilterGeneralThreads(threads)
	if len(result) != 2 {
		t.Errorf("Expected 2 general threads, got %d", len(result))
	}
}

func TestCountGeneralComments(t *testing.T) {
	threads := []azdevops.Thread{
		{
			ID:            1,
			ThreadContext: nil,
			Comments: []azdevops.Comment{
				{ID: 1, Content: "Looks good"},
				{ID: 2, Content: "Thanks", ParentCommentID: 1},
			},
		},
		{
			ID: 2,
			ThreadContext: &azdevops.ThreadContext{
				FilePath: "/src/main.go",
			},
			Comments: []azdevops.Comment{{ID: 3, Content: "Fix"}},
		},
		{
			ID:            3,
			ThreadContext: nil,
			Comments:      []azdevops.Comment{{ID: 4, Content: "Nice"}},
		},
	}

	count := CountGeneralComments(threads)
	if count != 3 {
		t.Errorf("Expected 3 general comments, got %d", count)
	}
}

func TestCountGeneralComments_Empty(t *testing.T) {
	count := CountGeneralComments(nil)
	if count != 0 {
		t.Errorf("Expected 0 for nil threads, got %d", count)
	}
}

func TestCountGeneralComments_NoGeneral(t *testing.T) {
	threads := []azdevops.Thread{
		{
			ID: 1,
			ThreadContext: &azdevops.ThreadContext{FilePath: "/src/main.go"},
			Comments:      []azdevops.Comment{{ID: 1, Content: "Code comment"}},
		},
	}

	count := CountGeneralComments(threads)
	if count != 0 {
		t.Errorf("Expected 0 for no general threads, got %d", count)
	}
}
