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
	leftViewport    viewport.Model  // For sessions list in split view
	rightViewport   viewport.Model  // For messages preview in split view
	currentMessages []string        // Cache for current session messages
	ready           bool
	err             error
	width           int
	height          int
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
		m.width = msg.Width
		m.height = msg.Height
		
		if !m.ready {
			// Initialize viewports
			m.viewport = viewport.New(msg.Width, msg.Height-3) // For project view
			
			// For session view: split screen
			leftWidth := msg.Width / 2 - 1
			rightWidth := msg.Width - leftWidth - 1
			viewHeight := msg.Height - 3
			
			m.leftViewport = viewport.New(leftWidth, viewHeight)
			m.rightViewport = viewport.New(rightWidth, viewHeight)
			
			m.ready = true
			m.updateViewport()
		} else {
			// Resize viewports
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - 3
			
			leftWidth := msg.Width / 2 - 1
			rightWidth := msg.Width - leftWidth - 1
			viewHeight := msg.Height - 3
			
			m.leftViewport.Width = leftWidth
			m.leftViewport.Height = viewHeight
			m.rightViewport.Width = rightWidth
			m.rightViewport.Height = viewHeight
			
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
					m.loadCurrentSessionMessages()
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
					m.loadCurrentSessionMessages()
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
					// Load messages for the first session
					m.loadCurrentSessionMessages()
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
	if m.currentMode == projectView {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)
	} else {
		// Update both viewports in session view
		var leftCmd, rightCmd tea.Cmd
		m.leftViewport, leftCmd = m.leftViewport.Update(msg)
		m.rightViewport, rightCmd = m.rightViewport.Update(msg)
		cmds = append(cmds, leftCmd, rightCmd)
	}

	return m, tea.Batch(cmds...)
}

func (m *model) updateViewport() {
	if m.currentMode == projectView {
		content := m.renderProjects()
		m.viewport.SetContent(content)
	} else {
		// Split screen for session view
		leftContent := m.renderSessionsList()
		rightContent := m.renderMessages()
		m.leftViewport.SetContent(leftContent)
		m.rightViewport.SetContent(rightContent)
	}
}

func (m *model) loadCurrentSessionMessages() {
	if m.selectedProject == nil || m.sessionCursor >= len(m.selectedProject.Sessions) {
		m.currentMessages = []string{}
		return
	}
	
	session := m.selectedProject.Sessions[m.sessionCursor]
	messages, err := sessions.FetchRecentMessagesForSession(session.SessionID)
	if err != nil {
		m.currentMessages = []string{fmt.Sprintf("Error loading messages: %v", err)}
	} else if len(messages) == 0 {
		m.currentMessages = []string{"No messages found for this session"}
	} else {
		m.currentMessages = messages
	}
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
	// This is now only used in renderContent for backward compatibility
	// The actual session view uses renderSessionsList and renderMessages
	return m.renderSessionsList()
}

func (m model) renderSessionsList() string {
	if m.selectedProject == nil {
		return "No project selected"
	}

	var s strings.Builder
	
	// Header for sessions list
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("229"))
	s.WriteString(headerStyle.Render("Sessions") + "\n")
	s.WriteString(strings.Repeat("─", m.leftViewport.Width-2) + "\n\n")
	
	for i, session := range m.selectedProject.Sessions {
		cursor := "  "
		if i == m.sessionCursor {
			cursor = "> "
		}
		
		// Date and time
		dateStyle := lipgloss.NewStyle()
		if i == m.sessionCursor {
			dateStyle = dateStyle.Foreground(lipgloss.Color("212")).Bold(true)
		} else {
			dateStyle = dateStyle.Foreground(lipgloss.Color("252"))
		}
		
		line := fmt.Sprintf("%s%s",
			cursor,
			session.LastActivity.Format("01-02 15:04"))
		
		s.WriteString(dateStyle.Render(line) + "\n")
		
		// Session ID (truncated)
		sessionIDStyle := lipgloss.NewStyle()
		if i == m.sessionCursor {
			sessionIDStyle = sessionIDStyle.Foreground(lipgloss.Color("245"))
		} else {
			sessionIDStyle = sessionIDStyle.Foreground(lipgloss.Color("238"))
		}
		
		truncatedID := session.SessionID
		if len(truncatedID) > 12 {
			truncatedID = truncatedID[:12] + "..."
		}
		sessionIDLine := fmt.Sprintf("  %s", truncatedID)
		s.WriteString(sessionIDStyle.Render(sessionIDLine) + "\n")
		
		if i < len(m.selectedProject.Sessions)-1 {
			s.WriteString("\n")
		}
	}
	
	return s.String()
}

func (m model) renderMessages() string {
	var s strings.Builder
	
	// Header
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("229"))
	
	s.WriteString(headerStyle.Render("Recent Messages") + "\n")
	dividerWidth := m.rightViewport.Width - 2
	if dividerWidth < 10 {
		dividerWidth = 10
	}
	s.WriteString(strings.Repeat("─", dividerWidth) + "\n\n")
	
	if len(m.currentMessages) == 0 {
		emptyStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Italic(true)
		s.WriteString(emptyStyle.Render("No messages found"))
		return s.String()
	}
	
	// Display messages
	messageStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252"))
	
	for i, msg := range m.currentMessages {
		// Message number
		numStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("243")).
			Bold(true)
		s.WriteString(numStyle.Render(fmt.Sprintf("%d. ", i+1)))
		
		// Message content (wrap long lines)
		wrapWidth := m.rightViewport.Width - 5
		if wrapWidth < 20 {
			wrapWidth = 20
		}
		lines := wrapText(msg, wrapWidth)
		for j, line := range lines {
			if j > 0 {
				s.WriteString("   ") // Indent continuation lines
			}
			s.WriteString(messageStyle.Render(line) + "\n")
		}
		
		if i < len(m.currentMessages)-1 {
			s.WriteString("\n")
		}
	}
	
	return s.String()
}

// wrapText wraps text to fit within the specified width
func wrapText(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	
	var lines []string
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{text}
	}
	
	currentLine := words[0]
	for _, word := range words[1:] {
		if len(currentLine)+1+len(word) > width {
			lines = append(lines, currentLine)
			currentLine = word
		} else {
			currentLine += " " + word
		}
	}
	if currentLine != "" {
		lines = append(lines, currentLine)
	}
	
	return lines
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
	
	if m.currentMode == projectView {
		return fmt.Sprintf("%s\n%s\n%s", header, m.viewport.View(), footer)
	} else {
		// Split screen view for sessions
		return fmt.Sprintf("%s\n%s\n%s", header, m.renderSplitView(), footer)
	}
}

func (m model) renderSplitView() string {
	// Use lipgloss to properly handle the layout
	leftStyle := lipgloss.NewStyle().
		Width(m.leftViewport.Width).
		Height(m.leftViewport.Height)
	
	rightStyle := lipgloss.NewStyle().
		Width(m.rightViewport.Width).
		Height(m.rightViewport.Height)
	
	dividerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("238")).
		Height(m.leftViewport.Height)
	
	leftContent := leftStyle.Render(m.leftViewport.View())
	rightContent := rightStyle.Render(m.rightViewport.View())
	
	// Create the divider
	divider := strings.Builder{}
	for i := 0; i < m.leftViewport.Height; i++ {
		divider.WriteString("│")
		if i < m.leftViewport.Height-1 {
			divider.WriteString("\n")
		}
	}
	
	// Join the views horizontally
	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		leftContent,
		dividerStyle.Render(divider.String()),
		rightContent,
	)
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