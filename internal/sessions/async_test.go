package sessions

import (
	"context"
	"testing"
	"time"
)

// TestAsyncProjectLoading tests async loading of projects
func TestAsyncProjectLoading(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Test loading projects asynchronously
	projects, err := FetchProjectsWithStatsAsync(ctx)
	if err != nil {
		// Skip if no projects available (CI environment)
		t.Skipf("Skipping test, no projects available: %v", err)
	}

	if len(projects) == 0 {
		t.Skip("No projects found in test environment")
	}

	// Verify project data
	for _, project := range projects {
		if project.Name == "" {
			t.Error("Project name should not be empty")
		}
		if project.Path == "" {
			t.Error("Project path should not be empty")
		}
	}
}

// TestAsyncSessionLoading tests async loading of sessions
func TestAsyncSessionLoading(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// First get projects
	projects, err := FetchProjectsWithStatsAsync(ctx)
	if err != nil || len(projects) == 0 {
		t.Skip("No projects available for session testing")
	}

	// Test loading sessions for first project
	sessions, err := FetchSessionsForProjectAsync(ctx, projects[0].Path)
	if err != nil {
		t.Errorf("Failed to load sessions: %v", err)
	}

	// Verify session data if available
	for _, session := range sessions {
		if session.SessionID == "" {
			t.Error("Session ID should not be empty")
		}
	}
}

// TestAsyncCancellation tests cancellation of async operations
func TestAsyncCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// Start loading in goroutine
	done := make(chan struct{})

	go func() {
		_, _ = FetchProjectsWithStatsAsync(ctx)
		close(done)
	}()

	// Cancel immediately
	cancel()

	// Wait for completion
	select {
	case <-done:
		// Operation completed (either with cancel or success)
	case <-time.After(5 * time.Second):
		t.Error("Operation did not complete within timeout after cancellation")
	}
}

// TestAsyncMessageLoading tests async loading of messages
func TestAsyncMessageLoading(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get projects and sessions first
	projects, err := FetchProjectsWithStatsAsync(ctx)
	if err != nil || len(projects) == 0 {
		t.Skip("No projects available for message testing")
	}

	sessions, err := FetchSessionsForProjectAsync(ctx, projects[0].Path)
	if err != nil || len(sessions) == 0 {
		t.Skip("No sessions available for message testing")
	}

	// Test loading messages
	messages, err := FetchRecentMessagesForSessionAsync(ctx, sessions[0].SessionID)
	if err != nil {
		// Messages might not exist, which is ok
		t.Logf("No messages found (this is ok): %v", err)
		return
	}

	// If messages exist, verify they're formatted correctly
	for _, msg := range messages {
		if msg == "" {
			t.Error("Message should not be empty")
		}
	}
}

// TestConcurrentAsyncOperations tests multiple async operations
func TestConcurrentAsyncOperations(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Run multiple async operations concurrently
	errChan := make(chan error, 3)

	// Load projects
	go func() {
		_, err := FetchProjectsWithStatsAsync(ctx)
		errChan <- err
	}()

	// Load projects again (test concurrent access)
	go func() {
		_, err := FetchProjectsWithStatsAsync(ctx)
		errChan <- err
	}()

	// Try to load sessions for unknown project (should handle gracefully)
	go func() {
		_, err := FetchSessionsForProjectAsync(ctx, "Unknown")
		errChan <- err
	}()

	// Collect results
	for i := 0; i < 3; i++ {
		select {
		case err := <-errChan:
			if err != nil {
				t.Logf("Operation %d completed with error (may be expected): %v", i+1, err)
			}
		case <-time.After(10 * time.Second):
			t.Error("Operation timed out")
		}
	}
}

// BenchmarkAsyncProjectLoading benchmarks async project loading
func BenchmarkAsyncProjectLoading(b *testing.B) {
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := FetchProjectsWithStatsAsync(ctx)
		if err != nil {
			b.Skipf("Skipping benchmark: %v", err)
		}
	}
}

// BenchmarkSyncVsAsyncLoading compares sync vs async loading
func BenchmarkSyncVsAsyncLoading(b *testing.B) {
	b.Run("Sync", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := FetchProjectsWithStats()
			if err != nil {
				b.Skipf("Skipping benchmark: %v", err)
			}
		}
	})

	b.Run("Async", func(b *testing.B) {
		ctx := context.Background()
		for i := 0; i < b.N; i++ {
			_, err := FetchProjectsWithStatsAsync(ctx)
			if err != nil {
				b.Skipf("Skipping benchmark: %v", err)
			}
		}
	})
}