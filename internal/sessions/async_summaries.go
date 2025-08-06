package sessions

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/strrl/claude-resume/internal/db"
)

// FetchSessionSummariesAsync fetches summaries for sessions asynchronously
func FetchSessionSummariesAsync(ctx context.Context, projectPath string, sessionIDs []string) (map[string]string, error) {
	if len(sessionIDs) == 0 {
		return make(map[string]string), nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	claudeDir := filepath.Join(homeDir, ".claude", "projects")
	globPattern := filepath.Join(claudeDir, "**", "*.jsonl")

	database, err := db.GetDB()
	if err != nil {
		return nil, err
	}

	// Use the existing batchFetchSummaries but with context support
	summariesChan := make(chan map[string]string, 1)
	
	go func() {
		// Check context before expensive operation
		select {
		case <-ctx.Done():
			summariesChan <- make(map[string]string)
			return
		default:
		}
		
		summaries := batchFetchSummaries(sessionIDs, globPattern, database)
		summariesChan <- summaries
	}()

	// Wait for result or context cancellation
	select {
	case summaries := <-summariesChan:
		return summaries, nil
	case <-ctx.Done():
		return make(map[string]string), ctx.Err()
	}
}