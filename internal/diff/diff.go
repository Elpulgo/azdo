package diff

import (
	"strconv"
	"strings"

	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/provider"
)

// LineType represents the type of a diff line
type LineType int

const (
	Context LineType = iota
	Added
	Removed
)

// Line represents a single line in a diff
type Line struct {
	Type    LineType
	Content string
	OldNum  int // line number in old file (0 if added)
	NewNum  int // line number in new file (0 if removed)
}

// Hunk represents a contiguous group of changes with surrounding context
type Hunk struct {
	OldStart int
	OldCount int
	NewStart int
	NewCount int
	Lines    []Line
}

// FileDiff represents the diff result for a single file
type FileDiff struct {
	Path       string
	ChangeType string // "add", "edit", "delete", "rename"
	OldPath    string // for renames
	Hunks      []Hunk
}

// ComputeDiff computes the diff between old and new content with the given
// number of context lines surrounding each change.
func ComputeDiff(oldContent, newContent string, contextLines int) []Hunk {
	oldLines := splitLines(oldContent)
	newLines := splitLines(newContent)

	// Compute LCS-based edit script
	ops := computeEditScript(oldLines, newLines)

	// Group into hunks with context
	return buildHunks(ops, contextLines)
}

// ParseUnifiedDiff parses a unified-diff patch — as returned in the per-file
// "patch" field of GitHub's pull-request files API — into []Hunk, reusing the
// same Line/Hunk shape ComputeDiff produces so the diff view renders both
// identically.
//
// The patch contains hunk headers ("@@ -a,b +c,d @@ [section]") followed by
// body lines prefixed with ' ' (context), '+' (added), or '-' (removed). Lines
// beginning with '\' (e.g. "\ No newline at end of file") are metadata and are
// skipped. Counts default to 1 when omitted ("@@ -1 +1 @@"). Input with no
// valid hunk header (empty string, or a "Binary files differ" notice) yields nil.
func ParseUnifiedDiff(patch string) []Hunk {
	if patch == "" {
		return nil
	}

	var hunks []Hunk
	var cur *Hunk
	var oldNum, newNum int

	for _, raw := range strings.Split(patch, "\n") {
		if strings.HasPrefix(raw, "@@") {
			h, oStart, nStart, ok := parseHunkHeader(raw)
			if !ok {
				continue
			}
			if cur != nil {
				hunks = append(hunks, *cur)
			}
			hc := h
			cur = &hc
			oldNum, newNum = oStart, nStart
			continue
		}
		if cur == nil || raw == "" {
			// Skip preamble before the first hunk and any trailing empty split
			// element. Genuine blank context lines carry a leading space.
			continue
		}
		switch raw[0] {
		case ' ':
			cur.Lines = append(cur.Lines, Line{Type: Context, Content: raw[1:], OldNum: oldNum, NewNum: newNum})
			oldNum++
			newNum++
		case '+':
			cur.Lines = append(cur.Lines, Line{Type: Added, Content: raw[1:], OldNum: 0, NewNum: newNum})
			newNum++
		case '-':
			cur.Lines = append(cur.Lines, Line{Type: Removed, Content: raw[1:], OldNum: oldNum, NewNum: 0})
			oldNum++
		case '\\':
			// "\ No newline at end of file" — metadata, not a content line.
		}
	}
	if cur != nil {
		hunks = append(hunks, *cur)
	}
	return hunks
}

// parseHunkHeader parses an "@@ -oldStart[,oldCount] +newStart[,newCount] @@"
// line. It returns the Hunk with its header counts filled in plus the starting
// old/new line numbers, or ok=false if the line is not a well-formed header.
func parseHunkHeader(line string) (h Hunk, oldStart, newStart int, ok bool) {
	rest := line[2:] // drop leading "@@"
	end := strings.Index(rest, "@@")
	if end < 0 {
		return Hunk{}, 0, 0, false
	}
	parts := strings.Fields(strings.TrimSpace(rest[:end])) // ["-a,b", "+c,d"]
	if len(parts) != 2 || !strings.HasPrefix(parts[0], "-") || !strings.HasPrefix(parts[1], "+") {
		return Hunk{}, 0, 0, false
	}
	oStart, oCount, ok1 := parseHunkRange(parts[0][1:])
	nStart, nCount, ok2 := parseHunkRange(parts[1][1:])
	if !ok1 || !ok2 {
		return Hunk{}, 0, 0, false
	}
	return Hunk{OldStart: oStart, OldCount: oCount, NewStart: nStart, NewCount: nCount}, oStart, nStart, true
}

