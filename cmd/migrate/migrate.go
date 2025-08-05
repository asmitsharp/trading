package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/golang-migrate/migrate/v4"
	chdriver "github.com/golang-migrate/migrate/v4/database/clickhouse"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func main() {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	var (
		direction = flag.String("dir", "up", "Migration direction: up or down")
		steps     = flag.Int("steps", 0, "Number of migrations to execute (0 = all)")
		dbType    = flag.String("db", "", "Database type: postgres or clickhouse")
		force     = flag.Int("force", 0, "Force migration version (use with caution)")
		version   = flag.Bool("version", false, "Print current migration version")
	)
	flag.Parse()

	if *dbType == "" {
		log.Fatal("Database type is required. Use -db=postgres or -db=clickhouse")
	}

	var m *migrate.Migrate
	var err error

	switch *dbType {
	case "postgres":
		m, err = setupPostgresMigration()
	case "clickhouse":
		m, err = setupClickHouseMigration()
	default:
		log.Fatalf("Unsupported database type: %s", *dbType)
	}

	if err != nil {
		log.Fatalf("Failed to setup migration: %v", err)
	}
	defer m.Close()

	// Handle force flag
	if *force > 0 {
		if err := m.Force(*force); err != nil {
			log.Fatalf("Failed to force migration version: %v", err)
		}
		log.Printf("Forced migration version to %d", *force)
		return
	}

	// Handle version flag
	if *version {
		version, dirty, err := m.Version()
		if err != nil && err != migrate.ErrNilVersion {
			log.Fatalf("Failed to get version: %v", err)
		}
		if err == migrate.ErrNilVersion {
			fmt.Println("No migrations have been applied yet")
		} else {
			fmt.Printf("Current version: %d (dirty: %v)\n", version, dirty)
		}
		return
	}

	// Execute migration
	switch *direction {
	case "up":
		if *steps > 0 {
			err = m.Steps(*steps)
		} else {
			err = m.Up()
		}
	case "down":
		if *steps > 0 {
			err = m.Steps(-*steps)
		} else {
			// Down all migrations
			err = m.Down()
		}
	default:
		log.Fatalf("Invalid direction: %s", *direction)
	}

	if err != nil {
		if err == migrate.ErrNoChange {
			log.Println("No migrations to apply")
		} else {
			log.Fatalf("Migration failed: %v", err)
		}
	} else {
		log.Printf("Migration %s completed successfully", *direction)
	}
}

func setupPostgresMigration() (*migrate.Migrate, error) {
	// Get database configuration from environment
	dbHost := getEnv("POSTGRES_HOST", "localhost")
	dbPort := getEnv("POSTGRES_PORT", "5432")
	dbUser := getEnv("POSTGRES_USER", "crypto_user")
	dbName := getEnv("POSTGRES_DB", "crypto_platform")
	dbPassword := getEnv("POSTGRES_PASSWORD", "crypto_password")

	// Build connection string
	connStr := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		dbUser, dbPassword, dbHost, dbPort, dbName)

	// Open database connection
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}

	// Create driver instance
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to create postgres driver: %w", err)
	}

	// Create migrate instance
	m, err := migrate.NewWithDatabaseInstance(
		"file://migrations/postgres",
		"postgres",
		driver,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create migrate instance: %w", err)
	}

	return m, nil
}

func setupClickHouseMigration() (*migrate.Migrate, error) {
	// Get database configuration from environment
	chHost := getEnv("CLICKHOUSE_HOST", "localhost")
	chPort := getEnv("CLICKHOUSE_PORT", "9001")
	chUser := getEnv("CLICKHOUSE_USER", "default")
	chPassword := getEnv("CLICKHOUSE_PASSWORD", "")
	chDatabase := getEnv("CLICKHOUSE_DATABASE", "crypto_platform")

	// For ClickHouse, we need to use the standard TCP port connection
	options := &clickhouse.Options{
		Addr: []string{fmt.Sprintf("%s:%s", chHost, chPort)},
		Auth: clickhouse.Auth{
			Database: chDatabase,
			Username: chUser,
			Password: chPassword,
		},
	}

	// Create a new connection with options
	chConn := clickhouse.OpenDB(options)

	// Create driver instance with database instance
	driver, err := chdriver.WithInstance(chConn, &chdriver.Config{
		DatabaseName: chDatabase,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create clickhouse driver: %w", err)
	}

	// Create migrate instance
	m, err := migrate.NewWithDatabaseInstance(
		"file://migrations/clickhouse",
		"clickhouse",
		driver,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create migrate instance: %w", err)
	}

	return m, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
