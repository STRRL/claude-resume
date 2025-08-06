package sessions

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/strrl/claude-resume/internal/db"
	"github.com/strrl/claude-resume/pkg/models"
)

// FetchProjectsWithStatsAsync fetches projects asynchronously
func FetchProjectsWithStatsAsync(ctx context.Context) ([]models.Project, error) {
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

	projectsQuery := fmt.Sprintf(`
		SELECT 
			COALESCE(cwd, 'Unknown') as project_path,
			COUNT(DISTINCT CAST(sessionId AS VARCHAR)) as session_count,
			MAX(timestamp) as last_activity
		FROM read_json('%s',
			format = 'newline_delimited',
			union_by_name = true,
			filename = true
		)
		WHERE sessionId IS NOT NULL
		GROUP BY cwd
		HAVING COUNT(DISTINCT CAST(sessionId AS VARCHAR)) > 0
		ORDER BY MAX(timestamp) DESC
		LIMIT 100
	`, globPattern)

	// Execute query asynchronously with context
	resultChan := ExecuteProjectsQueryAsync(ctx, database, projectsQuery)

	// Wait for result or cancellation
	select {
	case result := <-resultChan:
		if result.Error != nil {
			return nil, result.Error
		}
		return result.Projects, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// FetchSessionsForProjectAsync fetches sessions asynchronously
func FetchSessionsForProjectAsync(ctx context.Context, projectPath string) ([]models.Session, error) {
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

	var sessionsQuery string
	var args []interface{}

	if projectPath == "Unknown" {
		sessionsQuery = fmt.Sprintf(`
			WITH first_events AS (
				SELECT 
					CAST(sessionId AS VARCHAR) as session_id,
					parentUuid,
					timestamp,
					ROW_NUMBER() OVER (PARTITION BY sessionId ORDER BY timestamp ASC) as rn
				FROM read_json('%s',
					format = 'newline_delimited',
					union_by_name = true,
					filename = true
				)
				WHERE sessionId IS NOT NULL
				AND (cwd IS NULL OR cwd = '')
			)
			SELECT 
				fe.session_id,
				MAX(e.timestamp) as last_activity,
				CASE WHEN MIN(CASE WHEN fe.rn = 1 THEN fe.parentUuid END) IS NULL THEN false ELSE true END as is_resumed
			FROM first_events fe
			JOIN (
				SELECT CAST(sessionId AS VARCHAR) as session_id, timestamp
				FROM read_json('%s',
					format = 'newline_delimited',
					union_by_name = true,
					filename = true
				)
				WHERE sessionId IS NOT NULL
				AND (cwd IS NULL OR cwd = '')
			) e ON e.session_id = fe.session_id
			GROUP BY fe.session_id
			ORDER BY MAX(e.timestamp) DESC
			LIMIT 100
		`, globPattern, globPattern)
	} else {
		sessionsQuery = fmt.Sprintf(`
			WITH first_events AS (
				SELECT 
					CAST(sessionId AS VARCHAR) as session_id,
					parentUuid,
					timestamp,
					ROW_NUMBER() OVER (PARTITION BY sessionId ORDER BY timestamp ASC) as rn
				FROM read_json('%s',
					format = 'newline_delimited',
					union_by_name = true,
					filename = true
				)
				WHERE sessionId IS NOT NULL
				AND cwd = ?
			)
			SELECT 
				fe.session_id,
				MAX(e.timestamp) as last_activity,
				CASE WHEN MIN(CASE WHEN fe.rn = 1 THEN fe.parentUuid END) IS NULL THEN false ELSE true END as is_resumed
			FROM first_events fe
			JOIN (
				SELECT CAST(sessionId AS VARCHAR) as session_id, timestamp
				FROM read_json('%s',
					format = 'newline_delimited',
					union_by_name = true,
					filename = true
				)
				WHERE sessionId IS NOT NULL
				AND cwd = ?
			) e ON e.session_id = fe.session_id
			GROUP BY fe.session_id
			ORDER BY MAX(e.timestamp) DESC
			LIMIT 100
		`, globPattern, globPattern)
		args = []interface{}{projectPath, projectPath}
	}

	// Execute query asynchronously
	resultChan := ExecuteSessionsQueryAsync(ctx, database, sessionsQuery, args...)

	select {
	case result := <-resultChan:
		if result.Error != nil {
			return nil, result.Error
		}
		
		// Set project path for all sessions
		for i := range result.Sessions {
			result.Sessions[i].ProjectPath = projectPath
		}

		// Return sessions immediately without summaries for fast response
		// Summaries will be loaded in a separate async call if needed
		// This provides instant feedback to the user

		return result.Sessions, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// FetchRecentMessagesForSessionAsync fetches messages asynchronously
func FetchRecentMessagesForSessionAsync(ctx context.Context, sessionID string) ([]string, error) {
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

	messagesQuery := fmt.Sprintf(`
		WITH all_messages AS (
			SELECT 
				type,
				to_json(message) as message_json,
				timestamp,
				ROW_NUMBER() OVER (ORDER BY timestamp ASC) as row_num_asc,
				ROW_NUMBER() OVER (ORDER BY timestamp DESC) as row_num_desc,
				COUNT(*) OVER () as total_count
			FROM read_json('%s',
				format = 'newline_delimited',
				union_by_name = true,
				filename = true
			)
			WHERE CAST(sessionId AS VARCHAR) = ?
			AND type IN ('user', 'assistant')
			AND message IS NOT NULL
		)
		SELECT 
			type,
			message_json,
			CASE 
				WHEN row_num_asc <= 10 THEN 'first'
				WHEN row_num_desc <= 10 THEN 'last'
			END as position,
			total_count
		FROM all_messages
		WHERE row_num_asc <= 10 OR row_num_desc <= 10
		ORDER BY timestamp ASC
	`, globPattern)

	// Execute query asynchronously
	resultChan := ExecuteMessagesQueryAsync(ctx, database, messagesQuery, sessionID)

	select {
	case result := <-resultChan:
		if result.Error != nil {
			return nil, result.Error
		}
		return result.Messages, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// batchFetchSummariesAsync fetches summaries asynchronously
func batchFetchSummariesAsync(ctx context.Context, sessionIDs []string, globPattern string, database *sql.DB) map[string]string {
	summaries := make(map[string]string)

	if len(sessionIDs) == 0 {
		return summaries
	}

	// Use goroutine with context for async execution
	type summaryResult struct {
		sessionID string
		summary   string
	}

	resultChan := make(chan summaryResult, len(sessionIDs))
	
	go func() {
		defer close(resultChan)

		// Reuse existing batchFetchSummaries logic but with context checks
		for sessionID, summary := range batchFetchSummaries(sessionIDs, globPattern, database) {
			select {
			case <-ctx.Done():
				return
			case resultChan <- summaryResult{sessionID: sessionID, summary: summary}:
			}
		}
	}()

	// Collect results
	for {
		select {
		case result, ok := <-resultChan:
			if !ok {
				return summaries
			}
			summaries[result.sessionID] = result.summary
		case <-ctx.Done():
			return summaries
		}
	}
}