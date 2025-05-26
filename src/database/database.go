package database

import (
	"log"
	"os"
	"path/filepath"
	"project/config"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

// Init initializes the database connection.
// It uses the DSN from the application config.
// For "memory", it uses an in-memory SQLite database.
// For other DSNs, it assumes a file-based SQLite database.
func Init() (*gorm.DB, error) {
	var err error
	dsn := config.AppConfig.Database.DSN

	// GORM logger configuration
	gormLogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags), // io writer
		logger.Config{
			SlowThreshold:             logger.DefaultSlowThreshold, // Slow SQL threshold
			LogLevel:                  logger.Warn,                 // Log level (Warn, Error, Info)
			IgnoreRecordNotFoundError: true,                        // Ignore ErrRecordNotFound error for logger
			Colorful:                  true,                        // Disable color
		},
	)

	gormConfig := &gorm.Config{
		Logger: gormLogger,
	}

	if dsn == "memory" || dsn == "" { // Treat empty DSN as in-memory for safety
		log.Println("INFO: [Database] Initializing in-memory SQLite database (DSN: 'memory' or empty).")
		// For shared in-memory DB across connections (if needed, though typically not for a single app instance like this):
		// Use "file::memory:?cache=shared"
		DB, err = gorm.Open(sqlite.Open("file::memory:?cache=shared"), gormConfig)
	} else {
		log.Printf("INFO: [Database] Initializing file-based SQLite database at DSN: '%s'.", dsn)
		// Ensure the directory for the SQLite file exists.
		dbDir := filepath.Dir(dsn)
		if dbDir != "." && dbDir != "/" { // Avoid trying to create "." or root
			if _, statErr := os.Stat(dbDir); os.IsNotExist(statErr) {
				log.Printf("INFO: [Database] Database directory '%s' does not exist, attempting to create.", dbDir)
				if mkdirErr := os.MkdirAll(dbDir, 0755); mkdirErr != nil {
					log.Printf("ERROR: [Database] Failed to create database directory '%s': %v", dbDir, mkdirErr)
					return nil, fmt.Errorf("failed to create database directory '%s': %w", dbDir, mkdirErr)
				}
				log.Printf("INFO: [Database] Successfully created database directory '%s'.", dbDir)
			}
		}
		DB, err = gorm.Open(sqlite.Open(dsn), gormConfig)
	}

	if err != nil {
		log.Printf("ERROR: [Database] Failed to connect to database (DSN: '%s'): %v", dsn, err)
		return nil, fmt.Errorf("failed to connect to database (DSN: '%s'): %w", dsn, err)
	}

	log.Println("INFO: [Database] Database connection established successfully.")
	return DB, nil
}

// GetDB returns the global database instance.
// It panics if DB has not been initialized via Init().
func GetDB() *gorm.DB {
	if DB == nil {
		log.Fatal("FATAL: [Database] Database instance has not been initialized. Call database.Init() first.")
	}
	return DB
}
