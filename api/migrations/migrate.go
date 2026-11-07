// Package migrations applies versioned, main-service-owned SQL migrations.
package migrations

import (
	"crypto/sha256"
	"embed"
	"fmt"
	"io/fs"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

//go:embed all:postgres all:sqlite
var files embed.FS

func Apply(db *gorm.DB) error {
	return apply(db, files)
}

func apply(db *gorm.DB, files fs.FS) error {
	dialect := db.Dialector.Name()
	if dialect != "postgres" && dialect != "sqlite" {
		return fmt.Errorf("unsupported migration dialect %q", dialect)
	}
	names, err := fs.Glob(files, dialect+"/*.sql")
	if err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if dialect == "postgres" {
			if err := tx.Exec("SELECT pg_advisory_xact_lock(742790942)").Error; err != nil {
				return err
			}
		}
		if err := tx.Exec("CREATE TABLE IF NOT EXISTS main_schema_migrations (version INTEGER PRIMARY KEY, checksum TEXT NOT NULL)").Error; err != nil {
			return err
		}
		for _, name := range names {
			data, err := fs.ReadFile(files, name)
			if err != nil {
				return err
			}
			version, err := strconv.Atoi(strings.Split(strings.TrimPrefix(name, dialect+"/"), "_")[0])
			if err != nil {
				return err
			}
			checksum := fmt.Sprintf("%x", sha256.Sum256(data))
			var existing struct{ Checksum string }
			result := tx.Table("main_schema_migrations").Where("version = ?", version).Find(&existing)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected > 0 {
				if existing.Checksum != checksum {
					return fmt.Errorf("main migration %d checksum changed", version)
				}
				continue
			}
			for _, statement := range strings.Split(string(data), ";") {
				if strings.TrimSpace(statement) == "" {
					continue
				}
				if err := tx.Exec(statement).Error; err != nil {
					return fmt.Errorf("main migration: %w", err)
				}
			}
			if err := tx.Exec("INSERT INTO main_schema_migrations(version,checksum) VALUES (?,?)", version, checksum).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
