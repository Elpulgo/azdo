# Git Diff View Architecture — PR Detail Tab

## Context

The PR detail view currently shows description, reviewers, and comment threads as a flat scrollable list. This document details the architecture for adding a full git diff view per changed file with inline comments, comment creation, reply, and thread resolution — all within the TUI using theme-aware diff colors.

---

## 1. Current Architecture

### PR Detail Flow
```
PR List (table) → [enter] → PR Detail (viewport with threads) → [esc] → back to list
```

### Key Existing Components
- **`internal/azdevops/client.go`** — HTTP client with `get()`, `post()`, `put()`, `doRequest()` methods. Basic Auth with PAT. No `patch()` method.
- **`internal/azdevops/git.go`** — Types: `PullRequest`, `Thread`, `Comment`, `ThreadContext`, `FilePosition`. API methods: `ListPullRequests`, `GetPRThreads`, `AddPRComment`, `VotePullRequest`.
- **`internal/ui/pullrequests/detail.go`** — `DetailModel` with viewport, thread rendering, selection navigation. Renders: header → description → reviewers → comment threads.
- **`internal/ui/pullrequests/list.go`** — Uses generic `listview.Model[PullRequest]` with `detailAdapter` wrapping `DetailModel`.
- **`internal/ui/styles/styles.go`** — 59-field `Styles` struct generated from `Theme`. Used by all components.
- **`internal/ui/components/listview/listview.go`** — Generic list with `DetailView` interface: `Update()`, `View()`, `SetSize()`, `GetContextItems()`, `GetScrollPercent()`, `GetStatusMessage()`.

### View Hierarchy Pattern (from pipelines)
The pipelines module demonstrates the three-level view pattern: `list.go` manages `ViewList → ViewDetail → ViewLogs` transitions. The PR module will follow this exact pattern for `ViewList → ViewDetail → ViewDiff`.

---

## 2. Target Architecture

### New View Flow
```
PR List → [enter] → PR Detail (description + reviewers + comments)
                         ↓ [d]
                    Diff File List (changed files with change type icons)
                         ↓ [enter]
                    File Diff View (colored diff + inline comments)
                         ↓ [c/p/x]
                    Comment/Reply/Resolve actions
```

### Component Diagram
```
pullrequests/list.go (Model)
├── listview.Model[PullRequest]     (existing)
│   └── detailAdapter → DetailModel (existing — description + reviewers + threads)
└── DiffModel (NEW)
    ├── DiffFileList mode            (selectable list of changed files)
    └── DiffFileView mode            (scrollable diff with inline comments + input)
        ├── diff.ComputeDiff()       (pure diff engine)
        └── diff.MapThreadsToLines() (comment positioning)
```

---

## 3. Azure DevOps API Additions

### 3.1 New Client Method: `patch()`

Add to `client.go` following the `put()`/`post()` pattern:
```go
func (c *Client) patch(path string, body io.Reader) ([]byte, error) {
    return c.doRequest("PATCH", path, body)
}
```

### 3.2 PR Iterations

**Endpoint:** `GET /git/repositories/{repoId}/pullRequests/{prId}/iterations?api-version=7.1`

Each push to the PR source branch creates a new iteration. We need the latest iteration ID to fetch the changed files.

```go
type Iteration struct {
    ID          int       `json:"id"`
    Description string    `json:"description"`
    CreatedDate time.Time `json:"createdDate"`
}

type IterationsResponse struct {
    Count int         `json:"count"`
    Value []Iteration `json:"value"`
}

func (c *Client) GetPRIterations(repositoryID string, pullRequestID int) ([]Iteration, error)
```

### 3.3 Iteration Changes (Changed Files)

**Endpoint:** `GET /git/repositories/{repoId}/pullRequests/{prId}/iterations/{iterationId}/changes?api-version=7.1&$compareTo=0`

The `$compareTo=0` parameter compares against the base (target branch), giving the full diff scope.

```go
type IterationChange struct {
    ChangeID     int        `json:"changeId"`
    Item         ChangeItem `json:"item"`
    ChangeType   string     `json:"changeType"` // "add", "edit", "delete", "rename"
    OriginalPath string     `json:"originalPath,omitempty"`
}

type ChangeItem struct {
    ObjectID string `json:"objectId"`
    Path     string `json:"path"`
}

type IterationChangesResponse struct {
    ChangeEntries []IterationChange `json:"changeEntries"`
}

func (c *Client) GetPRIterationChanges(repositoryID string, pullRequestID int, iterationID int) ([]IterationChange, error)
```

### 3.4 File Content at Branch Version

