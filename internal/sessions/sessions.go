package sessions

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/strrl/claude-resume/internal/db"
	"github.com/strrl/claude-resume/pkg/models"
)

// FetchProjectsWithStats fetches all projects with aggregated session statistics
func FetchProjectsWithStats() ([]models.Project, error) {
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
	// Don't close the singleton connection

	// Optimized query to get projects with aggregated stats
	// Using a single pass through the data with direct aggregation
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

	rows, err := database.Query(projectsQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to execute projects query: %w", err)
	}
	defer rows.Close()

	var projects []models.Project
	for rows.Next() {
		var project models.Project
		var lastActivity sql.NullString
		
		if err := rows.Scan(&project.Path, &project.SessionCount, &lastActivity); err != nil {
			continue
		}
		
		// Extract project name from path
		if project.Path == "Unknown" || project.Path == "" {
			project.Name = "Unknown"
		} else {
			project.Name = filepath.Base(project.Path)
		}
		
		// Parse timestamp and convert to local time
		if lastActivity.Valid {
			if t, err := time.Parse(time.RFC3339, lastActivity.String); err == nil {
				project.LastActivity = t.Local()
			} else {
				project.LastActivity = time.Now()
			}
		} else {
			project.LastActivity = time.Now()
		}
		
		projects = append(projects, project)
	}
	
	return projects, nil
}

// batchFetchSummaries fetches summaries for multiple sessions in batch
func batchFetchSummaries(sessionIDs []string, globPattern string, database *sql.DB) map[string]string {
	summaries := make(map[string]string)
	
	if len(sessionIDs) == 0 {
		return summaries
	}
	
	// Build placeholders for IN clause
	placeholders := make([]string, len(sessionIDs))
	args := make([]interface{}, len(sessionIDs))
	for i, id := range sessionIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	
	// Query 1: Get last UUID for each session
	lastUuidsQuery := fmt.Sprintf(`
		WITH last_events AS (
			SELECT 
				CAST(sessionId AS VARCHAR) as session_id,
				CAST(uuid AS VARCHAR) as uuid_str,
				ROW_NUMBER() OVER (PARTITION BY sessionId ORDER BY timestamp DESC) as rn
			FROM read_json('%s',
				format = 'newline_delimited',
				union_by_name = true,
				filename = true
			)
			WHERE CAST(sessionId AS VARCHAR) IN (%s)
			AND type <> 'summary'
		)
		SELECT session_id, uuid_str
		FROM last_events
		WHERE rn = 1
	`, globPattern, strings.Join(placeholders, ","))
	
	rows, err := database.Query(lastUuidsQuery, args...)
	if err != nil {
		return summaries
	}
	defer rows.Close()
	
	sessionUuids := make(map[string]string)
	for rows.Next() {
		var sessionID, uuid string
		if err := rows.Scan(&sessionID, &uuid); err == nil {
			sessionUuids[sessionID] = uuid
		}
	}
	
	if len(sessionUuids) == 0 {
		return summaries
	}
	
	// Query 2: Get summaries for those UUIDs
	uuids := make([]string, 0, len(sessionUuids))
	uuidToSession := make(map[string]string)
	for sessionID, uuid := range sessionUuids {
		uuids = append(uuids, uuid)
		uuidToSession[uuid] = sessionID
	}
	
	placeholders2 := make([]string, len(uuids))
	args2 := make([]interface{}, len(uuids))
	for i, uuid := range uuids {
		placeholders2[i] = "?"
		args2[i] = uuid
	}
	
	summariesQuery := fmt.Sprintf(`
		SELECT 
			CAST(leafUuid AS VARCHAR) as leaf_uuid,
			summary
		FROM read_json('%s',
			format = 'newline_delimited',
			union_by_name = true,
			filename = true
		)
		WHERE type = 'summary'
		AND CAST(leafUuid AS VARCHAR) IN (%s)
	`, globPattern, strings.Join(placeholders2, ","))
	
	rows2, err := database.Query(summariesQuery, args2...)
	if err != nil {
		return summaries
	}
	defer rows2.Close()
	
	for rows2.Next() {
		var leafUuid, summary string
		if err := rows2.Scan(&leafUuid, &summary); err == nil {
			if sessionID, ok := uuidToSession[leafUuid]; ok {
				summaries[sessionID] = summary
			}
		}
	}
	
	return summaries
}

