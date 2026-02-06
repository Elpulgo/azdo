# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**azdo** is a TUI (Terminal User Interface) for working with Azure DevOps in the terminal.

- **Language**: Go
- **Project Type**: Terminal User Interface application
- **Purpose**: Interact with Azure DevOps from the command line

## Development Setup

This project uses Go modules for dependency management.

### Initial Setup
```bash
# Initialize Go module (if not already done)
go mod init github.com/Elpulgo/azdo

# Download dependencies
go mod download

# Tidy dependencies
go mod tidy
```

### Common Commands

```bash
# Build the application
go build -o azdo

# Run the application
go run .

# Run tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run tests for a specific package
go test ./path/to/package

# Format code
go fmt ./...

# Vet code for issues
go vet ./...
```

## Architecture

This is a new project. Architecture details will be added as the codebase develops.

Expected structure:
- TUI application for Azure DevOps interaction
- Standard Go project layout with `cmd/`, `pkg/`, and/or `internal/` directories
- Integration with Azure DevOps REST API

!!! CRITICAL !!! You should use beads (bd) as tracking system when creating tasks. ALWAYS run bd ready when a new session starts.


## Skill/role

When developing this project you are a pro in Golang, read the Skill.md file and use these approaches when devloping.

## TDD

!!! CRITICIAL !!! You should at all times use a TDD approach when developing the code.

Build tests first, fail, then make them green.