**Endpoint:** `GET /git/repositories/{repoId}/items?path={path}&versionType=branch&version={branchShortName}&api-version=7.1`

Returns raw file content. Accept header set to `text/plain` for raw content (not JSON-wrapped).

```go
func (c *Client) GetFileContent(repositoryID string, path string, branchName string) (string, error)
```

**Design decision:** Fetch file at both source and target branch, compute diff locally. This approach:
- Gives full control over context lines (exactly +5)
- Avoids inconsistencies in Azure DevOps diff API
- Enables accurate line-number mapping for inline comments
- Works reliably across all change types (add, edit, delete, rename)

### 3.5 Reply to Thread

**Endpoint:** `POST /git/repositories/{repoId}/pullRequests/{prId}/threads/{threadId}/comments?api-version=7.1`

```go
func (c *Client) ReplyToThread(repositoryID string, pullRequestID int, threadID int, content string) (*Comment, error)
```

Payload: `{"content": "...", "parentCommentId": 1, "commentType": "text"}`

### 3.6 Update Thread Status (Resolve)

**Endpoint:** `PATCH /git/repositories/{repoId}/pullRequests/{prId}/threads/{threadId}?api-version=7.1`

```go
func (c *Client) UpdateThreadStatus(repositoryID string, pullRequestID int, threadID int, status string) error
```

Payload: `{"status": "fixed"}` (or "active", "wontFix", "closed", "pending")

### 3.7 Create Code Comment (Thread with File Context)

**Endpoint:** `POST /git/repositories/{repoId}/pullRequests/{prId}/threads?api-version=7.1`

```go
func (c *Client) AddPRCodeComment(repositoryID string, pullRequestID int, filePath string, line int, content string) (*Thread, error)
```

Payload includes `threadContext` with `filePath`, `rightFileStart`, and `rightFileEnd` to attach the comment to a specific line.

---

## 4. Diff Computation Engine

### New Package: `internal/diff/`

Pure logic with no UI or API dependencies — highly testable.

### 4.1 Types

```go
package diff

type FileDiff struct {
    Path       string
    ChangeType string // "add", "edit", "delete", "rename"
    OldPath    string // for renames
    Hunks      []Hunk
}

type Hunk struct {
    OldStart int    // starting line in old file
    OldCount int    // number of lines from old file
    NewStart int    // starting line in new file
    NewCount int    // number of lines in new file
    Lines    []Line
}

type Line struct {
    Type    LineType
    Content string
    OldNum  int // line number in old file (0 if added)
    NewNum  int // line number in new file (0 if removed)
}

type LineType int
const (
    Context LineType = iota
    Added
    Removed
)
```

### 4.2 Core Algorithm: `ComputeDiff`

```go
func ComputeDiff(oldContent, newContent string, contextLines int) []Hunk
```

Algorithm:
1. Split both files into line slices
2. Compute edit script using LCS (Longest Common Subsequence)
3. Classify each line as Added, Removed, or Context
4. Group into hunks with `contextLines` (5) lines of surrounding context
5. Merge overlapping hunks (when changes are close together)
6. Assign proper line numbers (OldNum, NewNum) to each line

### 4.3 Comment Positioning: `MapThreadsToLines`

```go
func MapThreadsToLines(threads []azdevops.Thread, filePath string) map[int][]azdevops.Thread
```

Filters threads by `ThreadContext.FilePath == filePath`, maps each to `ThreadContext.RightFileStart.Line` (new-file line number). Returns a map for O(1) lookup during rendering.

---

## 5. Diff Styles

### New Fields in `Styles` struct (`styles.go`)

```go
// Diff styles
DiffAdded      lipgloss.Style // Success color (green) — added lines
DiffRemoved    lipgloss.Style // Error color (red) — removed lines
DiffContext    lipgloss.Style // ForegroundMuted — unchanged context lines
DiffHeader     lipgloss.Style // Primary + BackgroundAlt + Bold — file path header
DiffHunkHeader lipgloss.Style // Info color — @@ hunk markers
DiffLineNum    lipgloss.Style // ForegroundMuted, right-aligned — line number gutter
```

Generated in `NewStyles()` using existing theme colors:
- `DiffAdded` → `theme.Success` (green in all themes)
- `DiffRemoved` → `theme.Error` (red in all themes)
- `DiffContext` → `theme.ForegroundMuted` (gray)
- `DiffHeader` → `theme.Primary` fg + `theme.BackgroundAlt` bg + Bold
- `DiffHunkHeader` → `theme.Info` (blue)
- `DiffLineNum` → `theme.ForegroundMuted`, width 5, right-aligned

