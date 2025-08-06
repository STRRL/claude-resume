package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	debugMode bool
	rootCmd   = &cobra.Command{
		Use:   "claude-resume",
		Short: "Browse and resume recent Claude Code sessions",
		Long:  `claude-resume is a TUI application for browsing and resuming recent Claude Code sessions.`,
		RunE:  runTUI,
	}

	showCmd = &cobra.Command{
		Use:   "show [project] [session-id]",
		Short: "Show projects, sessions, or messages without TUI",
		Long: `Show projects, sessions, or messages in a non-interactive format.
Without arguments: lists all projects
With project name: lists all sessions in that project
With project name and session ID: shows recent messages for that session`,
		RunE: runShow,
	}

	debugCmd = &cobra.Command{
		Use:   "debug-session <session-id>",
		Short: "Debug a specific session to see raw data",
		Args:  cobra.ExactArgs(1),
		RunE:  runDebugSession,
	}
)

func init() {
	rootCmd.PersistentFlags().BoolVar(&debugMode, "debug", false, "Run in debug mode (list sessions without TUI)")
	rootCmd.AddCommand(showCmd)
	rootCmd.AddCommand(debugCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runTUI(cmd *cobra.Command, args []string) error {
	projects, err := fetchProjectsWithStats()
	if err != nil {
		return fmt.Errorf("failed to fetch projects: %w", err)
	}

	if len(projects) == 0 {
		fmt.Println("No projects found")
		return nil
	}

	// Debug mode: just list projects and sessions without TUI
	if debugMode {
		return runDebugMode(projects)
	}

	selectedSession, err := showTUI(projects)
	if err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	if selectedSession == nil {
		return nil
	}

	return executeClaudeResume(selectedSession.SessionID, selectedSession.ProjectPath)
}

func runDebugMode(projects []Project) error {
	fmt.Println("=== Debug Mode: Projects and Sessions ===")
	for i, project := range projects {
		fmt.Printf("\n%d. Project: %s\n", i+1, project.Name)
		fmt.Printf("   Path: %s\n", project.Path)
		fmt.Printf("   Sessions: %d\n", project.SessionCount)
		fmt.Printf("   Last Activity: %s\n", project.LastActivity.Format("2006-01-02 15:04"))
		
		if i == 0 {
			// Load sessions for the first project as an example
			sessions, err := fetchSessionsForProject(project.Path)
			if err != nil {
				fmt.Printf("   Error loading sessions: %v\n", err)
				continue
			}
			
			fmt.Println("   Sample sessions:")
			for j, session := range sessions {
				if j >= 3 { // Only show first 3 sessions
					break
				}
				fmt.Printf("   - %s (Session: %s)\n", 
					session.LastActivity.Format("2006-01-02 15:04"),
					session.SessionID)
			}
		}
	}
	return nil
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
	projects, err := fetchProjectsWithStats()
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
	projects, err := fetchProjectsWithStats()
	if err != nil {
		return fmt.Errorf("failed to fetch projects: %w", err)
	}

	var targetProject *Project
	for _, project := range projects {
		if project.Name == projectName || project.Path == projectName {
			targetProject = &project
			break
		}
	}

	if targetProject == nil {
		return fmt.Errorf("project '%s' not found", projectName)
	}

	// Fetch sessions for the project
	sessions, err := fetchSessionsForProject(targetProject.Path)
	if err != nil {
		return fmt.Errorf("failed to fetch sessions: %w", err)
	}

	if len(sessions) == 0 {
		fmt.Printf("No sessions found for project '%s'\n", projectName)
		return nil
	}

	fmt.Printf("Sessions for project '%s':\n", targetProject.Name)
	fmt.Printf("Path: %s\n", targetProject.Path)
	fmt.Println("===================================")
	
	for i, session := range sessions {
		fmt.Printf("%d. Session ID: %s\n", i+1, session.SessionID)
		fmt.Printf("   Last Activity: %s\n", session.LastActivity.Format("2006-01-02 15:04"))
		
		// Fetch and show recent messages
		messages, err := fetchRecentMessagesForSession(session.SessionID)
		if err == nil && len(messages) > 0 {
			fmt.Println("   Recent Messages:")
			for j, msg := range messages {
				if j >= 5 {
					break
				}
				truncatedMsg := truncate(msg, 50)
				fmt.Printf("     %d. %s\n", j+1, truncatedMsg)
			}
		}
		fmt.Println()
	}
	
	return nil
}

func showMessages(projectName, sessionID string) error {
	// First, verify the project exists
	projects, err := fetchProjectsWithStats()
	if err != nil {
		return fmt.Errorf("failed to fetch projects: %w", err)
	}

	var targetProject *Project
	for _, project := range projects {
		if project.Name == projectName || project.Path == projectName {
			targetProject = &project
			break
		}
	}

	if targetProject == nil {
		return fmt.Errorf("project '%s' not found", projectName)
	}

	// First check if the session exists for this project
	sessions, err := fetchSessionsForProject(targetProject.Path)
	if err != nil {
		return fmt.Errorf("failed to fetch sessions: %w", err)
	}

	sessionFound := false
	for _, session := range sessions {
		if session.SessionID == sessionID {
			sessionFound = true
			break
		}
	}

	if !sessionFound {
		fmt.Printf("Session '%s' not found in project '%s'\n", sessionID, projectName)
		fmt.Printf("\nAvailable sessions in this project:\n")
		for i, session := range sessions {
			if i >= 10 {
				fmt.Printf("... and %d more sessions\n", len(sessions)-10)
				break
			}
			fmt.Printf("  - %s (Last activity: %s)\n", session.SessionID, session.LastActivity.Format("2006-01-02 15:04"))
		}
		return nil
	}

	// Fetch messages for the session
	messages, err := fetchRecentMessagesForSession(sessionID)
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

func runDebugSession(cmd *cobra.Command, args []string) error {
	sessionID := args[0]
	
	fmt.Printf("Debugging session: %s\n", sessionID)
	fmt.Println("==========================================")
	
	// Try to fetch raw data about this session
	messages, err := debugSessionMessages(sessionID)
	if err != nil {
		return fmt.Errorf("failed to debug session: %w", err)
	}
	
	if len(messages) == 0 {
		fmt.Println("No messages found for this session")
	} else {
		fmt.Printf("Found %d messages:\n", len(messages))
		for i, msg := range messages {
			fmt.Printf("\n--- Message %d ---\n%s\n", i+1, msg)
		}
	}
	
	return nil
}