// FetchSessionsForProject fetches all sessions for a specific project
func FetchSessionsForProject(projectPath string) ([]models.Session, error) {
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
	// Don't close the singleton connection

	// Query to get sessions with resume status
	var sessionsQuery string
	if projectPath == "Unknown" {
		// Special case for sessions without a cwd
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
	}

	var rows *sql.Rows
	if projectPath == "Unknown" {
		rows, err = database.Query(sessionsQuery)
	} else {
		rows, err = database.Query(sessionsQuery, projectPath, projectPath)
	}
	
	if err != nil {
		return nil, fmt.Errorf("failed to execute sessions query: %w", err)
	}
	defer rows.Close()

	var sessions []models.Session
	sessionIDs := []string{}
	
	for rows.Next() {
		var session models.Session
		var lastActivity sql.NullString
		var isResumed bool
		
		if err := rows.Scan(&session.SessionID, &lastActivity, &isResumed); err != nil {
			continue
		}
		
		session.IsResumed = isResumed
		
		session.ProjectPath = projectPath
		
		// Parse timestamp and convert to local time
		if lastActivity.Valid {
			if t, err := time.Parse(time.RFC3339, lastActivity.String); err == nil {
				session.LastActivity = t.Local()
			} else {
				session.LastActivity = time.Now()
			}
		} else {
			session.LastActivity = time.Now()
		}
		
		sessions = append(sessions, session)
		sessionIDs = append(sessionIDs, session.SessionID)
	}
	
	// Batch fetch summaries for all sessions
	if len(sessionIDs) > 0 {
		summaries := batchFetchSummaries(sessionIDs, globPattern, database)
		for i := range sessions {
			if summary, ok := summaries[sessions[i].SessionID]; ok {
				sessions[i].Summary = summary
			}
		}
	}
	
	return sessions, nil
}

// FetchSummaryForSession fetches the summary for a specific session
func FetchSummaryForSession(sessionID string) string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	claudeDir := filepath.Join(homeDir, ".claude", "projects")
	globPattern := filepath.Join(claudeDir, "**", "*.jsonl")

	database, err := db.GetDB()
	if err != nil {
		return ""
	}

	// First query: Find the last UUID for this session
	lastUuidQuery := fmt.Sprintf(`
		SELECT 
			CAST(uuid AS VARCHAR) as uuid_str
		FROM read_json('%s',
			format = 'newline_delimited',
			union_by_name = true,
			filename = true
		)
		WHERE CAST(sessionId AS VARCHAR) = ?
		AND type <> 'summary'
		ORDER BY timestamp DESC
		LIMIT 1
	`, globPattern)

	var lastUuid string
	uuidRow := database.QueryRow(lastUuidQuery, sessionID)
	var uuidVal sql.NullString
	if err := uuidRow.Scan(&uuidVal); err == nil && uuidVal.Valid {
		lastUuid = uuidVal.String
	}

	// Second query: Find summary with matching leafUuid if we have a lastUuid
	if lastUuid != "" {
		summaryQuery := fmt.Sprintf(`
			SELECT 
				summary
			FROM read_json('%s',
				format = 'newline_delimited',
				union_by_name = true,
				filename = true
			)
			WHERE type = 'summary'
			AND CAST(leafUuid AS VARCHAR) = ?
			LIMIT 1
		`, globPattern)

		summaryRow := database.QueryRow(summaryQuery, lastUuid)
		var summary sql.NullString
		if err := summaryRow.Scan(&summary); err == nil && summary.Valid {
			return summary.String
		}
	}

	return ""
}

// FetchRecentMessagesForSession fetches the first 10 and last 10 messages for a session
func FetchRecentMessagesForSession(sessionID string) ([]string, error) {
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
	// Don't close the singleton connection

	// Fetch first 10 and last 10 messages for a complete conversation view
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

	rows, err := database.Query(messagesQuery, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to execute messages query: %w", err)
	}
	defer rows.Close()

	var messages []string
	var firstMessages []string
	var lastMessages []string
	var totalCount int64
	lastPosition := ""
	
	for rows.Next() {
		var messageType sql.NullString
		var messageJSON sql.NullString
		var position sql.NullString
		var count sql.NullInt64
		
		if err := rows.Scan(&messageType, &messageJSON, &position, &count); err != nil {
			continue
		}
		
		if count.Valid {
			totalCount = count.Int64
		}
		
		if messageJSON.Valid && messageJSON.String != "" && messageType.Valid && position.Valid {
			// Extract and format message with role
			formattedMsg := formatMessageWithRole(messageType.String, messageJSON.String)
			if formattedMsg != "" {
				if position.String == "first" {
					firstMessages = append(firstMessages, formattedMsg)
					lastPosition = "first"
				} else if position.String == "last" {
					// Only add to last messages if we've transitioned from first
					if lastPosition == "first" && len(lastMessages) == 0 {
						// Add separator if there are middle messages that were skipped
						if totalCount > 20 {
							messages = append(messages, firstMessages...)
							messages = append(messages, fmt.Sprintf("... (%d messages omitted) ...", totalCount-20))
							lastMessages = append(lastMessages, formattedMsg)
						} else {
							// No middle messages, just combine
							firstMessages = append(firstMessages, formattedMsg)
						}
					} else {
						lastMessages = append(lastMessages, formattedMsg)
					}
					lastPosition = "last"
				}
			}
		}
	}
	
	// Combine the messages
	if len(lastMessages) > 0 {
		messages = append(messages, lastMessages...)
	} else {
		messages = firstMessages
	}
	
	return messages, nil
}

