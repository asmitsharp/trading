package main

import (
	"database/sql"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func main() {
	// Load .env file
	godotenv.Load()

	// Parse command line flags
	var (
		migrationsPath = flag.String("path", "migrations", "Path to migrations directory")
		direction      = flag.String("dir", "up", "Migration direction: up or down")
		verbose        = flag.Bool("v", false, "Verbose output")
	)
	flag.Parse()

	// Database connection
	dbURL := getDBURL()
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Test connection
	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}

	fmt.Println("Connected to database successfully")

	// Create migrations table if it doesn't exist
	if err := createMigrationsTable(db); err != nil {
		log.Fatalf("Failed to create migrations table: %v", err)
	}

	// Get migration files
	files, err := getMigrationFiles(*migrationsPath, *direction)
	if err != nil {
		log.Fatalf("Failed to get migration files: %v", err)
	}

	if len(files) == 0 {
		fmt.Println("No migration files found")
		return
	}

	fmt.Printf("Found %d migration files\n", len(files))

	// Run migrations
	for _, file := range files {
		if *direction == "up" {
			if err := runMigrationUp(db, file, *verbose); err != nil {
				log.Fatalf("Failed to run migration %s: %v", file, err)
			}
		} else {
			if err := runMigrationDown(db, file, *verbose); err != nil {
				log.Fatalf("Failed to rollback migration %s: %v", file, err)
			}
		}
	}

	fmt.Println("All migrations completed successfully")
}

func getDBURL() string {
	// Build database URL from environment variables
	host := getEnv("POSTGRES_HOST", "localhost")
	port := getEnv("POSTGRES_PORT", "5432")
	user := getEnv("POSTGRES_USERNAME", "crypto_user")
	password := getEnv("POSTGRES_PASSWORD", "crypto_password")
	dbname := getEnv("POSTGRES_DATABASE", "crypto_platform")
	sslmode := getEnv("POSTGRES_SSLMODE", "disable")

	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		host, port, user, password, dbname, sslmode)
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func createMigrationsTable(db *sql.DB) error {
	query := `
	CREATE TABLE IF NOT EXISTS schema_migrations (
		version VARCHAR(255) PRIMARY KEY,
		applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`
	_, err := db.Exec(query)
	return err
}

func getMigrationFiles(path, direction string) ([]string, error) {
	var files []string
	
	// Read all SQL files
	entries, err := ioutil.ReadDir(path)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		// Skip non-SQL files
		if !strings.HasSuffix(name, ".sql") {
			continue
		}

		// Skip seed files
		if strings.Contains(name, "seed") {
			continue
		}

		// Skip ClickHouse migrations
		if strings.Contains(name, "clickhouse") {
			continue
		}

		// For up migrations, skip down files
		if direction == "up" && strings.Contains(name, ".down.sql") {
			continue
		}

		// For down migrations, only include down files
		if direction == "down" && !strings.Contains(name, ".down.sql") {
			continue
		}

		files = append(files, filepath.Join(path, name))
	}

	// Sort files to ensure consistent order
	sort.Strings(files)
	return files, nil
}

func runMigrationUp(db *sql.DB, file string, verbose bool) error {
	// Check if migration has already been applied
	version := filepath.Base(file)
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = $1", version).Scan(&count)
	if err != nil {
		return err
	}

	if count > 0 {
		if verbose {
			fmt.Printf("Skipping %s (already applied)\n", version)
		}
		return nil
	}

	// Read migration file
	content, err := ioutil.ReadFile(file)
	if err != nil {
		return err
	}

	// Begin transaction
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Execute migration
	if verbose {
		fmt.Printf("Running migration: %s\n", version)
	}

	if _, err := tx.Exec(string(content)); err != nil {
		return fmt.Errorf("failed to execute migration: %w", err)
	}

	// Record migration
	if _, err := tx.Exec("INSERT INTO schema_migrations (version) VALUES ($1)", version); err != nil {
		return fmt.Errorf("failed to record migration: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return err
	}

	fmt.Printf("✓ Applied migration: %s\n", version)
	return nil
}

func runMigrationDown(db *sql.DB, file string, verbose bool) error {
	// For rollback, we would need to implement the reverse logic
	// This is a simplified version
	version := strings.Replace(filepath.Base(file), ".down.sql", ".up.sql", 1)
	
	// Check if migration exists
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = $1", version).Scan(&count)
	if err != nil {
		return err
	}

	if count == 0 {
		if verbose {
			fmt.Printf("Skipping %s (not applied)\n", version)
		}
		return nil
	}

	// Read migration file
	content, err := ioutil.ReadFile(file)
	if err != nil {
		return err
	}

	// Begin transaction
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Execute rollback
	if verbose {
		fmt.Printf("Rolling back: %s\n", version)
	}

	if _, err := tx.Exec(string(content)); err != nil {
		return fmt.Errorf("failed to execute rollback: %w", err)
	}

	// Remove migration record
	if _, err := tx.Exec("DELETE FROM schema_migrations WHERE version = $1", version); err != nil {
		return fmt.Errorf("failed to remove migration record: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return err
	}

	fmt.Printf("✓ Rolled back migration: %s\n", version)
	return nil
}