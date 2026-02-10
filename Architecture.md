# azdo-tui — Azure DevOps Pipeline Dashboard & PR Tool

A terminal UI for monitoring pipelines, managing pull requests, and interacting with Azure DevOps without leaving your terminal.

## Project Structure

```
azdo-tui/
├── cmd/
│   └── azdo-tui/
│       └── main.go                 # Entry point, config loading, app bootstrap
│
├── internal/
│   ├── app/
│   │   ├── app.go                  # Root bubbletea model, tab navigation
│   │   └── keymap.go               # Global keybindings
│   │
│   ├── ui/
│   │   ├── styles/
│   │   │   └── theme.go            # lipgloss styles, color palette
│   │   │
│   │   ├── components/
│   │   │   ├── statusbar.go        # Bottom status bar (connection, help hints)
│   │   │   ├── header.go           # Top bar with project/org info + tabs
│   │   │   ├── table.go            # Reusable styled table wrapper
│   │   │   ├── logviewer.go        # Scrollable log viewport
│   │   │   ├── spinner.go          # Loading indicators
│   │   │   └── modal.go            # Confirmation dialogs
│   │   │
│   │   ├── pipelines/
│   │   │   ├── list.go             # Pipeline runs list view (main dashboard)
│   │   │   ├── detail.go           # Single run detail: stages, jobs, tasks
│   │   │   ├── logs.go             # Live log tail for a specific task
│   │   │   └── trigger.go          # Trigger a new run (branch picker, params)
│   │   │
│   │   ├── pullrequests/
│   │   │   ├── list.go             # Open PRs across repos
│   │   │   ├── detail.go           # PR detail: description, reviewers, threads
│   │   │   ├── diff.go             # File diff viewer (simplified)
│   │   │   └── actions.go          # Approve, reject, add comment
│   │   │
│   │   └── workitems/
│   │       ├── board.go            # Kanban-style board view
│   │       └── detail.go           # Work item detail + linked PRs
│   │
│   ├── azdevops/
│   │   ├── client.go               # HTTP client, auth, base URL config
│   │   ├── pipelines.go            # Pipeline runs, definitions, logs API
│   │   ├── git.go                  # Repos, PRs, diffs API
│   │   ├── wit.go                  # Work item tracking API
│   │   └── types.go                # Shared API response types
│   │
│   ├── polling/
│   │   ├── poller.go               # Background polling manager
│   │   └── events.go               # Event types sent to bubbletea
│   │
│   └── config/
│       ├── config.go               # Config struct + loading logic
│       └── keyring.go              # PAT storage (system keyring integration)
│
├── go.mod
├── go.sum
├── Makefile
├── .goreleaser.yaml                # Cross-platform releases
└── README.md
```

## Architecture Overview

```
┌─────────────────────────────────────────────────────────┐
│                     Terminal (TUI)                       │
│  ┌───────────┬──────────────┬────────────┐              │
│  │ Pipelines │ Pull Requests│ Work Items │  ← Tab Nav   │
│  └───────────┴──────────────┴────────────┘              │
│  ┌─────────────────────────────────────────────────────┐ │
│  │                                                     │ │
│  │              Active View (bubbletea)                │ │
│  │                                                     │ │
│  │  List → Detail → Logs/Diff (drill-down navigation) │ │
│  │                                                     │ │
│  └─────────────────────────────────────────────────────┘ │
│  ┌─────────────────────────────────────────────────────┐ │
│  │  Status Bar: org/project · connected · ? help       │ │
│  └─────────────────────────────────────────────────────┘ │
└────────────────────┬────────────────────────────────────┘
                     │
          ┌──────────┴──────────┐
          │   Polling Manager   │  ← Background goroutines
          │  (configurable      │     send tea.Msg to
          │   intervals)        │     bubbletea program
          └──────────┬──────────┘
                     │
          ┌──────────┴──────────┐
          │   Azure DevOps      │
          │   REST API Client   │  ← PAT auth, rate limiting,
          │                     │     response caching
          └─────────────────────┘
```

## Core Dependencies

