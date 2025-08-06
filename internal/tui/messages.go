package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/strrl/claude-resume/internal/sessions"
	"github.com/strrl/claude-resume/pkg/models"
)

// Message types for async operations
type (
	// SQLStartedMsg indicates a SQL operation has started
	SQLStartedMsg struct {
		RequestID string
		Operation string
		State     sessions.LoadingState
	}

	// SQLProgressMsg provides progress updates for long-running queries
	SQLProgressMsg struct {
		RequestID string
		Progress  float64
		Message   string
	}

	// SQLCompletedMsg indicates a SQL operation has completed
	SQLCompletedMsg struct {
		RequestID string
		Data      interface{}
		Error     error
		State     sessions.LoadingState
	}

	// SQLCancelledMsg indicates a SQL operation was cancelled
	SQLCancelledMsg struct {
		RequestID string
	}

	// ProjectsLoadedMsg contains loaded projects
	ProjectsLoadedMsg struct {
		Projects []models.Project
		Error    error
	}

	// SessionsLoadedMsg contains loaded sessions
	SessionsLoadedMsg struct {
		Sessions []models.Session
		Error    error
	}

	// SummariesLoadedMsg contains loaded session summaries
	SummariesLoadedMsg struct {
		ProjectPath string
		Summaries   map[string]string
		Error       error
	}

	// MessagesLoadedMsg contains loaded messages
	MessagesLoadedMsg struct {
		SessionID string
		Messages  []string
		Error     error
	}

	// TickMsg is sent periodically for spinner animation
	TickMsg time.Time
)

// Commands for async operations

// loadProjectsCmd loads projects asynchronously
func loadProjectsCmd(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		projects, err := sessions.FetchProjectsWithStatsAsync(ctx)
		return ProjectsLoadedMsg{
			Projects: projects,
			Error:    err,
		}
	}
}

// loadSessionsCmd loads sessions for a project asynchronously
func loadSessionsCmd(ctx context.Context, projectPath string) tea.Cmd {
	return func() tea.Msg {
		sessions, err := sessions.FetchSessionsForProjectAsync(ctx, projectPath)
		return SessionsLoadedMsg{
			Sessions: sessions,
			Error:    err,
		}
	}
}

// loadMessagesCmd loads messages for a session asynchronously
func loadMessagesCmd(ctx context.Context, sessionID string) tea.Cmd {
	return func() tea.Msg {
		messages, err := sessions.FetchRecentMessagesForSessionAsync(ctx, sessionID)
		return MessagesLoadedMsg{
			SessionID: sessionID,
			Messages:  messages,
			Error:     err,
		}
	}
}

// loadSummariesCmd loads summaries for sessions asynchronously
func loadSummariesCmd(ctx context.Context, projectPath string, sessionIDs []string) tea.Cmd {
	return func() tea.Msg {
		summaries, err := sessions.FetchSessionSummariesAsync(ctx, projectPath, sessionIDs)
		return SummariesLoadedMsg{
			ProjectPath: projectPath,
			Summaries:   summaries,
			Error:       err,
		}
	}
}

// tickCmd creates a ticker for spinner animation
func tickCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return TickMsg(t)
	})
}