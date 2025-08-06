package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/strrl/claude-resume/internal/sessions"
	"github.com/strrl/claude-resume/pkg/models"
)

type viewMode int

const (
	projectView viewMode = iota
	sessionView
)

type model struct {
	projects        []models.Project
	currentMode     viewMode
	projectCursor   int
	sessionCursor   int
	selectedProject *models.Project
	selectedSession *models.Session
	viewport        viewport.Model
	ready           bool
	err             error
}

func initialModel(projects []models.Project) model {
	return model{
		projects:      projects,
		currentMode:   projectView,
		projectCursor: 0,
		sessionCursor: 0,
	}
}

func (m model) Init() tea.Cmd {
	// Return a command to get the window size
	return tea.EnterAltScreen
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if !m.ready {
			m.viewport = viewport.New(msg.Width, msg.Height-3) // Leave room for header and footer
			m.viewport.YPosition = 0
			m.ready = true
			// Initialize content after viewport is ready
			m.updateViewport()
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - 3
			m.updateViewport()
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit

		case "up", "k":
			if m.currentMode == projectView {
				if m.projectCursor > 0 {
					m.projectCursor--
					m.updateViewport()
				}
			} else {
				if m.sessionCursor > 0 {
					m.sessionCursor--
					m.updateViewport()
				}
			}

		case "down", "j":
			if m.currentMode == projectView {
				if m.projectCursor < len(m.projects)-1 {
					m.projectCursor++
					m.updateViewport()
				}
			} else {
				if m.selectedProject != nil && m.sessionCursor < len(m.selectedProject.Sessions)-1 {
					m.sessionCursor++
					m.updateViewport()
				}
			}

		case "enter":
			if m.currentMode == projectView {
				// Load sessions for the selected project
				if m.projectCursor < len(m.projects) {
					project := m.projects[m.projectCursor]
					projectSessions, err := sessions.FetchSessionsForProject(project.Path)
					if err != nil {
						m.err = err
						return m, nil
					}
					project.Sessions = projectSessions
					m.selectedProject = &project
					m.currentMode = sessionView
					m.sessionCursor = 0
					m.updateViewport()
				}
			} else {
				// Select session to resume
				if m.selectedProject != nil && m.sessionCursor < len(m.selectedProject.Sessions) {
					m.selectedSession = &m.selectedProject.Sessions[m.sessionCursor]
					return m, tea.Quit
				}
			}

		case "esc", "backspace":
			if m.currentMode == sessionView {
				m.currentMode = projectView
				m.selectedProject = nil
				m.sessionCursor = 0
				m.updateViewport()
			}
		}
	}

	// Handle viewport updates
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m *model) updateViewport() {
	content := m.renderContent()
	m.viewport.SetContent(content)
}

func (m model) renderContent() string {
	if m.currentMode == projectView {
		return m.renderProjects()
	}
	return m.renderSessions()
}

func (m model) renderProjects() string {
	var s strings.Builder
	
	for i, project := range m.projects {
		cursor := "  "
		if i == m.projectCursor {
			cursor = "> "
		}
		
		style := lipgloss.NewStyle()
		if i == m.projectCursor {
			style = style.Foreground(lipgloss.Color("212")).Bold(true)
		}
		
		line := fmt.Sprintf("%s%s (%d sessions) - %s",
			cursor,
			project.Name,
			project.SessionCount,
			project.LastActivity.Format("2006-01-02 15:04"))
		
		s.WriteString(style.Render(line) + "\n")
	}
	
	return s.String()
}

func (m model) renderSessions() string {
	if m.selectedProject == nil {
		return "No project selected"
	}

	var s strings.Builder
	s.WriteString(fmt.Sprintf("Sessions for %s:\n", m.selectedProject.Name))
	s.WriteString(strings.Repeat("─", 50) + "\n\n")
	
	for i, session := range m.selectedProject.Sessions {
		cursor := "  "
		if i == m.sessionCursor {
			cursor = "> "
		}
		
		style := lipgloss.NewStyle()
		if i == m.sessionCursor {
			style = style.Foreground(lipgloss.Color("212")).Bold(true)
		}
		
		line := fmt.Sprintf("%s%s - %s",
			cursor,
			session.SessionID[:8],
			session.LastActivity.Format("2006-01-02 15:04"))
		
		s.WriteString(style.Render(line) + "\n")
		
		// Show recent messages for the current session
		if i == m.sessionCursor {
			messages, err := sessions.FetchRecentMessagesForSession(session.SessionID)
			if err == nil && len(messages) > 0 {
				s.WriteString("  Recent messages:\n")
				for j, msg := range messages {
					if j >= 5 {
						break
					}
					truncatedMsg := truncate(msg, 50)
					s.WriteString(fmt.Sprintf("    • %s\n", truncatedMsg))
				}
			}
		}
		
		s.WriteString("\n")
	}
	
	return s.String()
}

func (m model) View() string {
	if !m.ready {
		return "\n  Initializing..."
	}

	if m.err != nil {
		return fmt.Sprintf("\n  Error: %v\n", m.err)
	}

	header := m.renderHeader()
	footer := m.renderFooter()
	
	return fmt.Sprintf("%s\n%s\n%s", header, m.viewport.View(), footer)
}

func (m model) renderHeader() string {
	title := "Claude Resume - Projects"
	if m.currentMode == sessionView && m.selectedProject != nil {
		title = fmt.Sprintf("Claude Resume - %s", m.selectedProject.Name)
	}
	
	style := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("63"))
	
	return style.Render(title)
}

func (m model) renderFooter() string {
	info := "↑/↓: navigate • enter: select"
	if m.currentMode == sessionView {
		info += " • esc: back"
	}
	info += " • q: quit"
	
	style := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241"))
	
	return style.Render(info)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// ShowTUI displays the TUI and returns the selected session
func ShowTUI(projects []models.Project) (*models.Session, error) {
	p := tea.NewProgram(
		initialModel(projects),
		tea.WithAltScreen(),
	)

	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}

	m := finalModel.(model)
	return m.selectedSession, nil
}