// parseHunkRange parses a "start[,count]" range fragment. count defaults to 1
// when the comma form is absent (unified-diff shorthand for a single line).
func parseHunkRange(s string) (start, count int, ok bool) {
	count = 1
	if comma := strings.IndexByte(s, ','); comma >= 0 {
		c, err := strconv.Atoi(s[comma+1:])
		if err != nil {
			return 0, 0, false
		}
		count = c
		s = s[:comma]
	}
	start, err := strconv.Atoi(s)
	if err != nil {
		return 0, 0, false
	}
	return start, count, true
}

// editOp represents an edit operation
type editOp struct {
	Type    LineType
	Content string
	OldNum  int
	NewNum  int
}

// splitLines splits content into lines, handling empty content
func splitLines(content string) []string {
	if content == "" {
		return nil
	}
	lines := strings.Split(content, "\n")
	// Remove trailing empty string from final newline
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// computeEditScript computes the edit operations using LCS
func computeEditScript(oldLines, newLines []string) []editOp {
	m := len(oldLines)
	n := len(newLines)

	// Build LCS table
	lcs := make([][]int, m+1)
	for i := range lcs {
		lcs[i] = make([]int, n+1)
	}
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if oldLines[i-1] == newLines[j-1] {
				lcs[i][j] = lcs[i-1][j-1] + 1
			} else if lcs[i-1][j] >= lcs[i][j-1] {
				lcs[i][j] = lcs[i-1][j]
			} else {
				lcs[i][j] = lcs[i][j-1]
			}
		}
	}

	// Backtrack to produce edit script
	var ops []editOp
	i, j := m, n
	for i > 0 || j > 0 {
		if i > 0 && j > 0 && oldLines[i-1] == newLines[j-1] {
			ops = append(ops, editOp{Type: Context, Content: oldLines[i-1], OldNum: i, NewNum: j})
			i--
			j--
		} else if j > 0 && (i == 0 || lcs[i][j-1] >= lcs[i-1][j]) {
			ops = append(ops, editOp{Type: Added, Content: newLines[j-1], OldNum: 0, NewNum: j})
			j--
		} else {
			ops = append(ops, editOp{Type: Removed, Content: oldLines[i-1], OldNum: i, NewNum: 0})
			i--
		}
	}

	// Reverse (we built it backwards)
	for left, right := 0, len(ops)-1; left < right; left, right = left+1, right-1 {
		ops[left], ops[right] = ops[right], ops[left]
	}

	return ops
}

// buildHunks groups edit operations into hunks with surrounding context lines
func buildHunks(ops []editOp, contextLines int) []Hunk {
	if len(ops) == 0 {
		return nil
	}

	// Find indices of changed lines
	var changeIndices []int
	for i, op := range ops {
		if op.Type != Context {
			changeIndices = append(changeIndices, i)
		}
	}

	if len(changeIndices) == 0 {
		return nil // no changes
	}

	// Build ranges: each change gets contextLines before and after
	type rangeT struct{ start, end int }
	var ranges []rangeT
	for _, idx := range changeIndices {
		start := idx - contextLines
		if start < 0 {
			start = 0
		}
		end := idx + contextLines
		if end >= len(ops) {
			end = len(ops) - 1
		}
		ranges = append(ranges, rangeT{start, end})
	}

	// Merge overlapping ranges
	var merged []rangeT
	current := ranges[0]
	for i := 1; i < len(ranges); i++ {
		if ranges[i].start <= current.end+1 {
			if ranges[i].end > current.end {
				current.end = ranges[i].end
			}
		} else {
			merged = append(merged, current)
			current = ranges[i]
		}
	}
	merged = append(merged, current)

	// Build hunks from merged ranges
	var hunks []Hunk
	for _, r := range merged {
		hunk := Hunk{}
		for i := r.start; i <= r.end; i++ {
			op := ops[i]
			hunk.Lines = append(hunk.Lines, Line{
				Type:    op.Type,
				Content: op.Content,
				OldNum:  op.OldNum,
				NewNum:  op.NewNum,
			})
		}

		// Calculate hunk header numbers
		if len(hunk.Lines) > 0 {
			for _, line := range hunk.Lines {
				if line.OldNum > 0 {
					hunk.OldStart = line.OldNum
					break
				}
			}
			for _, line := range hunk.Lines {
				if line.NewNum > 0 {
					hunk.NewStart = line.NewNum
					break
				}
			}
			for _, line := range hunk.Lines {
				if line.Type != Added {
					hunk.OldCount++
				}
				if line.Type != Removed {
					hunk.NewCount++
				}
			}
		}

		hunks = append(hunks, hunk)
	}

	return hunks
}

