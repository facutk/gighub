package db

import (
	"database/sql"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func Setup(dataDir, dbName string) (*sql.DB, *Queries, error) {
	// Check if the data directory is writable by creating a temporary file.
	// This provides a clearer error message than the cryptic SQLite one.
	tmpFile, err := os.Create(filepath.Join(dataDir, ".writable"))
	if err != nil {
		return nil, nil, fmt.Errorf("the data directory ('%s') is not writable. Please check permissions. Original error: %w", dataDir, err)
	}
	tmpFile.Close()
	os.Remove(tmpFile.Name())

	// Initialize Database
	dbConn, err := sql.Open("sqlite", filepath.Join(dataDir, dbName))
	if err != nil {
		return nil, nil, fmt.Errorf("error opening database: %w", err)
	}

	if err := runMigrations(dbConn); err != nil {
		dbConn.Close()
		return nil, nil, err
	}

	return dbConn, New(dbConn), nil
}

func runMigrations(dbConn *sql.DB) error {
	// Initialize migration tracking
	if _, err := dbConn.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`); err != nil {
		return fmt.Errorf("error creating schema_migrations: %w", err)
	}

	var currentVersion int
	if err := dbConn.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&currentVersion); err != nil {
		return fmt.Errorf("error getting current version: %w", err)
	}

	// Run migrations
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("error reading migrations: %w", err)
	}
	for _, entry := range entries {
		parts := strings.Split(entry.Name(), "_")
		if len(parts) == 0 {
			continue
		}
		version, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}

		if version > currentVersion {
			fmt.Printf("Running migration %s...\n", entry.Name())
			content, _ := migrationsFS.ReadFile("migrations/" + entry.Name())

			tx, err := dbConn.Begin()
			if err != nil {
				return fmt.Errorf("error starting transaction: %w", err)
			}
			if _, err := tx.Exec(string(content)); err != nil {
				tx.Rollback()
				return fmt.Errorf("error running migration %s: %w", entry.Name(), err)
			}
			if _, err := tx.Exec("INSERT INTO schema_migrations (version) VALUES (?)", version); err != nil {
				tx.Rollback()
				return fmt.Errorf("error updating schema_migrations: %w", err)
			}
			if err := tx.Commit(); err != nil {
				return fmt.Errorf("error committing transaction: %w", err)
			}
		}
	}
	return nil
}