This ensures visual synergy across all 6 built-in themes (dark, gruvbox, nord, dracula, catppuccin, github) and any custom themes.

---

## 6. DiffModel UI Component

### New File: `internal/ui/pullrequests/diffview.go`

### 6.1 State Structure

```go
type DiffViewMode int
const (
    DiffFileList DiffViewMode = iota // selectable list of changed files
    DiffFileView                     // scrollable diff for single file
)

type InputMode int
const (
    InputNone    InputMode = iota
    InputNewComment        // creating new code comment on a line
    InputReply             // replying to existing thread
)

type DiffModel struct {
    client        *azdevops.Client
    pr            azdevops.PullRequest
    threads       []azdevops.Thread        // all PR threads

    // File list state
    changedFiles  []azdevops.IterationChange
    fileIndex     int

    // File diff state
    currentFile   *azdevops.IterationChange
    currentDiff   *diff.FileDiff
    fileThreads   map[int][]azdevops.Thread // newLineNum → threads

    // Flattened rendering
    diffLines     []diffLine               // all renderable lines
    selectedLine  int                      // cursor in diffLines

    // Input
    inputMode     InputMode
    textInput     textinput.Model
    replyThreadID int

    // Layout
    viewMode      DiffViewMode
    viewport      viewport.Model
    width, height int
    ready         bool
    loading       bool
    err           error
    statusMessage string
    spinner       *components.LoadingIndicator
    styles        *styles.Styles
}
```

### 6.2 Flattened Line Model

The diff view flattens hunks + inline comments into a single scrollable list:

```go
type diffLine struct {
    Type       diffLineType
    Content    string
    OldNum     int
    NewNum     int
    ThreadID   int    // non-zero if this is a comment line
    CommentIdx int
}

type diffLineType int
const (
    diffLineContext    diffLineType = iota
    diffLineAdded
    diffLineRemoved
    diffLineHunkHeader
    diffLineComment    // inline comment display
    diffLineFileHeader
)
```

### 6.3 Key Bindings

**DiffFileList mode:**
| Key | Action |
|-----|--------|
| `up/down` `j/k` | Navigate file list |
| `enter` | Open selected file diff |
| `esc` | Back to PR detail |
| `r` | Refresh |

**DiffFileView mode:**
| Key | Action |
|-----|--------|
| `up/down` `j/k` | Scroll through diff lines |
| `pgup/pgdown` | Page scroll |
| `c` | Create new comment on current line |
| `p` | Reply to nearest thread |
| `x` | Resolve nearest thread |
| `n/N` | Jump to next/previous comment |
| `esc` | Back to file list |

**Input mode (InputNewComment / InputReply):**
| Key | Action |
|-----|--------|
| typing | Text input |
| `enter` | Submit comment/reply |
| `esc` | Cancel input |

### 6.4 Data Loading Flow

```
Init()
  -> fetchChangedFiles()
    -> GetPRIterations() -> get latest iteration ID
    -> GetPRIterationChanges(latestIterationID) -> file list
    -> changedFilesMsg{changes, err}

[enter on file]
  -> fetchFileDiff(change)
    -> GetFileContent(path, targetBranch) -> old content
    -> GetFileContent(path, sourceBranch) -> new content
    -> diff.ComputeDiff(old, new, 5)       -> hunks
    -> diff.MapThreadsToLines(threads, path) -> positioned comments
    -> fileDiffMsg{diff, fileThreads, err}
```

File diffs are lazy-loaded — only fetched when a file is selected.

### 6.5 Rendering

**File list mode** — renders each file with change type icon:
```
  + src/new-file.go                    (Success style — green)
  ~ src/modified-file.go               (Info style — blue)
  - src/deleted-file.go                (Error style — red)
  -> src/old-name.go -> src/new-name.go  (Warning style — yellow)
```

**File diff mode** — renders colored diff with line numbers and inline comments:
```
+-- src/internal/app/handler.go ---------------------+  <- DiffHeader
|                                                      |
| @@ -10,7 +10,9 @@                                   |  <- DiffHunkHeader
|   10   10    func handleRequest(ctx context.Context) |  <- DiffContext
|   11   11        logger := ctx.Value("logger")       |  <- DiffContext
|   12        -     result := process(input)            |  <- DiffRemoved (red)
|        12   +     result, err := process(input)       |  <- DiffAdded (green)
|        13   +     if err != nil {                     |  <- DiffAdded (green)
|        14   +         return err                      |  <- DiffAdded (green)
|        15   +     }                                   |  <- DiffAdded (green)
|                                                      |
|         * Active  ../app/handler.go:12               |  <- Inline comment thread
|         John: Should we log this error?              |
|           +- Jane: Good point, will add logging       |
|                                                      |
|   13   16        return result                       |  <- DiffContext
+------------------------------------------------------+
```