// formatMessageWithRole formats a message with its role and truncated content
func formatMessageWithRole(messageType, messageStr string) string {
	// First, check if it's a JSON string that needs to be unescaped
	if strings.HasPrefix(messageStr, `"`) && strings.HasSuffix(messageStr, `"`) {
		var unquoted string
		if err := json.Unmarshal([]byte(messageStr), &unquoted); err == nil {
			messageStr = unquoted
		}
	}
	
	// Try to parse as message object
	var message map[string]interface{}
	if err := json.Unmarshal([]byte(messageStr), &message); err != nil {
		return ""
	}
	
	// Get the content field
	contentRaw, ok := message["content"]
	if !ok {
		return ""
	}
	
	// Format based on message type
	rolePrefix := ""
	switch messageType {
	case "user":
		rolePrefix = "[User] "
	case "assistant":
		rolePrefix = "[Assistant] "
	default:
		rolePrefix = fmt.Sprintf("[%s] ", messageType)
	}
	
	// Handle different content formats
	switch content := contentRaw.(type) {
	case string:
		// Simple string content - truncate to 50 chars
		truncated := truncateString(content, 50)
		return rolePrefix + truncated
		
	case []interface{}:
		// Array of content items - could be text or tool use
		var result []string
		
		for _, item := range content {
			if itemMap, ok := item.(map[string]interface{}); ok {
				// Check type field
				if typeStr, ok := itemMap["type"].(string); ok {
					switch typeStr {
					case "text":
						// Text message
						if text, ok := itemMap["text"].(string); ok && text != "" {
							// Skip system reminders
							if !strings.Contains(text, "system-reminder") {
								truncated := truncateString(text, 50)
								result = append(result, truncated)
							}
						}
						
					case "tool_use":
						// Tool call from assistant
						toolName := "unknown"
						if name, ok := itemMap["name"].(string); ok {
							toolName = name
						}
						
						// Get truncated input
						inputStr := ""
						if input, ok := itemMap["input"].(map[string]interface{}); ok {
							// Try to get a summary of the input
							if cmd, ok := input["command"].(string); ok {
								inputStr = truncateString(cmd, 30)
							} else if path, ok := input["file_path"].(string); ok {
								inputStr = filepath.Base(path)
							} else if pattern, ok := input["pattern"].(string); ok {
								inputStr = truncateString(pattern, 20)
							} else {
								// Generic truncation of input
								inputBytes, _ := json.Marshal(input)
								inputStr = truncateString(string(inputBytes), 30)
							}
						}
						
						if inputStr != "" {
							result = append(result, fmt.Sprintf("🔧 %s: %s", toolName, inputStr))
						} else {
							result = append(result, fmt.Sprintf("🔧 %s", toolName))
						}
						
					case "tool_result":
						// Tool result from user
						if content, ok := itemMap["content"].(string); ok {
							// Show truncated tool result
							truncated := truncateString(content, 40)
							result = append(result, fmt.Sprintf("↩ %s", truncated))
						}
					}
				}
			}
		}
		
		if len(result) > 0 {
			return rolePrefix + strings.Join(result, " | ")
		}
	}
	
	return ""
}

