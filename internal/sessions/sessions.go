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

// FetchRecentMessagesForSession fetches the last 5 user text messages for a session
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

	// Optimized SQL query:
	// 1. Filter by sessionId and type='user' at the database level
	// 2. Order by timestamp to get most recent messages
	// 3. Use a subquery to reverse order for final output
	// 4. Fetch more than 5 to account for tool results, but limit to reasonable amount
	messagesQuery := fmt.Sprintf(`
		SELECT message_json FROM (
			SELECT 
				to_json(message) as message_json,
				timestamp
			FROM read_json('%s',
				format = 'newline_delimited',
				union_by_name = true,
				filename = true
			)
			WHERE CAST(sessionId AS VARCHAR) = ?
			AND type = 'user'
			AND message IS NOT NULL
			ORDER BY timestamp DESC
			LIMIT 30
		) AS recent_messages
		ORDER BY timestamp ASC
	`, globPattern)

	rows, err := database.Query(messagesQuery, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to execute messages query: %w", err)
	}
	defer rows.Close()

	var messages []string
	for rows.Next() {
		var messageJSON sql.NullString
		if err := rows.Scan(&messageJSON); err != nil {
			continue
		}
		
		if messageJSON.Valid && messageJSON.String != "" {
			// Extract content from the message JSON in Go
			content := extractMessageContent(messageJSON.String)
			// Add the extracted content only if it's actual user text
			if content != "" {
				messages = append(messages, strings.TrimSpace(content))
				// Stop after we have 5 actual text messages
				if len(messages) >= 5 {
					break
				}
			}
		}
	}

	return messages, nil
}

// extractMessageContent extracts text content from Claude message format
func extractMessageContent(messageStr string) string {
	// First, check if it's a JSON string that needs to be unescaped
	if strings.HasPrefix(messageStr, `"`) && strings.HasSuffix(messageStr, `"`) {
		// It's a quoted string, need to unmarshal it first
		var unquoted string
		if err := json.Unmarshal([]byte(messageStr), &unquoted); err == nil {
			messageStr = unquoted
		}
	}
	
	// Try to parse as message object
	var message map[string]interface{}
	if err := json.Unmarshal([]byte(messageStr), &message); err != nil {
		// Not valid JSON, return empty to skip
		return ""
	}
	
	// Get the content field first
	contentRaw, ok := message["content"]
	if !ok {
		return ""
	}
	
	// Handle different content formats
	switch content := contentRaw.(type) {
	case string:
		// Simple string content - this is a real user message!
		return content
		
	case []interface{}:
		// Array of content items
		var texts []string
		hasToolResult := false
		
		for _, item := range content {
			if itemMap, ok := item.(map[string]interface{}); ok {
				// Check if this is a tool result
				if _, hasToolID := itemMap["tool_use_id"]; hasToolID {
					// This is a tool result message - skip it entirely
					return ""
				}
				
				// Check for type field
				if typeStr, ok := itemMap["type"].(string); ok {
					if typeStr == "tool_result" {
						hasToolResult = true
						continue // Skip tool results
					}
					
					if typeStr == "text" {
						// This is a text message
						if text, ok := itemMap["text"].(string); ok && text != "" {
							// Skip system reminders and interruption messages
							if !strings.Contains(text, "Request interrupted by user") &&
							   !strings.Contains(text, "system-reminder") &&
							   !strings.Contains(text, "<system-reminder>") {
								texts = append(texts, text)
							}
						}
					}
				}
			}
		}
		
		// Only return if we found actual text content (not just tool results)
		if len(texts) > 0 && !hasToolResult {
			return strings.Join(texts, " ")
		}
		
		// If we only have tool results, return empty to skip this message
		if hasToolResult && len(texts) == 0 {
			return ""
		}
		
		if len(texts) > 0 {
			return strings.Join(texts, " ")
		}
	}
	
	// Return empty string to skip this message
	return ""
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