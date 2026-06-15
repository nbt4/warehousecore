// Package repository provides PostgreSQL database connection for WarehouseCore
package repository

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"warehousecore/config"
)

// Common errors
var (
	ErrNotFound = errors.New("not found")
)

// DB holds the database connection pool (sql.DB)
var DB *sql.DB

// GormDB holds the GORM database connection for auth and models
var GormDB *gorm.DB

// InitDatabase initializes the PostgreSQL database connection
func InitDatabase(cfg *config.Config) error {
	// Build PostgreSQL DSN
	dsn := buildPostgresDSN(cfg)

	// Open sql.DB for direct SQL queries
	sqlDB, err := sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	// PostgreSQL connection pool settings
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetConnMaxLifetime(time.Hour)
	sqlDB.SetConnMaxIdleTime(30 * time.Minute)

	// Test connection
	if err := sqlDB.Ping(); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	DB = sqlDB
	log.Printf("PostgreSQL database connection established: %s@%s:%s/%s",
		cfg.Database.User, cfg.Database.Host, cfg.Database.Port, cfg.Database.Name)

	// Initialize GORM with PostgreSQL driver
	gormDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		SkipDefaultTransaction: false,
		PrepareStmt:            true,
		CreateBatchSize:        100,
		Logger:                 logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return fmt.Errorf("failed to initialize GORM: %w", err)
	}

	GormDB = gormDB
	log.Println("GORM PostgreSQL connection established successfully")

	return nil
}

// buildPostgresDSN creates the PostgreSQL connection string
func buildPostgresDSN(cfg *config.Config) string {
	host := cfg.Database.Host
	if host == "" {
		host = "localhost"
	}

	port := cfg.Database.Port
	if port == "" {
		port = "5432"
	}

	dbName := cfg.Database.Name
	if dbName == "" {
		dbName = "rentalcore"
	}

	user := cfg.Database.User
	if user == "" {
		user = "rentalcore"
	}

	password := cfg.Database.Password
	if password == "" {
		log.Fatal("DB_PASS environment variable is required but not set") // FIXED: removed hardcoded password
	}

	sslMode := cfg.Database.SSLMode
	if sslMode == "" {
		sslMode = "disable"
	}

	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		host, port, user, password, dbName, sslMode,
	)
}

// CloseDatabase closes the database connection properly
func CloseDatabase() error {
	if DB != nil {
		return DB.Close()
	}
	return nil
}

// GetDB returns the GORM database connection
func GetDB() *gorm.DB {
	return GormDB
}

// GetSQLDB returns the raw SQL database connection
func GetSQLDB() *sql.DB {
	return DB
}

// HashAPIKey creates a stable SHA-256 hex hash of an API key.
func HashAPIKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}
