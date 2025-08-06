package sessions

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/strrl/claude-resume/pkg/models"
)

// LoadingState represents the state of an async operation
type LoadingState int

const (
	StateIdle LoadingState = iota
	StateLoadingProjects
	StateLoadingSessions
	StateLoadingMessages
	StateCancelling
	StateError
)

// SQLRequest represents a SQL query request
type SQLRequest struct {
	Query     string
	Args      []interface{}
	RequestID string
	Type      LoadingState
	Context   context.Context
}

// SQLResult represents the result of a SQL query
type SQLResult struct {
	Data      interface{}
	Error     error
	RequestID string
	Type      LoadingState
}

// SQLProgress represents progress updates for long-running queries
type SQLProgress struct {
	RequestID string
	Progress  float64
	Message   string
}

// AsyncExecutor manages async SQL execution
type AsyncExecutor struct {
	db        *sql.DB
	requests  chan SQLRequest
	mu        sync.RWMutex
	contexts  map[string]context.CancelFunc
	closed    bool
	closeOnce sync.Once
}

// NewAsyncExecutor creates a new async executor
func NewAsyncExecutor(db *sql.DB) *AsyncExecutor {
	return &AsyncExecutor{
		db:       db,
		requests: make(chan SQLRequest, 10),
		contexts: make(map[string]context.CancelFunc),
	}
}

// Start begins processing SQL requests
func (e *AsyncExecutor) Start() {
	go e.processRequests()
}

// Close shuts down the executor
func (e *AsyncExecutor) Close() {
	e.closeOnce.Do(func() {
		e.mu.Lock()
		e.closed = true
		close(e.requests)
		// Cancel all active requests
		for _, cancel := range e.contexts {
			cancel()
		}
		e.mu.Unlock()
	})
}

// processRequests handles incoming SQL requests
func (e *AsyncExecutor) processRequests() {
	for req := range e.requests {
		e.handleRequest(req)
	}
}

// handleRequest processes a single SQL request
func (e *AsyncExecutor) handleRequest(req SQLRequest) {
	// Store cancel function
	ctx, cancel := context.WithCancel(req.Context)
	e.mu.Lock()
	e.contexts[req.RequestID] = cancel
	e.mu.Unlock()

	// Clean up when done
	defer func() {
		e.mu.Lock()
		delete(e.contexts, req.RequestID)
		e.mu.Unlock()
	}()

	// Execute query with context
	rows, err := e.db.QueryContext(ctx, req.Query, req.Args...)
	if err != nil {
		// Check if cancelled
		if ctx.Err() == context.Canceled {
			return // Don't send error for cancelled queries
		}
		// Handle other errors through the result channel
		return
	}
	defer rows.Close()

	// Process results based on query type
	switch req.Type {
	case StateLoadingProjects:
		// Process project results
		for rows.Next() {
			// Check for cancellation
			if ctx.Err() == context.Canceled {
				return
			}
			// Process row (implementation depends on actual query)
			// Results would be sent through a channel in a full implementation
		}
	case StateLoadingSessions:
		// Process session results
		for rows.Next() {
			// Check for cancellation
			if ctx.Err() == context.Canceled {
				return
			}
			// Process row
			// Results would be sent through a channel in a full implementation
		}
	case StateLoadingMessages:
		// Process message results
		for rows.Next() {
			// Check for cancellation
			if ctx.Err() == context.Canceled {
				return
			}
			// Process row
			// Results would be sent through a channel in a full implementation
		}
	}
}

// Submit submits a new SQL request
func (e *AsyncExecutor) Submit(ctx context.Context, query string, args []interface{}, queryType LoadingState) string {
	e.mu.RLock()
	if e.closed {
		e.mu.RUnlock()
		return ""
	}
	e.mu.RUnlock()

	requestID := uuid.New().String()
	req := SQLRequest{
		Query:     query,
		Args:      args,
		RequestID: requestID,
		Type:      queryType,
		Context:   ctx,
	}

	select {
	case e.requests <- req:
		return requestID
	case <-ctx.Done():
		return ""
	}
}

// Cancel cancels a specific request
func (e *AsyncExecutor) Cancel(requestID string) {
	e.mu.RLock()
	cancel, ok := e.contexts[requestID]
	e.mu.RUnlock()

	if ok {
		cancel()
	}
}

// CancelAll cancels all active requests
func (e *AsyncExecutor) CancelAll() {
	e.mu.RLock()
	cancels := make([]context.CancelFunc, 0, len(e.contexts))
	for _, cancel := range e.contexts {
		cancels = append(cancels, cancel)
	}
	e.mu.RUnlock()

	for _, cancel := range cancels {
		cancel()
	}
}

