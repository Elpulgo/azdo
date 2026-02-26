# azdo-tui

A Terminal User Interface (TUI) for Azure DevOps - monitor pipelines directly from your terminal.

![Tests](https://img.shields.io/github/actions/workflow/status/Elpulgo/azdo/ci.yml?label=tests)
![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)
![License](https://img.shields.io/badge/License-MIT-blue.svg)

## Features

### Multi-Tab Interface
- **Pipelines** (Tab 1): Monitor and drill into pipeline runs
- **Pull Requests** (Tab 2): View and track pull requests
- **Work Items** (Tab 3): Browse and manage work items
- Switch between tabs using `1`, `2`, `3` keys or `←`/`→` arrow keys

### Pipeline Dashboard
- View recent pipeline runs in a sortable table
- Color-coded status indicators (✓ Success, ✗ Failed, ● Running, ○ Queued)
- Live auto-refresh with configurable polling interval
- Connection status indicator in footer
- Hierarchical detail view with stages, jobs, and tasks
- Duration tracking for each step
- Full log viewer with scrollable viewport

### Pull Requests
- List view of pull requests with status indicators
- Detailed view showing PR information and metadata
- Vote on PRs directly from the detail view (approve, reject, suggestions, wait, reset)

### Work Items
- List view of work items with status and type information
- Detailed view showing work item details
- Change work item state directly from the detail view (dynamically fetches available states)

### User Experience
- Help modal with all keyboard shortcuts (press `?`)
- Secure PAT storage using system keyring
- Context-aware keybinding hints
- Graceful error handling with automatic retry
- Seven built-in themes with true color support

## Installation

### From Source

```bash
git clone https://github.com/Elpulgo/azdo.git
cd azdo
go build -o azdo-tui ./cmd/azdo-tui
```

### Using Go Install

```bash
go install github.com/Elpulgo/azdo/cmd/azdo-tui@latest
```

## Configuration

### 1. Create Configuration File

Create a configuration file at the following location:
- **Linux/macOS**: `~/.config/azdo-tui/config.yaml`
- **Windows**: `C:\Users\<username>\.config\azdo-tui\config.yaml`

```yaml
# Azure DevOps organization name (required)
organization: your-org-name

# Azure DevOps project name (required)
project: your-project-name

# Polling interval in seconds (optional, default: 60)
polling_interval: 60

# Theme (optional, default: dark)
# Available themes: dark, gruvbox, nord, dracula, catppuccin, github, retro
theme: dark
```

Or copy the example configuration:

**Linux/macOS:**
```bash
mkdir -p ~/.config/azdo-tui
cp config.yaml.example ~/.config/azdo-tui/config.yaml
```

**Windows (PowerShell):**
```powershell
New-Item -ItemType Directory -Force -Path "$env:USERPROFILE\.config\azdo-tui"
Copy-Item config.yaml.example "$env:USERPROFILE\.config\azdo-tui\config.yaml"
```

**Configuration Options:**
- `organization`: Your Azure DevOps organization name (required)
- `project`: Your Azure DevOps project name (required)
- `polling_interval`: How often to refresh data in seconds (optional, default: 60)
- `theme`: Color theme for the UI (optional, default: Dracula)

**Available Themes:**
- `dark` - Default dark theme with blue and cyan accents
- `gruvbox` - Retro groove color scheme
- `nord` - Arctic, north-bluish color palette
- `dracula` - Dark theme with purple and pink accents
- `catppuccin` - Soothing pastel theme (Mocha variant)
- `github` - GitHub Dark theme
- `retro` - Matrix-inspired green phosphor on black

### Custom Themes

You can create your own custom themes by placing JSON theme files in the themes directory:
- **Linux/macOS**: `~/.config/azdo-tui/themes/`
- **Windows**: `C:\Users\<username>\.config\azdo-tui\themes\`

**Creating a Custom Theme:**

1. Create the themes directory if it doesn't exist:
   ```bash
   mkdir -p ~/.config/azdo-tui/themes
   ```

2. Create a JSON theme file (e.g., `mytheme.json`):
   ```json
   {
     "name": "mytheme",
     "primary": "#0088ff",
     "secondary": "#00aaff",
     "accent": "#ff8800",
     "success": "#00ff88",
     "warning": "#ffaa00",
     "error": "#ff4444",
     "info": "#00ccff",
     "background": "#1a1b26",
     "background_alt": "#24283b",
     "background_select": "#343b58",
     "foreground": "#c0caf5",
     "foreground_muted": "#787c99",
     "foreground_bold": "#ffffff",
     "select_foreground": "#ffffff",
     "select_background": "#0088ff",
     "border": "#3b4261",
     "link": "#7aa2f7",
     "spinner": "#bb9af7",
     "tab_active_foreground": "#ffffff",
     "tab_active_background": "#0088ff",
     "tab_inactive_foreground": "#787c99"
   }
   ```

3. Set the theme in your `config.yaml`:
   ```yaml
   theme: mytheme
   ```

4. Restart the application to use your custom theme.

See `example-theme.json` in the repository for a complete template with all available color properties. Colors can be specified as:
- Hex values: `#ff0000` or `#f00`
- ANSI 256 colors: `"1"`, `"33"`, `"196"`

### 2. Azure DevOps Personal Access Token (PAT)

On first run, the application will prompt you to enter your Azure DevOps PAT. The token is securely stored in your system's credential manager:
- **Windows**: Windows Credential Manager
- **macOS**: Keychain
- **Linux**: Secret Service (gnome-keyring, KWallet, etc.)

**Required PAT Scopes:**
| Scope | Access | Used For |
|-------|--------|----------|
| **Build** | Read | Pipeline runs, build timelines, and logs |
| **Code** | Read & Write | List PRs, view threads/iterations/diffs, vote on PRs, add comments, and update thread status |
| **Work Items** | Read & Write | Query and view work items, fetch available states, and change work item state |

To create a PAT:
1. Go to Azure DevOps → User Settings → Personal Access Tokens
2. Click "New Token"
3. Select the required scopes
4. Copy the generated token

## Usage

```bash
./azdo-tui
```

Or if installed via `go install`:

```bash
azdo-tui
```

## Keyboard Shortcuts

### Global
| Key | Action |
|-----|--------|
| `1`, `2`, `3` | Switch to Pipelines/PR/Work Items tab |
| `←/→` | Previous / next tab |
| `r` | Refresh data |
| `↑/↓` or `j/k` | Navigate up/down |
| `pgup/pgdn` | Page up/down |
| `enter` | View details / expand |
| `f` | Search / filter |
| `esc` | Go back / dismiss search |
| `?` | Toggle help modal |
| `t` | Select theme |
| `q` or `Ctrl+C` | Quit |

### PR Detail View
| Key | Action |
|-----|--------|
| `v` | Vote on pull request |

### Work Item Detail View
| Key | Action |
|-----|--------|
| `s` | Change work item state |

### Log Viewer
| Key | Action |
|-----|--------|
| `g` | Jump to top |
| `G` | Jump to bottom |

## Project Structure

```
azdo/
├── cmd/azdo-tui/           # Application entry point
├── internal/
│   ├── app/                # Root Bubble Tea application with tab management
│   ├── azdevops/           # Azure DevOps API client
│   │   ├── client.go       # HTTP client with authentication
│   │   ├── pipelines.go    # Pipeline runs API
│   │   ├── timeline.go     # Build timeline API
│   │   ├── logs.go         # Build logs API
│   │   ├── pullrequests.go # Pull requests API
│   │   ├── workitems.go    # Work items API
│   │   └── types.go        # API response types
│   ├── config/             # Configuration management
│   │   ├── config.go       # YAML config with Viper
│   │   └── keyring.go      # Secure PAT storage
│   ├── polling/            # Live update system
│   │   ├── poller.go       # Background polling
│   │   ├── events.go       # Message types
│   │   └── errorhandler.go # Graceful degradation
│   └── ui/
│       ├── components/     # Reusable UI components
│       │   ├── statusbar.go    # Footer with keybindings
│       │   ├── contextbar.go   # View-specific info bar
│       │   ├── help.go         # Help modal overlay
│       │   ├── spinner.go      # Loading indicator
│       │   └── table/          # Local table fork (TrueColor fix)
│       ├── styles/         # Theming system
│       │   ├── theme.go        # Theme type definition
│       │   ├── themes.go       # Built-in themes
│       │   └── styles.go       # Lipgloss styles
│       ├── pipelines/      # Pipeline views
│       │   ├── list.go         # Pipeline runs table
│       │   ├── detail.go       # Timeline tree view
│       │   └── logviewer.go    # Log content viewer
│       ├── pullrequests/   # Pull request views
│       │   ├── list.go         # PR list table
│       │   └── detail.go       # PR detail view
│       ├── workitems/      # Work item views
│       │   ├── list.go         # Work items table
│       │   └── detail.go       # Work item detail view
│       └── patinput/       # PAT input prompt
├── config.yaml.example     # Example configuration
└── Architecture.md         # Detailed architecture docs
```

## Technology Stack

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - Terminal UI framework
- [Bubbles](https://github.com/charmbracelet/bubbles) - TUI components (table, viewport)
- [Lipgloss](https://github.com/charmbracelet/lipgloss) - Styling and layout
- [Viper](https://github.com/spf13/viper) - Configuration management
- [go-keyring](https://github.com/zalando/go-keyring) - Secure credential storage

## Development

### Running Tests

```bash
go test ./...
```

### Running with Coverage

```bash
go test -cover ./...
```

### Building

```bash
go build -o azdo-tui ./cmd/azdo-tui
```

### Releases

This project uses [GoReleaser](https://goreleaser.com/) for automated cross-platform builds and releases.

**Supported Platforms:**
- Linux (amd64, arm64)
- macOS (amd64, arm64)
- Windows (amd64, arm64)

**Local Testing:**

```bash
# Install goreleaser
go install github.com/goreleaser/goreleaser/v2@latest

# Build snapshot (without publishing)
goreleaser build --snapshot --clean

# Full release dry-run
goreleaser release --snapshot --clean
```

**Creating a Release:**

1. Ensure all changes are committed
2. Create and push a new tag:
   ```bash
   git tag -a v0.1.0 -m "Release v0.1.0"
   git push origin v0.1.0
   ```
3. GoReleaser will automatically create a GitHub release with binaries for all platforms

Binaries will be available in the `dist/` directory after running GoReleaser locally, or as GitHub release assets when publishing.

## Roadmap

- [x] Pull requests tab
- [x] Work items tab
- [x] Multiple theme support
- [ ] Pipeline filtering and search
- [ ] Trigger pipeline runs
- [ ] Multi-project support
- [x] Theme switching within the app
- [x] PR voting (approve, reject, suggestions, wait, reset)
- [x] Work item state changes

## Contributing

Contributions are welcome! Please check the `Architecture.md` file for implementation details.

## Notes

### Local Table Fork (TrueColor Fix)

The table component at `internal/ui/components/table/` is a local fork of
[charmbracelet/bubbles/table](https://github.com/charmbracelet/bubbles/tree/main/table).
The only change is replacing `runewidth.Truncate` with `ansi.Truncate` from
`github.com/charmbracelet/x/ansi` in the `headersView()` and `renderRow()`
functions.

**Why:** The upstream bubbles table uses `go-runewidth` for truncation, which is
not ANSI-aware — it counts escape code characters (e.g. `\x1b[38;2;R;G;Bm`) as
having visual width. This causes columns to misalign and text to get truncated
when cell values contain styled ANSI content (like our colored status icons).
The `x/ansi` package's `Truncate` function is a drop-in replacement that
properly skips ANSI escape sequences.

**When upgrading bubbletea/bubbles:** Check if the upstream table has switched
from `runewidth.Truncate` to `ansi.Truncate`. If so, the local fork can be
removed and we can go back to importing `github.com/charmbracelet/bubbles/table`
directly.

## License

MIT License - see [LICENSE](LICENSE) for details.
