package diff

import (
	"strings"

	"github.com/Elpulgo/azdo/internal/azdevops"
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