```go
// go.mod
module github.com/yourusername/azdo-tui

go 1.23

require (
    github.com/charmbracelet/bubbletea    v1.3.4
    github.com/charmbracelet/lipgloss     v1.1.0
    github.com/charmbracelet/bubbles      v0.20.0
    github.com/zalando/go-keyring         v0.2.6
    github.com/spf13/viper               v1.19.0
    github.com/spf13/cobra               v1.8.1
)
```

## Key Design Decisions

### 1. Bubbletea Model Hierarchy

The app uses a nested model pattern. The root `App` model manages tabs and delegates
messages to the active child model.

```go
// internal/app/app.go
package app

import (
    tea "github.com/charmbracelet/bubbletea"
    "azdo-tui/internal/ui/pipelines"
    "azdo-tui/internal/ui/pullrequests"
    "azdo-tui/internal/ui/workitems"
)

type Tab int

const (
    TabPipelines Tab = iota
    TabPullRequests
    TabWorkItems
)

type Model struct {
    activeTab      Tab
    pipelines      pipelines.Model
    pullRequests   pullrequests.Model
    workItems      workitems.Model
    statusBar      components.StatusBarModel
    width, height  int
    client         *azdevops.Client
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyMsg:
        switch msg.String() {
        case "1":
            m.activeTab = TabPipelines
        case "2":
            m.activeTab = TabPullRequests
        case "3":
            m.activeTab = TabWorkItems
        case "q", "ctrl+c":
            return m, tea.Quit
        }
    case tea.WindowSizeMsg:
        m.width = msg.Width
        m.height = msg.Height
    }

    // Delegate to active tab
    var cmd tea.Cmd
    switch m.activeTab {
    case TabPipelines:
        m.pipelines, cmd = m.pipelines.Update(msg)
    case TabPullRequests:
        m.pullRequests, cmd = m.pullRequests.Update(msg)
    case TabWorkItems:
        m.workItems, cmd = m.workItems.Update(msg)
    }
    return m, cmd
}
```

### 2. Azure DevOps API Client

Thin wrapper over the REST API. No need for the full Microsoft SDK — the API is
straightforward and keeping it lean avoids pulling in large dependency trees.

```go
// internal/azdevops/client.go
package azdevops

import (
    "encoding/json"
    "fmt"
    "net/http"
    "time"
)

type Client struct {
    baseURL    string   // https://dev.azure.com/{org}
    project    string
    pat        string
    httpClient *http.Client
}

func NewClient(org, project, pat string) *Client {
    return &Client{
        baseURL:    fmt.Sprintf("https://dev.azure.com/%s", org),
        project:    project,
        pat:        pat,
        httpClient: &http.Client{Timeout: 15 * time.Second},
    }
}

func (c *Client) get(path string, result any) error {
    url := fmt.Sprintf("%s/%s/_apis/%s", c.baseURL, c.project, path)
    req, _ := http.NewRequest("GET", url, nil)
    req.SetBasicAuth("", c.pat)
    req.Header.Set("Accept", "application/json")

    resp, err := c.httpClient.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("API returned %d", resp.StatusCode)
    }
    return json.NewDecoder(resp.Body).Decode(result)
}
```

```go
// internal/azdevops/pipelines.go
package azdevops

import "time"

type PipelineRun struct {
    ID         int       `json:"id"`
    Name       string    `json:"name"`
    State      string    `json:"state"`      // "inProgress", "completed", "canceling"
    Result     string    `json:"result"`      // "succeeded", "failed", "canceled"
    Pipeline   Pipeline  `json:"pipeline"`
    CreatedDate time.Time `json:"createdDate"`
    FinishedDate time.Time `json:"finishedDate"`
    SourceBranch string
}

type Pipeline struct {
    ID   int    `json:"id"`
    Name string `json:"name"`
}

func (c *Client) ListPipelineRuns(top int) ([]PipelineRun, error) {
    var result struct {
        Value []PipelineRun `json:"value"`
    }
    path := fmt.Sprintf("pipelines?api-version=7.1&$top=%d", top)
    // Note: actual endpoint is build/builds for richer data
    err := c.get(fmt.Sprintf("build/builds?api-version=7.1&$top=%d", top), &result)
    return result.Value, err
}

func (c *Client) GetBuildTimeline(buildID int) (*Timeline, error) {
    var result Timeline
    path := fmt.Sprintf("build/builds/%d/timeline?api-version=7.1", buildID)
    err := c.get(path, &result)
    return &result, err
}

func (c *Client) GetBuildLogs(buildID, logID int) (string, error) {
    // Returns plain text log content
    // ...
}
```

