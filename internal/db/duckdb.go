package db

import (
	"database/sql"
	"fmt"
	"sync"

	_ "github.com/marcboeker/go-duckdb"
)

var (
	dbInstance *sql.DB
	dbOnce     sync.Once
	dbErr      error
)

// GetDB returns a singleton DuckDB connection
func GetDB() (*sql.DB, error) {
	dbOnce.Do(func() {
		dbInstance, dbErr = initializeDuckDB()
	})
	return dbInstance, dbErr
}

// initializeDuckDB initializes a DuckDB connection with JSON extension
func initializeDuckDB() (*sql.DB, error) {
	db, err := sql.Open("duckdb", "")
	if err != nil {
		return nil, fmt.Errorf("failed to open DuckDB: %w", err)
	}

	// Set connection pool settings for better performance
	db.SetMaxOpenConns(1) // DuckDB works best with single connection
	db.SetMaxIdleConns(1)

	// Install and load the JSON extension
	if _, err := db.Exec("INSTALL json"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to install JSON extension: %w", err)
	}

	if _, err := db.Exec("LOAD json"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to load JSON extension: %w", err)
	}

	return db, nil
}

// InitializeDuckDB is kept for backward compatibility but uses singleton
func InitializeDuckDB() (*sql.DB, error) {
	return GetDB()
}