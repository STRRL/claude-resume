package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type viewMode int

const (
	projectView viewMode = iota
	sessionView
)

type model struct {
	projects         []Project
	currentMode      viewMode
	projectCursor    int
	sessionCursor    int
	selectedProject  *Project
	selectedSession  *Session
	viewport         viewport.Model
	ready            bool
	err              error
}

func initialModel(projects []Project) model {
	return model{
		projects:    projects,
		currentMode: projectView,
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		headerHeight := 4
		footerHeight := 3
		verticalMarginHeight := headerHeight + footerHeight

		if !m.ready {
			m.viewport = viewport.New(msg.Width, msg.Height-verticalMarginHeight)
			m.viewport.YPosition = headerHeight
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - verticalMarginHeight
		}

		// Update viewport content
		switch m.currentMode {
		case projectView:
			m.viewport.SetContent(m.renderProjectList())
		case sessionView:
			m.viewport.SetContent(m.renderSessionList())
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit

		case "esc":
			if m.currentMode == sessionView {
				m.currentMode = projectView
				m.sessionCursor = 0
				m.viewport.SetContent(m.renderProjectList())
				m.viewport.GotoTop()
			}

		case "up", "k":
			switch m.currentMode {
			case projectView:
				if m.projectCursor > 0 {
					m.projectCursor--
					m.viewport.SetContent(m.renderProjectList())
					// Ensure cursor is visible
					m.ensureCursorVisible()
				}
			case sessionView:
				if m.sessionCursor > 0 {
					m.sessionCursor--
					m.viewport.SetContent(m.renderSessionList())
					m.ensureCursorVisible()
				}
			}

		case "down", "j":
			switch m.currentMode {
			case projectView:
				if m.projectCursor < len(m.projects)-1 {
					m.projectCursor++
					m.viewport.SetContent(m.renderProjectList())
					m.ensureCursorVisible()
				}
			case sessionView:
				if m.selectedProject != nil && m.sessionCursor < len(m.selectedProject.Sessions)-1 {
					m.sessionCursor++
					m.viewport.SetContent(m.renderSessionList())
					m.ensureCursorVisible()
				}
			}

		case "enter":
			switch m.currentMode {
			case projectView:
				if m.projectCursor < len(m.projects) {
					m.selectedProject = &m.projects[m.projectCursor]
					// Load sessions for the selected project
					sessions, err := fetchSessionsForProject(m.selectedProject.Path)
					if err != nil {
						m.err = err
						return m, nil
					}
					// Load recent messages for each session
					for i := range sessions {
						messages, err := fetchRecentMessagesForSession(sessions[i].SessionID)
						if err == nil {
							sessions[i].RecentMessages = messages
						}
					}
					m.selectedProject.Sessions = sessions
					m.currentMode = sessionView
					m.sessionCursor = 0
					m.viewport.SetContent(m.renderSessionList())
					m.viewport.GotoTop()
				}
			case sessionView:
				if m.selectedProject != nil && m.sessionCursor < len(m.selectedProject.Sessions) {
					m.selectedSession = &m.selectedProject.Sessions[m.sessionCursor]
					return m, tea.Quit
				}
			}

		default:
			// Handle viewport keys (page up/down, etc)
			m.viewport, cmd = m.viewport.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	// Handle viewport updates
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m *model) ensureCursorVisible() {
	// Rough calculation of lines per item
	linesPerItem := 2 // Adjust based on your item rendering
	
	currentLine := m.projectCursor * linesPerItem
	if m.currentMode == sessionView {
		currentLine = m.sessionCursor * linesPerItem
	}

	// Scroll to make cursor visible
	if currentLine < m.viewport.YOffset {
		m.viewport.SetYOffset(currentLine)
	} else if currentLine > m.viewport.YOffset+m.viewport.Height-linesPerItem {
		m.viewport.SetYOffset(currentLine - m.viewport.Height + linesPerItem)
	}
}


func (m model) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n", m.err)
	}

	if !m.ready {
		return "\n  Initializing..."
	}

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205"))

	projectStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("214"))

	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241"))

	var header string
	switch m.currentMode {
	case projectView:
		header = titleStyle.Render("Claude Resume - Select a Project")
	case sessionView:
		header = titleStyle.Render("Claude Resume - Select a Session") + "\n" +
			projectStyle.Render(fmt.Sprintf("Project: %s", m.selectedProject.Name))
	}

	var footer string
	switch m.currentMode {
	case projectView:
		footer = helpStyle.Render("↑/k: up • ↓/j: down • enter: select • q: quit")
	case sessionView:
		footer = helpStyle.Render("↑/k: up • ↓/j: down • enter: select • esc: back • q: quit")
	}

	return fmt.Sprintf("%s\n\n%s\n%s", header, m.viewport.View(), footer)
}

func (m model) renderProjectList() string {
	selectedStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205")).
		Background(lipgloss.Color("237"))

	normalStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252"))

	countStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245"))

	var b strings.Builder

	for i, project := range m.projects {
		cursor := "  "
		if m.projectCursor == i {
			cursor = "> "
		}

		sessionCount := project.SessionCount
		lastActivity := project.LastActivity.Format("2006-01-02 15:04")

		if m.projectCursor == i {
			b.WriteString(selectedStyle.Render(cursor))
			b.WriteString(selectedStyle.Render(fmt.Sprintf("%-40s ", truncate(project.Name, 40))))
			b.WriteString(countStyle.Render(fmt.Sprintf("(%d sessions) ", sessionCount)))
			b.WriteString(selectedStyle.Render(lastActivity))
		} else {
			b.WriteString(cursor)
			b.WriteString(normalStyle.Render(fmt.Sprintf("%-40s ", truncate(project.Name, 40))))
			b.WriteString(countStyle.Render(fmt.Sprintf("(%d sessions) ", sessionCount)))
			b.WriteString(normalStyle.Render(lastActivity))
		}
		b.WriteString("\n")
	}

	return strings.TrimSuffix(b.String(), "\n")
}

func (m model) renderSessionList() string {
	if m.selectedProject == nil {
		return "No project selected"
	}

	selectedStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205")).
		Background(lipgloss.Color("237"))

	normalStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252"))

	messageStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		MarginLeft(4)

	var b strings.Builder

	for i, session := range m.selectedProject.Sessions {
		cursor := "  "
		if m.sessionCursor == i {
			cursor = "> "
		}

		if m.sessionCursor == i {
			b.WriteString(selectedStyle.Render(cursor))
			b.WriteString(selectedStyle.Render(session.LastActivity.Format("2006-01-02 15:04")))
		} else {
			b.WriteString(cursor)
			b.WriteString(normalStyle.Render(session.LastActivity.Format("2006-01-02 15:04")))
		}
		b.WriteString("\n")

		// Show recent messages
		if len(session.RecentMessages) > 0 {
			for j, msg := range session.RecentMessages {
				if j >= 5 { // Only show first 5 messages
					break
				}
				truncatedMsg := truncate(msg, 50) // Only first 50 characters
				b.WriteString(messageStyle.Render(fmt.Sprintf("  %d. %s", j+1, truncatedMsg)))
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")
	}

	return strings.TrimSuffix(b.String(), "\n")
}


func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func showTUI(projects []Project) (*Session, error) {
	p := tea.NewProgram(initialModel(projects), tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}

	m := finalModel.(model)
	return m.selectedSession, nil
}