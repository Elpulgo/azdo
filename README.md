# azdo-tui

A Terminal User Interface (TUI) for Azure DevOps - monitor pipelines directly from your terminal.

![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)
![License](https://img.shields.io/badge/License-MIT-blue.svg)

## Features

### Pipeline Dashboard
- View recent pipeline runs in a sortable table
- Color-coded status indicators (✓ Success, ✗ Failed, ● Running, ○ Queued)
- Live auto-refresh with configurable polling interval
- Connection status indicator in footer

### Pipeline Detail View
- Hierarchical view of stages, jobs, and tasks
- Duration tracking for each step
- Log indicator showing which items have viewable logs
- Status messages for selected items

### Log Viewer
- Full log content for any task
- Scrollable viewport with keyboard navigation
- Timestamps automatically stripped for cleaner display
- Jump to top/bottom with g/G keys

### Additional Features
- Help modal with all keyboard shortcuts (press `?`)
- Secure PAT storage using system keyring
- Context-aware keybinding hints
- Graceful error handling with automatic retry

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

Create a configuration file at `~/.config/azdo-tui/config.yaml`:

```yaml
# Azure DevOps organization name (required)
organization: your-org-name

# Azure DevOps project name (required)
project: your-project-name

# Polling interval in seconds (optional, default: 60)
polling_interval: 30

# Theme (optional, default: dark)
theme: dark
```

Or copy the example configuration:

```bash
mkdir -p ~/.config/azdo-tui
cp config.yaml.example ~/.config/azdo-tui/config.yaml
```

### 2. Azure DevOps Personal Access Token (PAT)

On first run, the application will prompt you to enter your Azure DevOps PAT. The token is securely stored in your system keyring (Windows Credential Manager, macOS Keychain, or Linux Secret Service).

**Required PAT Scopes:**
- `Build` - Read (for pipeline runs and logs)

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
| `r` | Refresh data |
| `↑/↓` or `j/k` | Navigate up/down |
| `pgup/pgdn` | Page up/down |
| `enter` | View details / expand |
| `esc` | Go back |
| `?` | Toggle help modal |
| `q` or `Ctrl+C` | Quit |

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
│   ├── app/                # Root Bubble Tea application
│   ├── azdevops/           # Azure DevOps API client
│   │   ├── client.go       # HTTP client with authentication
│   │   ├── pipelines.go    # Pipeline runs API
│   │   ├── timeline.go     # Build timeline API
│   │   ├── logs.go         # Build logs API
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
│       │   └── spinner.go      # Loading indicator
│       ├── pipelines/      # Pipeline views
│       │   ├── list.go         # Pipeline runs table
│       │   ├── detail.go       # Timeline tree view
│       │   └── logviewer.go    # Log content viewer
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

## Roadmap

- [ ] Pull requests tab
- [ ] Work items board
- [ ] Pipeline filtering and search
- [ ] Trigger pipeline runs
- [ ] Multi-project support

## Contributing

Contributions are welcome! Please check the `Architecture.md` file for implementation details.

## License

MIT License - see [LICENSE](LICENSE) for details.