### 3. Background Polling with tea.Msg

Polling runs in goroutines and sends updates as bubbletea messages, keeping the
UI reactive without blocking.

```go
// internal/polling/poller.go
package polling

import (
    "time"
    tea "github.com/charmbracelet/bubbletea"
    "azdo-tui/internal/azdevops"
)

// Messages sent to bubbletea
type PipelineRunsUpdated struct {
    Runs []azdevops.PipelineRun
    Err  error
}

type PRsUpdated struct {
    PRs []azdevops.PullRequest
    Err error
}

// Returns a tea.Cmd that polls pipeline runs on an interval
func PollPipelineRuns(client *azdevops.Client, interval time.Duration) tea.Cmd {
    return tea.Every(interval, func(t time.Time) tea.Msg {
        runs, err := client.ListPipelineRuns(25)
        return PipelineRunsUpdated{Runs: runs, Err: err}
    })
}

// One-shot fetch (for initial load or manual refresh)
func FetchPipelineRuns(client *azdevops.Client) tea.Cmd {
    return func() tea.Msg {
        runs, err := client.ListPipelineRuns(25)
        return PipelineRunsUpdated{Runs: runs, Err: err}
    }
}
```

### 4. Pipeline List View

```go
// internal/ui/pipelines/list.go
package pipelines

import (
    "fmt"
    tea "github.com/charmbracelet/bubbletea"
    "github.com/charmbracelet/bubbles/table"
    "github.com/charmbracelet/lipgloss"
    "azdo-tui/internal/azdevops"
    "azdo-tui/internal/polling"
)

type Model struct {
    table    table.Model
    runs     []azdevops.PipelineRun
    client   *azdevops.Client
    loading  bool
    err      error
    // sub-view for drill-down
    detail   *DetailModel
    viewMode ViewMode
}

type ViewMode int
const (
    ViewList ViewMode = iota
    ViewDetail
    ViewLogs
)

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
    switch msg := msg.(type) {
    case polling.PipelineRunsUpdated:
        m.loading = false
        if msg.Err != nil {
            m.err = msg.Err
            return m, nil
        }
        m.runs = msg.Runs
        m.table.SetRows(runsToRows(m.runs))
        return m, nil

    case tea.KeyMsg:
        if m.viewMode == ViewList {
            switch msg.String() {
            case "enter":
                // Drill into selected run
                selected := m.table.Cursor()
                if selected < len(m.runs) {
                    m.viewMode = ViewDetail
                    m.detail = NewDetailModel(m.client, m.runs[selected])
                    return m, m.detail.Init()
                }
            case "r":
                // Manual refresh
                m.loading = true
                return m, polling.FetchPipelineRuns(m.client)
            }
        }
        if m.viewMode == ViewDetail {
            if msg.String() == "esc" {
                m.viewMode = ViewList
                m.detail = nil
                return m, nil
            }
        }
    }

    if m.viewMode == ViewList {
        var cmd tea.Cmd
        m.table, cmd = m.table.Update(msg)
        return m, cmd
    }
    return m, nil
}

func runsToRows(runs []azdevops.PipelineRun) []table.Row {
    rows := make([]table.Row, len(runs))
    for i, r := range runs {
        rows[i] = table.Row{
            statusIcon(r.State, r.Result),
            r.Pipeline.Name,
            r.SourceBranch,
            r.CreatedDate.Format("15:04:05"),
            duration(r),
        }
    }
    return rows
}

func statusIcon(state, result string) string {
    switch {
    case state == "inProgress":
        return lipgloss.NewStyle().Foreground(lipgloss.Color("33")).Render("⟳")  // blue
    case result == "succeeded":
        return lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("✓")  // green
    case result == "failed":
        return lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("✗") // red
    default:
        return lipgloss.NewStyle().Foreground(lipgloss.Color("243")).Render("○") // gray
    }
}
```

### 5. Configuration

```yaml
# ~/.config/azdo-tui/config.yaml
organization: my-org
project: my-project
# PAT is stored in system keyring, set via: azdo-tui auth login
polling:
  pipelines: 15s
  pullrequests: 30s
theme: dark  # dark | light | dracula | catppuccin
```

