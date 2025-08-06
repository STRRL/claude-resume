package commands

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/strrl/claude-resume/internal/sessions"
	"github.com/strrl/claude-resume/pkg/models"
)

// NewShowCommand creates the show command
func NewShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show [project] [session-id]",
		Short: "Show projects, sessions, or messages without TUI",
		Long: `Show projects, sessions, or messages in a non-interactive format.
Without arguments: lists all projects
With project name: lists all sessions in that project
With project name and session ID: shows recent messages for that session`,
		RunE: runShow,
	}
}

func runShow(cmd *cobra.Command, args []string) error {
	switch len(args) {
	case 0:
		// Show all projects
		return showProjects()
	case 1:
		// Show sessions for a specific project
		return showSessions(args[0])
	case 2:
		// Show messages for a specific session
		return showMessages(args[0], args[1])
	default:
		return fmt.Errorf("too many arguments. Usage: claude-resume show [project] [session-id]")
	}
}

func showProjects() error {
	projects, err := sessions.FetchProjectsWithStats()
	if err != nil {
		return fmt.Errorf("failed to fetch projects: %w", err)
	}

	if len(projects) == 0 {
		fmt.Println("No projects found")
		return nil
	}

	fmt.Println("Projects:")
	fmt.Println("=========")
	for i, project := range projects {
		fmt.Printf("%d. %s\n", i+1, project.Name)
		fmt.Printf("   Path: %s\n", project.Path)
		fmt.Printf("   Sessions: %d\n", project.SessionCount)
		fmt.Printf("   Last Activity: %s\n", project.LastActivity.Format("2006-01-02 15:04"))
		fmt.Println()
	}
	
	return nil
}

func showSessions(projectName string) error {
	// First, find the project by name
	projects, err := sessions.FetchProjectsWithStats()
	if err != nil {
		return fmt.Errorf("failed to fetch projects: %w", err)
	}

	var targetProject *models.Project
	for _, project := range projects {
		if project.Name == projectName || project.Path == projectName {
			p := project
			targetProject = &p
			break
		}
	}

	if targetProject == nil {
		return fmt.Errorf("project '%s' not found", projectName)
	}

	// Fetch sessions for the project
	projectSessions, err := sessions.FetchSessionsForProject(targetProject.Path)
	if err != nil {
		return fmt.Errorf("failed to fetch sessions: %w", err)
	}

	if len(projectSessions) == 0 {
		fmt.Printf("No sessions found for project '%s'\n", projectName)
		return nil
	}

	fmt.Printf("Sessions for project '%s':\n", targetProject.Name)
	fmt.Printf("Path: %s\n", targetProject.Path)
	fmt.Println("===================================")
	
	for i, session := range projectSessions {
		fmt.Printf("%d. Session ID: %s\n", i+1, session.SessionID)
		fmt.Printf("   Last Activity: %s\n", session.LastActivity.Format("2006-01-02 15:04"))
		
		// Fetch and show recent messages
		messages, err := sessions.FetchRecentMessagesForSession(session.SessionID)
		if err == nil && len(messages) > 0 {
			fmt.Println("   Recent Messages:")
			for j, msg := range messages {
				if j >= 5 {
					break
				}
				truncatedMsg := truncateString(msg, 50)
				fmt.Printf("     %d. %s\n", j+1, truncatedMsg)
			}
		}
		fmt.Println()
	}
	
	return nil
}

func showMessages(projectName, sessionID string) error {
	// First, verify the project exists
	projects, err := sessions.FetchProjectsWithStats()
	if err != nil {
		return fmt.Errorf("failed to fetch projects: %w", err)
	}

	var targetProject *models.Project
	for _, project := range projects {
		if project.Name == projectName || project.Path == projectName {
			p := project
			targetProject = &p
			break
		}
	}

	if targetProject == nil {
		return fmt.Errorf("project '%s' not found", projectName)
	}

	// First check if the session exists for this project
	projectSessions, err := sessions.FetchSessionsForProject(targetProject.Path)
	if err != nil {
		return fmt.Errorf("failed to fetch sessions: %w", err)
	}

	sessionFound := false
	for _, session := range projectSessions {
		if session.SessionID == sessionID {
			sessionFound = true
			break
		}
	}

	if !sessionFound {
		fmt.Printf("Session '%s' not found in project '%s'\n", sessionID, projectName)
		fmt.Printf("\nAvailable sessions in this project:\n")
		for i, session := range projectSessions {
			if i >= 10 {
				fmt.Printf("... and %d more sessions\n", len(projectSessions)-10)
				break
			}
			fmt.Printf("  - %s (Last activity: %s)\n", session.SessionID, session.LastActivity.Format("2006-01-02 15:04"))
		}
		return nil
	}

	// Fetch messages for the session
	messages, err := sessions.FetchRecentMessagesForSession(sessionID)
	if err != nil {
		return fmt.Errorf("failed to fetch messages: %w", err)
	}

	if len(messages) == 0 {
		fmt.Printf("No messages found for session '%s' in project '%s'\n", sessionID, projectName)
		fmt.Println("\nThis might mean the session has no user messages or the messages couldn't be parsed.")
		return nil
	}

	fmt.Printf("Recent messages for session '%s' in project '%s':\n", sessionID, targetProject.Name)
	fmt.Println("================================================")
	
	for i, msg := range messages {
		if i >= 5 {
			fmt.Println("\n(showing first 5 messages only)")
			break
		}
		fmt.Printf("\n%d. %s\n", i+1, msg)
	}
	
	return nil
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}