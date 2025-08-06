# claude-resume

An interactive TUI (Terminal User Interface) tool for browsing and resuming Claude Code sessions with a sophisticated split-screen interface and real-time conversation preview.

## Features

- **Project Browser**: View all projects with Claude Code sessions, session counts, and last activity timestamps
- **Split-Screen Session View**: Browse sessions on the left while previewing conversation messages on the right
- **Smart Message Display**: Shows first 10 and last 10 messages with automatic truncation and omission indicators
- **Rich Message Formatting**: 
  - Role-based color coding (User/Assistant)
  - Tool call visualization with icons (🔧 for calls, ↩ for results)
  - Intelligent 50-character truncation for readability
- **Instant Session Resume**: Resume any session with a single keystroke
- **Clean Terminal UI**: Proper viewport scrolling and responsive layout

## Installation

### Quick Start with Homebrew

```bash
# Add this tap to your Homebrew
brew tap strrl/collective

# Install packages
brew install claude-resume
```

### From source

```bash
# Clone the repository
git clone https://github.com/strrl/claude-resume
cd claude-resume

# Build and install
make install
```

### Manual build

```bash
go build -o claude-resume
./claude-resume
```

## Usage

```bash
# Run the TUI to browse and select a session
claude-resume

# Debug a specific session (shows the messages in that session)
claude-resume debug-session <session-id>
```

### Keyboard Navigation

#### Project View
- `↑` / `k`: Move up
- `↓` / `j`: Move down  
- `Enter`: Select project and view sessions
- `q` / `Ctrl+C`: Quit

#### Session View (Split-Screen)
- `↑` / `k`: Navigate through sessions (left panel)
- `↓` / `j`: Navigate through sessions (left panel)
- Message preview updates automatically (right panel)
- `Enter`: Resume the selected session
- `Esc` / `Backspace`: Return to project view
- `q` / `Ctrl+C`: Quit

## Requirements

- Go 1.21 or higher
- Claude Code CLI installed and in PATH
- Access to Claude Code session files in `~/.claude/projects/`

## How It Works

1. **Data Source**: Reads session data from `~/.claude/projects/**/*.jsonl` files
2. **DuckDB Processing**: Uses DuckDB's JSON capabilities with SQL window functions for efficient data queries
3. **Three-Level Interface**:
   - **Project View**: Browse all projects with aggregated statistics
   - **Session View**: Split-screen with session list (left) and message preview (right)
   - **Message Preview**: Intelligently displays conversation context with first/last messages
4. **Session Resume**: Changes to project directory and executes `claude --resume <session-id>`

### Message Preview Intelligence

The message preview system uses sophisticated SQL window functions to:
- Fetch the first 10 and last 10 messages of each conversation
- Automatically detect and display omitted message counts
- Parse nested JSON structures for tool calls and responses
- Apply role-based formatting and truncation for optimal readability

## Technical Stack

- **Language**: Go 1.21+
- **TUI Framework**: [Bubble Tea](https://github.com/charmbracelet/bubbletea) with [Lipgloss](https://github.com/charmbracelet/lipgloss) styling
- **Database**: [DuckDB](https://github.com/marcboeker/go-duckdb) with JSON extension for JSONL processing
- **Architecture**: Clean separation with cmd/, internal/, and pkg/ structure

## License

MIT License - see LICENSE file