```go
// internal/config/config.go
package config

import "github.com/spf13/viper"

type Config struct {
    Organization string        `mapstructure:"organization"`
    Project      string        `mapstructure:"project"`
    Polling      PollingConfig `mapstructure:"polling"`
    Theme        string        `mapstructure:"theme"`
}

type PollingConfig struct {
    Pipelines    string `mapstructure:"pipelines"`
    PullRequests string `mapstructure:"pullrequests"`
}

func Load() (*Config, error) {
    viper.SetConfigName("config")
    viper.SetConfigType("yaml")
    viper.AddConfigPath("$HOME/.config/azdo-tui")
    viper.SetDefault("polling.pipelines", "15s")
    viper.SetDefault("polling.pullrequests", "30s")
    viper.SetDefault("theme", "dark")

    if err := viper.ReadInConfig(); err != nil {
        return nil, err
    }
    var cfg Config
    return &cfg, viper.Unmarshal(&cfg)
}
```

## Entry Point

```go
// cmd/azdo-tui/main.go
package main

import (
    "fmt"
    "os"

    tea "github.com/charmbracelet/bubbletea"
    "azdo-tui/internal/app"
    "azdo-tui/internal/azdevops"
    "azdo-tui/internal/config"
)

func main() {
    cfg, err := config.Load()
    if err != nil {
        fmt.Fprintf(os.Stderr, "Config error: %v\n", err)
        os.Exit(1)
    }

    pat, err := config.GetPAT(cfg.Organization)
    if err != nil {
        fmt.Fprintf(os.Stderr, "Auth error: %v\nRun: azdo-tui auth login\n", err)
        os.Exit(1)
    }

    client := azdevops.NewClient(cfg.Organization, cfg.Project, pat)
    model := app.NewModel(client, cfg)

    p := tea.NewProgram(model, tea.WithAltScreen())
    if _, err := p.Run(); err != nil {
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
        os.Exit(1)
    }
}
```

## Suggested Build Order

Start small, iterate fast:

1. **Milestone 1 — Skeleton** (~1-2 hours)
   - Config loading + PAT auth (keyring or env var)
   - API client with `ListPipelineRuns`
   - Basic bubbletea app with a single table view
   - See pipeline runs in your terminal ✓

2. **Milestone 2 — Drill-down** (~2-3 hours)
   - Build timeline (stages/jobs) detail view
   - Log viewer with scrollable viewport
   - Enter/Esc navigation between list → detail → logs

3. **Milestone 3 — Live updates** (~1-2 hours)
   - Background polling with `tea.Every`
   - Status bar with connection state
   - Auto-refresh indicator

4. **Milestone 4 — PR tab** (~3-4 hours)
   - PR list across repos
   - PR detail with threads
   - Approve/reject actions
   - Optional: simplified diff viewer

5. **Milestone 5 — Polish** (~2-3 hours)
   - Theming (catppuccin, dracula, etc.)
   - Help overlay (`?` key)
   - Trigger pipeline runs
   - Work items tab (if desired)
   - `.goreleaser.yaml` for cross-platform builds

## Azure DevOps API Endpoints Reference

| Feature | Endpoint | API Version |
|---------|----------|-------------|
| List builds | `GET {project}/_apis/build/builds` | 7.1 |
| Build timeline | `GET {project}/_apis/build/builds/{id}/timeline` | 7.1 |
| Build logs | `GET {project}/_apis/build/builds/{id}/logs/{logId}` | 7.1 |
| List PRs | `GET {project}/_apis/git/repositories/{repo}/pullrequests` | 7.1 |
| PR threads | `GET {project}/_apis/git/repositories/{repo}/pullrequests/{id}/threads` | 7.1 |
| Update PR | `PATCH {project}/_apis/git/repositories/{repo}/pullrequests/{id}` | 7.1 |
| Work items (WIQL) | `POST {project}/_apis/wit/wiql` | 7.1 |
| Work item by ID | `GET {project}/_apis/wit/workitems/{id}` | 7.1 |

All endpoints use base URL: `https://dev.azure.com/{organization}/`
Auth: Basic auth with empty username and PAT as password.