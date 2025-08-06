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
		
		// Parse timestamp
		if lastActivity.Valid {
			if t, err := time.Parse(time.RFC3339, lastActivity.String); err == nil {
				project.LastActivity = t
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

	// Optimized query to get sessions for a specific project
	// Direct aggregation without CTE for better performance
	var sessionsQuery string
	if projectPath == "Unknown" {
		// Special case for sessions without a cwd
		sessionsQuery = fmt.Sprintf(`
			SELECT 
				CAST(sessionId AS VARCHAR) as session_id,
				MAX(timestamp) as last_activity
			FROM read_json('%s',
				format = 'newline_delimited',
				union_by_name = true,
				filename = true
			)
			WHERE sessionId IS NOT NULL
			AND (cwd IS NULL OR cwd = '')
			GROUP BY sessionId
			ORDER BY MAX(timestamp) DESC
			LIMIT 100
		`, globPattern)
	} else {
		sessionsQuery = fmt.Sprintf(`
			SELECT 
				CAST(sessionId AS VARCHAR) as session_id,
				MAX(timestamp) as last_activity
			FROM read_json('%s',
				format = 'newline_delimited',
				union_by_name = true,
				filename = true
			)
			WHERE sessionId IS NOT NULL
			AND cwd = ?
			GROUP BY sessionId
			ORDER BY MAX(timestamp) DESC
			LIMIT 100
		`, globPattern)
	}

	var rows *sql.Rows
	if projectPath == "Unknown" {
		rows, err = database.Query(sessionsQuery)
	} else {
		rows, err = database.Query(sessionsQuery, projectPath)
	}
	
	if err != nil {
		return nil, fmt.Errorf("failed to execute sessions query: %w", err)
	}
	defer rows.Close()

	var sessions []models.Session
	for rows.Next() {
		var session models.Session
		var lastActivity sql.NullString
		
		if err := rows.Scan(&session.SessionID, &lastActivity); err != nil {
			continue
		}
		
		session.ProjectPath = projectPath
		
		// Parse timestamp
		if lastActivity.Valid {
			if t, err := time.Parse(time.RFC3339, lastActivity.String); err == nil {
				session.LastActivity = t
			} else {
				session.LastActivity = time.Now()
			}
		} else {
			session.LastActivity = time.Now()
		}
		
		sessions = append(sessions, session)
	}
	
	return sessions, nil
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
	
	cmd := exec.Command("claude", "--resume", sessionID)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// DebugSessionMessages returns debug information about messages in a session
func DebugSessionMessages(sessionID string) ([]string, error) {
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

	// First, let's find any actual user text messages
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

	var messages []string
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
									messages = append(messages, msg)
								}
							} else if typeStr == "tool_result" {
								// This is a tool result, skip for now but count it
								msg := fmt.Sprintf("User Message %d (tool_result) at %s: [Tool Result]", 
									userMsgCount, timestamp.String)
								messages = append(messages, msg)
							}
						}
					}
				} else if content, ok := msgObj["content"].(string); ok {
					// Direct string content
					msg := fmt.Sprintf("User Message %d (string) at %s:\n%s", 
						userMsgCount, timestamp.String, content)
					messages = append(messages, msg)
				}
			}
		}
	}

	if len(messages) == 0 {
		messages = append(messages, fmt.Sprintf("Found %d user events but no text messages", userMsgCount))
	}

	return messages, nil
}