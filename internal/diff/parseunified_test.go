package diff

import "testing"

// summarize counts context/added/removed lines across all hunks for compact
// assertions that don't depend on exact content.
func summarize(hunks []Hunk) (ctx, add, del int) {
	for _, h := range hunks {
		for _, l := range h.Lines {
			switch l.Type {
			case Context:
				ctx++
			case Added:
				add++
			case Removed:
				del++
			}
		}
	}
	return
}

func TestParseUnifiedDiff_EmptyAndBinary(t *testing.T) {
	if got := ParseUnifiedDiff(""); got != nil {
		t.Errorf("empty patch: got %v, want nil", got)
	}
	if got := ParseUnifiedDiff("Binary files a/x.png and b/x.png differ"); got != nil {
		t.Errorf("binary notice: got %v, want nil (no hunk header)", got)
	}
}

func TestParseUnifiedDiff_EditHunkLineNumbers(t *testing.T) {
	patch := "@@ -1,4 +1,5 @@\n a\n b\n-c\n+c2\n+c3\n d"
	hunks := ParseUnifiedDiff(patch)
	if len(hunks) != 1 {
		t.Fatalf("len(hunks) = %d, want 1", len(hunks))
	}
	h := hunks[0]
	if h.OldStart != 1 || h.OldCount != 4 || h.NewStart != 1 || h.NewCount != 5 {
		t.Errorf("header = -%d,%d +%d,%d, want -1,4 +1,5", h.OldStart, h.OldCount, h.NewStart, h.NewCount)
	}
	ctx, add, del := summarize(hunks)
	if ctx != 3 || add != 2 || del != 1 {
		t.Errorf("counts ctx/add/del = %d/%d/%d, want 3/2/1", ctx, add, del)
	}

	// Spot-check line numbering: context "a" is old/new line 1; the added lines
	// carry only NewNum; the removed line carries only OldNum.
	want := []Line{
		{Type: Context, Content: "a", OldNum: 1, NewNum: 1},
		{Type: Context, Content: "b", OldNum: 2, NewNum: 2},
		{Type: Removed, Content: "c", OldNum: 3, NewNum: 0},
		{Type: Added, Content: "c2", OldNum: 0, NewNum: 3},
		{Type: Added, Content: "c3", OldNum: 0, NewNum: 4},
		{Type: Context, Content: "d", OldNum: 4, NewNum: 5},
	}
	for i, w := range want {
		got := h.Lines[i]
		if got != w {
			t.Errorf("line[%d] = %+v, want %+v", i, got, w)
		}
	}
}

func TestParseUnifiedDiff_AddedFile(t *testing.T) {
	// New file: old range is -0,0; every body line is an addition.
	patch := "@@ -0,0 +1,2 @@\n+hello\n+world"
	hunks := ParseUnifiedDiff(patch)
	if len(hunks) != 1 {
		t.Fatalf("len(hunks) = %d, want 1", len(hunks))
	}
	ctx, add, del := summarize(hunks)
	if ctx != 0 || add != 2 || del != 0 {
		t.Errorf("counts ctx/add/del = %d/%d/%d, want 0/2/0", ctx, add, del)
	}
	if hunks[0].Lines[0].NewNum != 1 || hunks[0].Lines[1].NewNum != 2 {
		t.Errorf("added line numbers = %d,%d, want 1,2", hunks[0].Lines[0].NewNum, hunks[0].Lines[1].NewNum)
	}
}

func TestParseUnifiedDiff_DeletedFile(t *testing.T) {
	// Deleted file: new range is +0,0; every body line is a removal.
	patch := "@@ -1,2 +0,0 @@\n-gone1\n-gone2"
	hunks := ParseUnifiedDiff(patch)
	ctx, add, del := summarize(hunks)
	if ctx != 0 || add != 0 || del != 2 {
		t.Errorf("counts ctx/add/del = %d/%d/%d, want 0/0/2", ctx, add, del)
	}
	if hunks[0].Lines[0].OldNum != 1 || hunks[0].Lines[1].OldNum != 2 {
		t.Errorf("removed line numbers = %d,%d, want 1,2", hunks[0].Lines[0].OldNum, hunks[0].Lines[1].OldNum)
	}
}

func TestParseUnifiedDiff_MultipleHunks(t *testing.T) {
	patch := "@@ -1,2 +1,2 @@\n a\n-b\n+B\n@@ -10,2 +10,3 @@ func foo() {\n ctx\n+new\n more"
	hunks := ParseUnifiedDiff(patch)
	if len(hunks) != 2 {
		t.Fatalf("len(hunks) = %d, want 2", len(hunks))
	}
	if hunks[1].OldStart != 10 || hunks[1].NewStart != 10 {
		t.Errorf("second hunk start = -%d +%d, want -10 +10", hunks[1].OldStart, hunks[1].NewStart)
	}
	// The section heading after the closing "@@" must not be treated as content.
	if hunks[1].Lines[0].Content != "ctx" {
		t.Errorf("first body line of hunk 2 = %q, want ctx (heading ignored)", hunks[1].Lines[0].Content)
	}
}

func TestParseUnifiedDiff_OmittedCountAndNoNewlineMarker(t *testing.T) {
	// Single-line ranges omit the count; the "\ No newline" marker is metadata.
	patch := "@@ -1 +1 @@\n-old\n+new\n\\ No newline at end of file"
	hunks := ParseUnifiedDiff(patch)
	if len(hunks) != 1 {
		t.Fatalf("len(hunks) = %d, want 1", len(hunks))
	}
	h := hunks[0]
	if h.OldCount != 1 || h.NewCount != 1 {
		t.Errorf("counts = old %d new %d, want 1/1 (omitted count defaults to 1)", h.OldCount, h.NewCount)
	}
	ctx, add, del := summarize(hunks)
	if ctx != 0 || add != 1 || del != 1 {
		t.Errorf("counts ctx/add/del = %d/%d/%d, want 0/1/1 (\\ marker skipped)", ctx, add, del)
	}
}
