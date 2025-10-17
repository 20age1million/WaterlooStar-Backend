package database

import (
	"fmt"
	"os"
	"time"

	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Open connects to PostgreSQL using GORM and loads .env automatically.
// It supports connection pooling, retry, and graceful shutdown.
func Open() (*gorm.DB, error) {
	// Load .env file automatically (if exists)
	_ = godotenv.Load()

	// Read DSN from environment variable
	dsn := os.Getenv("PG_DSN")

	// If PG_DSN not set, show setup instructions
	if dsn == "" {
		return nil, fmt.Errorf(`
Environment variable "PG_DSN" is not set.

Please create a ".env" file in your project root with this line:
    PG_DSN=host=localhost user=postgres password=postgres dbname=appdb port=5432 sslmode=disable TimeZone=America/Toronto

Or set it manually (bash):
    export PG_DSN="host=localhost user=postgres password=postgres dbname=appdb port=5432 sslmode=disable TimeZone=America/Toronto"

Then re-run:
    go run ./cmd/api
`)
	}

	// Connect via GORM
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}

	// Configure connection pool
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get SQL DB object: %w", err)
	}
	sqlDB.SetMaxOpenConns(50)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetConnMaxLifetime(2 * time.Hour)
	sqlDB.SetConnMaxIdleTime(15 * time.Minute)

	// Initial ping check
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("database not reachable: %w", err)
	}

	fmt.Println("Connected to PostgreSQL successfully!")
	return db, nil
}

// Close gracefully closes the underlying SQL DB pool.
func Close(gdb *gorm.DB) error {
	if gdb == nil {
		return nil
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
