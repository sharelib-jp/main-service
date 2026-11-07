// Package database opens the main service's SQL database.
package database

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Open opens a SQLite database. For concurrent writes, set _busy_timeout and
// _txlock=immediate in the DSN; enable _foreign_keys when using foreign keys.
func Open(dsn string) (*gorm.DB, error) {
	return OpenWithDriver("sqlite", dsn)
}

// OpenWithDriver opens SQLite or PostgreSQL. An empty driver selects SQLite.
// Opening a connection never changes the schema. main applies repository-owned
// versioned migrations explicitly after configuration and authentication setup.
func OpenWithDriver(driver, dsn string) (*gorm.DB, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, fmt.Errorf("database DSN must not be empty")
	}
	var dialector gorm.Dialector
	switch driver {
	case "", "sqlite":
		dialector = sqlite.Open(dsn)
	case "postgres":
		dialector = postgres.Open(dsn)
	default:
		return nil, fmt.Errorf("unsupported database driver %q", driver)
	}
	gormLogger := logger.New(log.New(os.Stderr, "", log.LstdFlags), logger.Config{
		SlowThreshold: time.Second, LogLevel: logger.Warn, ParameterizedQueries: true,
	})
	db, err := gorm.Open(dialector, &gorm.Config{TranslateError: true, Logger: gormLogger})
	if err != nil {
		// A dialect may have created a pool before its initial connection failed.
		if db != nil {
			if pool, poolErr := db.DB(); poolErr == nil {
				_ = pool.Close()
			}
		}
		return nil, fmt.Errorf("open database: %w", err)
	}
	pool, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get database connection pool: %w", err)
	}
	if db.Dialector.Name() == "sqlite" {
		pool.SetMaxOpenConns(1)
	} else {
		pool.SetMaxOpenConns(10)
	}
	return db, nil
}
