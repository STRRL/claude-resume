# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

claude-resume is a Go-based TUI (Terminal User Interface) application that allows users to browse and resume recent Claude Code sessions. It provides a two-level navigation interface where users first select a project, then select a specific session within that project.

## Architecture

The application follows a clean separation of concerns with three main components:

1. **main.go** - Entry point that orchestrates the flow: fetch projects → show TUI → execute claude resume
2. **sessions.go** - Handles all DuckDB queries and data operations for fetching projects and sessions from Claude's JSONL files
3. **tui.go** - Implements the Bubble Tea-based terminal UI with split-screen session view

### Data Flow

1. DuckDB queries read from `~/.claude/projects/**/*.jsonl` files
2. Projects are fetched first with aggregated statistics (session count, last activity)
3. Sessions are lazily loaded only when a project is selected
4. When a session is selected, the app changes to the project directory and executes `claude --resume <session-id>`

### Key SQL Patterns

The app uses two main query patterns:

- **Project aggregation**: Groups all events by `cwd` (project path) to get session counts and last activity
- **Session details**: Filters events by project path and aggregates by sessionId, including fetching the last user message

## Development Commands

```bash
# Build the application
make build

# Build and run immediately
make run

# Install to Go's bin directory
make install

# Run tests
make test

# Clean build artifacts
make clean

# Update dependencies
go mod tidy
```

## Testing and Debugging

When working on the TUI:
- The app uses viewport components for scrolling - be aware that color codes need special handling
- Split-screen mode is triggered when entering session view
- Test with projects that have many sessions to verify scrolling behavior

For SQL query debugging:
- DuckDB requires explicit installation of the json extension (`INSTALL json; LOAD json;`)
- Session IDs must be cast to VARCHAR to avoid binary data issues
- Use `COALESCE(cwd, 'Unknown')` to handle null project paths

## Important Implementation Details

### Terminal Handling
- Uses `tea.WithAltScreen()` for clean terminal restoration
- Viewports are dynamically resized when switching between project and session views
- Window resize messages are sent as commands to trigger viewport recalculation

### Session ID Handling
- Session IDs from JSONL files may contain binary data
- Always use `CAST(sessionId AS VARCHAR)` in SQL queries
- The app changes to the project directory before executing `claude --resume`

### Performance Considerations
- Sessions are loaded on-demand when a project is selected (not all at once)
- Recent messages for a session are fetched separately when navigating in session view
- Queries limit results to prevent overwhelming the UI (100 projects, 100 sessions per project)