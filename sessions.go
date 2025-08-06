package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/marcboeker/go-duckdb"
)

type Session struct {
	SessionID       string
	ProjectPath     string
	ProjectName     string
	LastActivity    time.Time
	RecentMessages  []string // Last 5 user messages
}

type Project struct {
	Path         string
	Name         string
	SessionCount int
	LastActivity time.Time
	Sessions     []Session
}


func initializeDuckDB() (*sql.DB, error) {
	db, err := sql.Open("duckdb", "")
	if err != nil {
		return nil, fmt.Errorf("failed to open DuckDB: %w", err)
	}

	// Install and load json extension
	_, err = db.Exec("INSTALL json")
	if err != nil {
		return nil, fmt.Errorf("failed to install json extension: %w", err)
	}
	_, err = db.Exec("LOAD json")
	if err != nil {
		return nil, fmt.Errorf("failed to load json extension: %w", err)
	}

	return db, nil
}

func fetchProjectsWithStats() ([]Project, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	claudeDir := filepath.Join(homeDir, ".claude", "projects")
	globPattern := filepath.Join(claudeDir, "**", "*.jsonl")

	db, err := initializeDuckDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	// Query to aggregate by project
	projectQuery := fmt.Sprintf(`
		WITH all_events AS (
			SELECT 
				CAST(sessionId AS VARCHAR) as sessionId,
				COALESCE(cwd, 'Unknown') as project_path,
				timestamp,
				type
			FROM read_json('%s',
				format = 'newline_delimited',
				union_by_name = true,
				filename = true
			)
			WHERE sessionId IS NOT NULL
		),
		project_stats AS (
			SELECT 
				project_path,
				COUNT(DISTINCT sessionId) as session_count,
				MAX(timestamp) as last_activity,
				COUNT(*) as total_events
			FROM all_events
			GROUP BY project_path
		)
		SELECT 
			project_path,
			session_count,
			last_activity,
			total_events
		FROM project_stats
		ORDER BY last_activity DESC
		LIMIT 100
	`, globPattern)

	rows, err := db.Query(projectQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to execute project query: %w", err)
	}
	defer rows.Close()

	var projects []Project
	for rows.Next() {
		var project Project
		var sessionCount int
		var totalEvents int
		var timestampStr string

		err := rows.Scan(&project.Path, &sessionCount, &timestampStr, &totalEvents)
		if err != nil {
			return nil, fmt.Errorf("failed to scan project row: %w", err)
		}

		project.Name = filepath.Base(project.Path)
		if project.Name == "" || project.Name == "." {
			project.Name = "Unknown Project"
		}
		
		project.SessionCount = sessionCount
		project.LastActivity, _ = time.Parse(time.RFC3339, timestampStr)
		// Sessions will be loaded on demand
		project.Sessions = nil

		projects = append(projects, project)
	}

	return projects, nil
}

func fetchSessionsForProject(projectPath string) ([]Session, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	claudeDir := filepath.Join(homeDir, ".claude", "projects")
	globPattern := filepath.Join(claudeDir, "**", "*.jsonl")

	db, err := initializeDuckDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	// Query to get sessions for a specific project
	sessionQuery := fmt.Sprintf(`
		WITH all_events AS (
			SELECT 
				CAST(sessionId AS VARCHAR) as sessionId,
				COALESCE(cwd, 'Unknown') as project_path,
				timestamp,
				type,
				message
			FROM read_json('%s',
				format = 'newline_delimited',
				union_by_name = true,
				filename = true
			)
			WHERE sessionId IS NOT NULL
			AND COALESCE(cwd, 'Unknown') = ?
		),
		session_stats AS (
			SELECT 
				sessionId,
				MAX(timestamp) as last_activity,
				COUNT(*) as event_count,
				MIN(timestamp) as first_activity
			FROM all_events
			GROUP BY sessionId
		)
		SELECT 
			ss.sessionId,
			? as project_path,
			ss.last_activity,
			ss.first_activity,
			ss.event_count
		FROM session_stats ss
		ORDER BY ss.last_activity DESC
		LIMIT 100
	`, globPattern)

	rows, err := db.Query(sessionQuery, projectPath, projectPath)
	if err != nil {
		return nil, fmt.Errorf("failed to execute session query: %w", err)
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var session Session
		var timestampStr string
		var firstActivityStr string
		var eventCount int

		err := rows.Scan(&session.SessionID, &session.ProjectPath, &timestampStr, &firstActivityStr, &eventCount)
		if err != nil {
			return nil, fmt.Errorf("failed to scan session row: %w", err)
		}

		session.LastActivity, _ = time.Parse(time.RFC3339, timestampStr)
		session.ProjectName = filepath.Base(projectPath)

		sessions = append(sessions, session)
	}

	return sessions, nil
}


func fetchRecentMessagesForSession(sessionID string) ([]string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	claudeDir := filepath.Join(homeDir, ".claude", "projects")
	globPattern := filepath.Join(claudeDir, "**", "*.jsonl")

	db, err := initializeDuckDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	// Query to get last 5 user TEXT messages (not tool results) for a specific session
	// We need to filter at the SQL level to avoid getting only tool results
	messagesQuery := fmt.Sprintf(`
		WITH all_user_messages AS (
			SELECT 
				to_json(message) as message_json,
				timestamp,
				type
			FROM read_json('%s',
				format = 'newline_delimited',
				union_by_name = true,
				filename = true
			)
			WHERE CAST(sessionId AS VARCHAR) = ?
			AND type = 'user'
			AND message IS NOT NULL
			ORDER BY timestamp DESC
			LIMIT 50
		)
		SELECT message_json
		FROM all_user_messages
		ORDER BY timestamp ASC
	`, globPattern)

	rows, err := db.Query(messagesQuery, sessionID)
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
			// Extract content from the message JSON
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

	// Reverse the messages to show oldest first (since we queried DESC but want to display ASC)
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages, nil
}

// Helper function to extract content from Claude message format
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

func executeClaudeResume(sessionID string, projectPath string) error {
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

func debugSessionMessages(sessionID string) ([]string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	claudeDir := filepath.Join(homeDir, ".claude", "projects")
	globPattern := filepath.Join(claudeDir, "**", "*.jsonl")

	db, err := initializeDuckDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()

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

	rows, err := db.Query(textQuery, sessionID)
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