// truncateString truncates a string to maxLen characters
func truncateString(s string, maxLen int) string {
	// Remove newlines and excessive whitespace
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.Join(strings.Fields(s), " ")
	
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// ExecuteClaudeResume changes to project directory and executes claude --resume
func ExecuteClaudeResume(sessionID string, projectPath string) error {
	// Change to project directory first
	if projectPath != "" && projectPath != "Unknown" {
		if err := os.Chdir(projectPath); err != nil {
			return fmt.Errorf("failed to change to project directory %s: %w", projectPath, err)
		}
	}
	
	// Try to find claude executable
	claudePath := "claude"
	
	// Check if claude is in PATH
	if _, err := exec.LookPath("claude"); err != nil {
		// Check common installation locations
		homeDir, _ := os.UserHomeDir()
		possiblePaths := []string{
			filepath.Join(homeDir, ".claude", "local", "claude"),
			"/usr/local/bin/claude",
			"/opt/homebrew/bin/claude",
		}
		
		for _, path := range possiblePaths {
			if _, err := os.Stat(path); err == nil {
				claudePath = path
				break
			}
		}
	}
	
	cmd := exec.Command(claudePath, "--resume", sessionID)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// SessionDebugInfo contains debug information about a session
type SessionDebugInfo struct {
	Summary  string
	Messages []string
}

// DebugSessionMessages returns debug information about messages in a session
func DebugSessionMessages(sessionID string) (*SessionDebugInfo, error) {
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
	// Don't close the singleton connection

	debugInfo := &SessionDebugInfo{
		Messages: []string{},
	}

	// First query: Find the last UUID for this session
	lastUuidQuery := fmt.Sprintf(`
		SELECT 
			CAST(uuid AS VARCHAR) as uuid_str
		FROM read_json('%s',
			format = 'newline_delimited',
			union_by_name = true,
			filename = true
		)
		WHERE CAST(sessionId AS VARCHAR) = ?
		AND type <> 'summary'
		ORDER BY timestamp DESC
		LIMIT 1
	`, globPattern)

	var lastUuid string
	uuidRow := database.QueryRow(lastUuidQuery, sessionID)
	var uuidVal sql.NullString
	if err := uuidRow.Scan(&uuidVal); err == nil && uuidVal.Valid {
		lastUuid = uuidVal.String
	}

	// Second query: Find summary with matching leafUuid if we have a lastUuid
	if lastUuid != "" {
		summaryQuery := fmt.Sprintf(`
			SELECT 
				summary
			FROM read_json('%s',
				format = 'newline_delimited',
				union_by_name = true,
				filename = true
			)
			WHERE type = 'summary'
			AND CAST(leafUuid AS VARCHAR) = ?
			LIMIT 1
		`, globPattern)

		summaryRow := database.QueryRow(summaryQuery, lastUuid)
		var summary sql.NullString
		if err := summaryRow.Scan(&summary); err == nil && summary.Valid {
			debugInfo.Summary = summary.String
		}
	}

	// Then, let's find any actual user text messages
	textQuery := fmt.Sprintf(`
		SELECT 
			type,
			to_json(message) as message_json,
			timestamp
		FROM read_json('%s',
			format = 'newline_delimited',
			union_by_name = true,
			filename = true
		)
		WHERE CAST(sessionId AS VARCHAR) = ?
		AND type = 'user'
		ORDER BY timestamp ASC
	`, globPattern)

	rows, err := database.Query(textQuery, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to execute debug query: %w", err)
	}
	defer rows.Close()

	userMsgCount := 0
	for rows.Next() {
		var eventType sql.NullString
		var messageJSON sql.NullString
		var timestamp sql.NullString
		
		if err := rows.Scan(&eventType, &messageJSON, &timestamp); err != nil {
			continue
		}
		
		userMsgCount++
		if messageJSON.Valid && messageJSON.String != "" {
			// Parse the message to look for actual text content
			var msgObj map[string]interface{}
			if err := json.Unmarshal([]byte(messageJSON.String), &msgObj); err == nil {
				if content, ok := msgObj["content"].([]interface{}); ok {
					for _, item := range content {
						if itemMap, ok := item.(map[string]interface{}); ok {
							// Look for text type messages
							if typeStr, _ := itemMap["type"].(string); typeStr == "text" {
								if text, ok := itemMap["text"].(string); ok {
									msg := fmt.Sprintf("User Message %d (text) at %s:\n%s", 
										userMsgCount, timestamp.String, text)
									debugInfo.Messages = append(debugInfo.Messages, msg)
								}
							} else if typeStr == "tool_result" {
								// This is a tool result, skip for now but count it
								msg := fmt.Sprintf("User Message %d (tool_result) at %s: [Tool Result]", 
									userMsgCount, timestamp.String)
								debugInfo.Messages = append(debugInfo.Messages, msg)
							}
						}
					}
				} else if content, ok := msgObj["content"].(string); ok {
					// Direct string content
					msg := fmt.Sprintf("User Message %d (string) at %s:\n%s", 
						userMsgCount, timestamp.String, content)
					debugInfo.Messages = append(debugInfo.Messages, msg)
				}
			}
		}
	}

	if len(debugInfo.Messages) == 0 && userMsgCount > 0 {
		debugInfo.Messages = append(debugInfo.Messages, fmt.Sprintf("Found %d user events but no text messages", userMsgCount))
	}

	return debugInfo, nil
}