// AsyncQueryResult wraps query results with metadata
type AsyncQueryResult struct {
	Projects []models.Project
	Sessions []models.Session
	Messages []string
	Error    error
}

// ExecuteProjectsQueryAsync executes a projects query asynchronously
func ExecuteProjectsQueryAsync(ctx context.Context, db *sql.DB, query string, args ...interface{}) <-chan AsyncQueryResult {
	resultChan := make(chan AsyncQueryResult, 1)

	go func() {
		defer close(resultChan)

		// Add timeout to prevent hanging
		queryCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		rows, err := db.QueryContext(queryCtx, query, args...)
		if err != nil {
			select {
			case resultChan <- AsyncQueryResult{Error: err}:
			case <-ctx.Done():
			}
			return
		}
		defer rows.Close()

		var projects []models.Project
		for rows.Next() {
			// Check for cancellation
			select {
			case <-ctx.Done():
				return
			default:
			}

			var project models.Project
			var lastActivity sql.NullString

			if err := rows.Scan(&project.Path, &project.SessionCount, &lastActivity); err != nil {
				continue
			}

			// Process project (same logic as before)
			if project.Path == "Unknown" || project.Path == "" {
				project.Name = "Unknown"
			} else {
				project.Name = filepath.Base(project.Path)
			}

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

		select {
		case resultChan <- AsyncQueryResult{Projects: projects}:
		case <-ctx.Done():
		}
	}()

	return resultChan
}

// ExecuteSessionsQueryAsync executes a sessions query asynchronously
func ExecuteSessionsQueryAsync(ctx context.Context, db *sql.DB, query string, args ...interface{}) <-chan AsyncQueryResult {
	resultChan := make(chan AsyncQueryResult, 1)

	go func() {
		defer close(resultChan)

		// Add timeout
		queryCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		rows, err := db.QueryContext(queryCtx, query, args...)
		if err != nil {
			select {
			case resultChan <- AsyncQueryResult{Error: err}:
			case <-ctx.Done():
			}
			return
		}
		defer rows.Close()

		var sessions []models.Session
		for rows.Next() {
			// Check for cancellation
			select {
			case <-ctx.Done():
				return
			default:
			}

			var session models.Session
			var lastActivity sql.NullString
			var isResumed bool

			if err := rows.Scan(&session.SessionID, &lastActivity, &isResumed); err != nil {
				continue
			}

			session.IsResumed = isResumed

			// Parse timestamp
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
		}

		select {
		case resultChan <- AsyncQueryResult{Sessions: sessions}:
		case <-ctx.Done():
		}
	}()

	return resultChan
}

// ExecuteMessagesQueryAsync executes a messages query asynchronously
func ExecuteMessagesQueryAsync(ctx context.Context, db *sql.DB, query string, sessionID string) <-chan AsyncQueryResult {
	resultChan := make(chan AsyncQueryResult, 1)

	go func() {
		defer close(resultChan)

		// Add timeout
		queryCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()

		rows, err := db.QueryContext(queryCtx, query, sessionID)
		if err != nil {
			select {
			case resultChan <- AsyncQueryResult{Error: fmt.Errorf("failed to execute messages query: %w", err)}:
			case <-ctx.Done():
			}
			return
		}
		defer rows.Close()

		var messages []string
		var firstMessages []string
		var lastMessages []string
		var totalCount int64
		lastPosition := ""

		for rows.Next() {
			// Check for cancellation
			select {
			case <-ctx.Done():
				return
			default:
			}

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
				formattedMsg := formatMessageWithRole(messageType.String, messageJSON.String)
				if formattedMsg != "" {
					if position.String == "first" {
						firstMessages = append(firstMessages, formattedMsg)
						lastPosition = "first"
					} else if position.String == "last" {
						if lastPosition == "first" && len(lastMessages) == 0 {
							if totalCount > 20 {
								messages = append(messages, firstMessages...)
								messages = append(messages, fmt.Sprintf("... (%d messages omitted) ...", totalCount-20))
								lastMessages = append(lastMessages, formattedMsg)
							} else {
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

		// Combine messages
		if len(lastMessages) > 0 {
			messages = append(messages, lastMessages...)
		} else {
			messages = firstMessages
		}

		select {
		case resultChan <- AsyncQueryResult{Messages: messages}:
		case <-ctx.Done():
		}
	}()

	return resultChan
}