Line number gutter: `old  new` — shows both old and new line numbers. For added lines, old is blank. For removed lines, new is blank.

---

## 7. Integration into PR Detail Flow

### 7.1 DetailModel Changes (`detail.go`)

- Add `d` key handler that emits `enterDiffViewMsg{}`
- Add `GetThreads()` public method to expose threads to DiffModel
- Update `GetContextItems()` to include `{Key: "d", Description: "diff"}`

### 7.2 List Model Changes (`list.go`)

Add `ViewDiff` to view mode enum and manage DiffModel lifecycle:

```go
type ViewMode int
const (
    ViewList   ViewMode = iota
    ViewDetail
    ViewDiff
)
```

The `Model.Update()` intercepts `enterDiffViewMsg` in detail mode -> creates `DiffModel` with current PR + threads -> sets `viewMode = ViewDiff`. Esc in DiffFileList mode -> returns to detail view.

The `Model.View()` delegates to `diffView.View()` when `viewMode == ViewDiff`.

Context bar, scroll percent, and status message all delegate to the active sub-view.

---

## 8. Files Summary

### New Files
| File | Purpose | Est. Lines |
|------|---------|------------|
| `internal/diff/diff.go` | Diff types + ComputeDiff + MapThreadsToLines | ~200 |
| `internal/diff/diff_test.go` | Table-driven tests for diff engine | ~300 |
| `internal/ui/pullrequests/diffview.go` | DiffModel component | ~500 |
| `internal/ui/pullrequests/diffview_test.go` | DiffModel tests | ~400 |

### Modified Files
| File | Changes |
|------|---------|
| `internal/azdevops/client.go` | Add `patch()` method |
| `internal/azdevops/git.go` | Add iteration/change types + 6 API methods |
| `internal/azdevops/git_test.go` | Tests for all new API methods |
| `internal/ui/styles/styles.go` | Add 6 diff styles to Styles struct |
| `internal/ui/pullrequests/detail.go` | Add `d` key, GetThreads(), update context items |
| `internal/ui/pullrequests/detail_test.go` | Tests for new detail behavior |
| `internal/ui/pullrequests/list.go` | Add ViewDiff mode + DiffModel lifecycle |
| `internal/ui/pullrequests/list_test.go` | Integration tests for view transitions |

---

## 9. TDD Implementation Phases

### Phase 1: API Layer
1. `patch()` method + test
2. `GetPRIterations()` + types + test
3. `GetPRIterationChanges()` + types + test
4. `GetFileContent()` + test
5. `ReplyToThread()` + test
6. `UpdateThreadStatus()` + test
7. `AddPRCodeComment()` + test

### Phase 2: Diff Engine
8. Diff types (`FileDiff`, `Hunk`, `Line`, `LineType`)
9. `ComputeDiff()` + extensive table-driven tests
10. `MapThreadsToLines()` + tests

### Phase 3: Diff Styles
11. Add 6 diff styles to `Styles` + tests across all themes

### Phase 4: DiffModel Component
12. DiffModel struct + file list mode + navigation tests
13. Data loading (fetchChangedFiles, fetchFileDiff) + message handling tests
14. File diff rendering + inline comments + tests
15. Comment/Reply/Resolve input handlers + tests

### Phase 5: Integration
16. DetailModel `d` key + GetThreads() + tests
17. List Model ViewDiff mode + transitions + tests

### Phase 6: Edge Cases
18. Binary files (show "Binary file" message)
19. Large files (>500KB threshold warning)
20. New/deleted files (all-added / all-removed diffs)
21. Renamed files (show old -> new path)
22. Empty PR (no iterations / no changes message)

---

## 10. Verification Plan

1. `go test ./...` passes at each phase
2. `go build -o azdo` succeeds
3. `go vet ./...` clean
4. Manual testing workflow:
   - Open PR detail -> press `d` -> see changed files list
   - Select file -> see colored diff with +5 context lines
   - Verify colors match active theme (try switching themes)
   - Navigate to inline comment -> verify positioned at correct line
   - Press `c` on a diff line -> type comment -> enter -> verify created via API
   - Press `p` near comment -> type reply -> enter -> verify reply created
   - Press `x` near comment -> verify thread resolved (status = "fixed")
   - Press `esc` -> back to file list -> `esc` -> back to PR detail
