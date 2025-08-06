# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

claude-resume is a Go-based TUI (Terminal User Interface) application that allows users to browse and resume recent Claude Code sessions. It provides a sophisticated split-screen interface where users can browse projects, view sessions, and preview conversation messages in real-time before resuming.

## Architecture

The application follows the Go standard project layout with a clean separation of concerns:

### Directory Structure
- **cmd/claude-resume/** - Application entry point and CLI commands
- **internal/sessions/** - DuckDB queries and data operations for fetching projects and sessions
- **internal/tui/** - Bubble Tea-based terminal UI implementation
- **internal/db/** - Database initialization and connection management
- **pkg/models/** - Shared data models (Project, Session)

### Data Flow

1. DuckDB queries read from `~/.claude/projects/**/*.jsonl` files
2. Projects are fetched first with aggregated statistics (session count, last activity)
3. Sessions are lazily loaded only when a project is selected
4. Messages are lazily loaded when navigating sessions (first 10 + last 10 messages)
5. When a session is selected, the app changes to the project directory and executes `claude --resume <session-id>`

### Key Components

- **cmd/claude-resume/main.go**: CLI commands using Cobra (show, debug-session)
- **internal/sessions/sessions.go**: Core data fetching logic with SQL queries
- **internal/tui/tui.go**: Interactive terminal UI with viewport support
- **internal/db/duckdb.go**: DuckDB initialization with JSON extension
- **pkg/models/models.go**: Shared Project and Session structs

### Key SQL Patterns

The app uses three main query patterns:

- **Project aggregation**: Groups all events by `cwd` (project path) to get session counts and last activity
- **Session details**: Filters events by project path and aggregates by sessionId
- **Message preview**: Uses SQL window functions (ROW_NUMBER() OVER) to efficiently select first 10 and last 10 messages, with support for all message types (user, assistant, tool calls)

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
- Split-screen mode is triggered when entering session view with left/right viewports
- Test with projects that have many sessions to verify scrolling behavior
- Initial render issue fixed by calling `updateViewport()` on first WindowSizeMsg

For SQL query debugging:
- DuckDB requires explicit installation of the json extension (`INSTALL json; LOAD json;`)
- Session IDs must be cast to VARCHAR to avoid binary data issues
- Use `COALESCE(cwd, 'Unknown')` to handle null project paths
- Window functions (ROW_NUMBER() OVER) used for efficient message selection

For message parsing:
- Messages can be string or array format in JSON
- Tool calls have nested structure with `type`, `name`, and `input` fields
- System reminders and interruption messages are filtered out

## Important Implementation Details

### Terminal Handling
- Uses `tea.WithAltScreen()` for clean terminal restoration
- Split-screen mode with separate left/right viewports in session view
- Viewports are dynamically resized on window resize events
- Uses `lipgloss.JoinHorizontal()` for proper split-screen layout
- Initial render fix: calls `updateViewport()` when window size is first received

### Session ID Handling
- Session IDs from JSONL files may contain binary data
- Always use `CAST(sessionId AS VARCHAR)` in SQL queries
- The app changes to the project directory before executing `claude --resume`

### Message Display Features
- **Smart Truncation**: All messages truncated to 50 characters for readability
- **Role-Based Formatting**: Different colors for User (blue), Assistant (pink), and tool messages
- **Tool Call Visualization**: 
  - Tool calls shown with 🔧 icon and truncated parameters
  - Tool results shown with ↩ icon
- **Omission Indicator**: Shows "... (N messages omitted) ..." when middle messages are skipped
- **Efficient Loading**: Messages loaded lazily when navigating sessions

### Performance Considerations
- Sessions are loaded on-demand when a project is selected (not all at once)
- Messages are loaded lazily when navigating sessions in split-screen view
- SQL window functions used for efficient first/last message selection
- Queries limit results to prevent overwhelming the UI (100 projects, 100 sessions per project)
- Singleton DuckDB connection pattern prevents connection overhead

## IMPORTANT: Git Commit Policy

⚠️ **NEVER EXECUTE GIT COMMIT COMMANDS UNLESS EXPLICITLY REQUESTED** ⚠️

Claude Code MUST NOT run any git commit commands automatically or proactively. Git commits should only be created when the user explicitly asks for them using phrases like:
- "commit these changes"
- "create a commit"
- "git commit"
- "please commit"

This ensures that:
- Users maintain full control over their git history
- No unwanted commits are created
- Users can review all changes before committing
- Commit messages can be properly crafted when requested

If changes need to be saved but no commit was requested, simply inform the user that the changes have been made to the files without creating a commit.