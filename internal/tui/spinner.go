package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Spinner represents a loading spinner
type Spinner struct {
	frames []string
	frame  int
}

// NewSpinner creates a new spinner
func NewSpinner() *Spinner {
	return &Spinner{
		frames: []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"},
		frame:  0,
	}
}

// Next advances the spinner to the next frame
func (s *Spinner) Next() {
	s.frame = (s.frame + 1) % len(s.frames)
}

// View returns the current spinner frame
func (s *Spinner) View() string {
	return s.frames[s.frame]
}

// LoadingIndicator creates a loading indicator with a message
type LoadingIndicator struct {
	spinner  *Spinner
	message  string
	progress float64
	showProgress bool
}

// NewLoadingIndicator creates a new loading indicator
func NewLoadingIndicator(message string) *LoadingIndicator {
	return &LoadingIndicator{
		spinner: NewSpinner(),
		message: message,
	}
}

// SetProgress sets the progress percentage (0-100)
func (l *LoadingIndicator) SetProgress(progress float64) {
	l.progress = progress
	l.showProgress = true
}

// SetMessage updates the loading message
func (l *LoadingIndicator) SetMessage(message string) {
	l.message = message
}

// Tick advances the spinner animation
func (l *LoadingIndicator) Tick() {
	l.spinner.Next()
}

// View renders the loading indicator
func (l *LoadingIndicator) View() string {
	spinnerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("212"))

	messageStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("250"))

	var content string
	if l.showProgress {
		// Show progress bar
		progressBar := renderProgressBar(l.progress, 20)
		content = fmt.Sprintf("%s %s %s (%.0f%%)",
			spinnerStyle.Render(l.spinner.View()),
			messageStyle.Render(l.message),
			progressBar,
			l.progress)
	} else {
		// Just show spinner and message
		content = fmt.Sprintf("%s %s",
			spinnerStyle.Render(l.spinner.View()),
			messageStyle.Render(l.message))
	}

	return content
}

// renderProgressBar creates a simple progress bar
func renderProgressBar(progress float64, width int) string {
	if progress < 0 {
		progress = 0
	}
	if progress > 100 {
		progress = 100
	}

	filled := int(float64(width) * progress / 100)
	empty := width - filled

	barStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("42"))

	emptyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("238"))
	
	return barStyle.Render(strings.Repeat("█", filled)) + 
		emptyStyle.Render(strings.Repeat("░", empty))
}

// LoadingOverlay creates a centered loading overlay
func LoadingOverlay(width, height int, indicator *LoadingIndicator) string {
	content := indicator.View()
	
	// Add cancel hint
	cancelHint := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Render("[ESC to cancel]")
	
	// Combine content and hint
	fullContent := fmt.Sprintf("%s\n\n%s", content, cancelHint)
	
	// Center the content
	style := lipgloss.NewStyle().
		Width(width).
		Height(height).
		Align(lipgloss.Center, lipgloss.Center)
	
	return style.Render(fullContent)
}