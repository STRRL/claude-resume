package models

import "time"

// Session represents a Claude Code session
type Session struct {
	SessionID    string
	ProjectPath  string
	LastActivity time.Time
}

// Project represents a project with aggregated session information
type Project struct {
	Name         string
	Path         string
	SessionCount int
	LastActivity time.Time
	Sessions     []Session // Lazily loaded when needed
}