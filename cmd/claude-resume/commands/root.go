package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/strrl/claude-resume/internal/sessions"
	"github.com/strrl/claude-resume/internal/tui"
	"github.com/strrl/claude-resume/pkg/models"
)

var debugMode bool

// NewRootCommand creates the root command
func NewRootCommand() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "claude-resume",
		Short: "Browse and resume recent Claude Code sessions",
		Long:  `claude-resume is a TUI application for browsing and resuming recent Claude Code sessions.`,
		RunE:  runTUI,
	}

	rootCmd.PersistentFlags().BoolVar(&debugMode, "debug", false, "Run in debug mode (list sessions without TUI)")
	rootCmd.AddCommand(NewShowCommand())
	rootCmd.AddCommand(NewDebugCommand())

	return rootCmd
}

// Execute runs the root command
func Execute() {
	rootCmd := NewRootCommand()
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runTUI(cmd *cobra.Command, args []string) error {
	projects, err := sessions.FetchProjectsWithStats()
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

	selectedSession, err := tui.ShowTUI(projects)
	if err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	if selectedSession == nil {
		return nil
	}

	return sessions.ExecuteClaudeResume(selectedSession.SessionID, selectedSession.ProjectPath)
}

func runDebugMode(projects []models.Project) error {
	fmt.Println("=== Debug Mode: Projects and Sessions ===")
	for i, project := range projects {
		fmt.Printf("\n%d. Project: %s\n", i+1, project.Name)
		fmt.Printf("   Path: %s\n", project.Path)
		fmt.Printf("   Sessions: %d\n", project.SessionCount)
		fmt.Printf("   Last Activity: %s\n", project.LastActivity.Format("2006-01-02 15:04"))
		
		if i == 0 {
			// Load sessions for the first project as an example
			projectSessions, err := sessions.FetchSessionsForProject(project.Path)
			if err != nil {
				fmt.Printf("   Error loading sessions: %v\n", err)
				continue
			}
			
			fmt.Println("   Sample sessions:")
			for j, session := range projectSessions {
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