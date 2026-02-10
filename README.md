# azdo-tui

A Terminal User Interface (TUI) for Azure DevOps - monitor pipelines, manage pull requests, and track work items without leaving your terminal.

## Features

### ✅ Milestone 1: Pipelines Dashboard (Completed)
- View recent pipeline runs in a table
- Color-coded status indicators (✓ Success, ✗ Failed, ⟳ Running)
- Manual refresh with 'r' key
- Navigate with arrow keys

### 🚧 Coming Soon
- Pipeline detail view with stages and jobs
- Log viewer for pipeline runs
- Pull requests tab
- Work items board
- Live auto-refresh

## Installation

```bash
go install github.com/Elpulgo/azdo/cmd/azdo-tui@latest
```

Or build from source:

```bash
git clone https://github.com/Elpulgo/azdo.git
cd azdo
go build -o azdo-tui ./cmd/azdo-tui
```

## Configuration

1. Create a configuration file at `~/.config/azdo-tui/config.yaml`:

```yaml
organization: your-org-name
project: your-project-name
polling_interval: 60
theme: dark
```

Or copy the example:

```bash
mkdir -p ~/.config/azdo-tui
cp config.yaml.example ~/.config/azdo-tui/config.yaml
# Edit the file with your organization and project
```

2. Store your Azure DevOps Personal Access Token (PAT) in the system keyring:

```go
// TODO: Implement auth login command
// For now, you can set the PAT programmatically using the keyring
```

**Note:** PAT storage via keyring is implemented. An auth command will be added in a future milestone.

## Usage

```bash
azdo-tui
```

### Keyboard Shortcuts

- `r` - Refresh pipeline runs
- `↑/↓` - Navigate through the list
- `q` or `Ctrl+C` - Quit

## Development

### Running Tests

```bash
go test ./...
```

### Project Structure

```
azdo-tui/
├── cmd/azdo-tui/          # Main application entry point
├── internal/
│   ├── app/               # Root bubbletea application model
│   ├── azdevops/          # Azure DevOps API client
│   │   ├── client.go      # HTTP client with auth
│   │   ├── pipelines.go   # Pipeline API endpoints
│   │   └── types.go       # API response types
│   ├── config/            # Configuration management
│   │   ├── config.go      # Config loading with Viper
│   │   └── keyring.go     # Secure PAT storage
│   └── ui/
│       └── pipelines/     # Pipeline views
│           └── list.go    # Pipeline list table
└── Architecture.md        # Detailed architecture docs
```

## Technology Stack

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - Terminal UI framework
- [Bubbles](https://github.com/charmbracelet/bubbles) - TUI components (table, viewport, etc.)
- [Lipgloss](https://github.com/charmbracelet/lipgloss) - Styling and layout
- [Viper](https://github.com/spf13/viper) - Configuration management
- [go-keyring](https://github.com/zalando/go-keyring) - Secure credential storage

## Contributing

Contributions are welcome! Please check the Architecture.md file for implementation details and the roadmap.

## License

MIT License
