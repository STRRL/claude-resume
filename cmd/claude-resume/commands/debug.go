package commands

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/strrl/claude-resume/internal/sessions"
)

// NewDebugCommand creates the debug-session command
func NewDebugCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "debug-session <session-id>",
		Short: "Debug a specific session to see raw data",
		Args:  cobra.ExactArgs(1),
		RunE:  runDebugSession,
	}
}

func runDebugSession(cmd *cobra.Command, args []string) error {
	sessionID := args[0]
	
	fmt.Printf("Debugging session: %s\n", sessionID)
	fmt.Println("==========================================")
	
	// Try to fetch raw data about this session
	debugInfo, err := sessions.DebugSessionMessages(sessionID)
	if err != nil {
		return fmt.Errorf("failed to debug session: %w", err)
	}
	
	// Display summary if available
	if debugInfo.Summary != "" {
		fmt.Println("\n=== SESSION SUMMARY ===")
		fmt.Printf("%s\n", debugInfo.Summary)
		fmt.Println("=======================")
	} else {
		fmt.Println("\n=== NO SUMMARY AVAILABLE ===")
	}
	
	// Display messages
	fmt.Println("\n=== MESSAGES ===")
	if len(debugInfo.Messages) == 0 {
		fmt.Println("No messages found for this session")
	} else {
		fmt.Printf("Found %d messages:\n", len(debugInfo.Messages))
		for i, msg := range debugInfo.Messages {
			fmt.Printf("\n--- Message %d ---\n%s\n", i+1, msg)
		}
	}
	
	return nil
}