// CountCommentsPerFile counts the total number of comments per file path
// across all threads. Threads without a file context (general comments) are excluded.
// Returns a map from file path to total comment count.
func CountCommentsPerFile(threads []azdevops.Thread) map[string]int {
	result := make(map[string]int)
	for _, thread := range threads {
		if thread.ThreadContext == nil || thread.ThreadContext.FilePath == "" {
			continue
		}
		result[thread.ThreadContext.FilePath] += len(thread.Comments)
	}
	return result
}

// FilterGeneralThreads returns only threads without a file context (general PR comments).
// Threads with a ThreadContext that has an empty FilePath are also considered general.
func FilterGeneralThreads(threads []azdevops.Thread) []azdevops.Thread {
	var result []azdevops.Thread
	for _, thread := range threads {
		if thread.ThreadContext == nil || thread.ThreadContext.FilePath == "" {
			result = append(result, thread)
		}
	}
	return result
}

// CountGeneralComments counts the total number of comments across all general threads
// (threads without a file context).
func CountGeneralComments(threads []azdevops.Thread) int {
	count := 0
	for _, thread := range threads {
		if thread.ThreadContext == nil || thread.ThreadContext.FilePath == "" {
			count += len(thread.Comments)
		}
	}
	return count
}

// MapThreadsToLines maps PR threads to line numbers for a specific file.
// Returns a map from new-file line number to threads at that line.
func MapThreadsToLines(threads []azdevops.Thread, filePath string) map[int][]azdevops.Thread {
	result := make(map[int][]azdevops.Thread)
	for _, thread := range threads {
		if thread.ThreadContext == nil {
			continue
		}
		if thread.ThreadContext.FilePath != filePath {
			continue
		}
		if thread.ThreadContext.RightFileStart == nil {
			continue
		}
		line := thread.ThreadContext.RightFileStart.Line
		result[line] = append(result[line], thread)
	}
	return result
}

// --- provider.Thread variants ---
// The following functions mirror the azdevops.Thread helpers above but operate
// on the neutral provider.Thread type. They are used by the PR views after the
// migration to provider.Provider (task 7).

// CountCommentsPerFileP counts the total number of comments per file path
// across all provider threads. General comment threads (FilePath == "") are excluded.
func CountCommentsPerFileP(threads []provider.Thread) map[string]int {
	result := make(map[string]int)
	for _, thread := range threads {
		if thread.FilePath == "" {
			continue
		}
		result[thread.FilePath] += len(thread.Comments)
	}
	return result
}

// FilterGeneralThreadsP returns only provider threads without a file context
// (i.e. FilePath == ""), which are general PR comment threads.
func FilterGeneralThreadsP(threads []provider.Thread) []provider.Thread {
	var result []provider.Thread
	for _, thread := range threads {
		if thread.FilePath == "" {
			result = append(result, thread)
		}
	}
	return result
}

// MapThreadsToLinesP maps provider threads to line numbers for a specific file.
// Returns a map from new-file line number to provider threads at that line.
func MapThreadsToLinesP(threads []provider.Thread, filePath string) map[int][]provider.Thread {
	result := make(map[int][]provider.Thread)
	for _, thread := range threads {
		if thread.FilePath != filePath {
			continue
		}
		if thread.Line == 0 {
			continue
		}
		result[thread.Line] = append(result[thread.Line], thread)
	}
	return result
}

// FilterSystemThreadsP filters out system-generated provider threads
// (e.g. Microsoft.VisualStudio service comments, vote notifications, policy updates).
func FilterSystemThreadsP(threads []provider.Thread) []provider.Thread {
	filtered := make([]provider.Thread, 0, len(threads))
	for _, thread := range threads {
		if !isSystemThreadP(thread) {
			filtered = append(filtered, thread)
		}
	}
	return filtered
}

// isVotedCommentP checks whether a comment content is a vote notification
// (e.g. "John Doe voted 10", "Jane Smith voted -5").
// Duplicated from azdevops.isVotedComment to avoid a cross-package dependency.
func isVotedCommentP(content string) bool {
	idx := strings.Index(content, "voted")
	if idx == -1 {
		return false
	}
	after := strings.TrimSpace(content[idx+5:])
	if len(after) == 0 {
		return false
	}
	if after[0] == '-' && len(after) > 1 {
		after = after[1:]
	}
	return len(after) > 0 && after[0] >= '0' && after[0] <= '9'
}

func isSystemThreadP(thread provider.Thread) bool {
	for _, comment := range thread.Comments {
		if strings.HasPrefix(comment.AuthorName, "Microsoft.VisualStudio") {
			return true
		}
		content := strings.TrimSpace(comment.Content)
		if strings.HasPrefix(content, "Microsoft.VisualStudio") {
			return true
		}
		if strings.Contains(content, "Policy status has been updated") {
			return true
		}
		if isVotedCommentP(content) {
			return true
		}
	}
	return false
}
