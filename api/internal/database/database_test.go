package database

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"
)

func sqliteDSN(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "repository.db") + "?_busy_timeout=5000&_foreign_keys=on&_txlock=immediate"
}

func openTestDB(t *testing.T, driver, dsn string) *gorm.DB {
	t.Helper()
	db, err := OpenWithDriver(driver, dsn)
	if err != nil {
		t.Fatal(err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pool.Close() })
	return db
}

func TestSQLitePersistsExistingData(t *testing.T) {
	dsn := sqliteDSN(t)
	original := openTestDB(t, "sqlite", dsn)
	if err := original.Exec("CREATE TABLE existing_records (id INTEGER PRIMARY KEY, value TEXT NOT NULL)").Error; err != nil {
		t.Fatal(err)
	}
	if err := original.Exec("INSERT INTO existing_records (id, value) VALUES (?, ?)", 1, "keep this data").Error; err != nil {
		t.Fatal(err)
	}
	reopened := openTestDB(t, "", dsn)
	var value string
	if err := reopened.Raw("SELECT value FROM existing_records WHERE id = ?", 1).Scan(&value).Error; err != nil || value != "keep this data" {
		t.Fatalf("existing data changed: %q, %v", value, err)
	}
	if reopened.Migrator().HasTable("users") || reopened.Migrator().HasTable("oauth_clients") {
		t.Fatal("opening the repository database created authentication tables")
	}
	pool, _ := reopened.DB()
	if got := pool.Stats().MaxOpenConnections; got != 1 {
		t.Fatalf("SQLite max open connections = %d, want 1", got)
	}
}

func TestSQLiteConcurrentTransactionsAndRollback(t *testing.T) {
	dsn := sqliteDSN(t)
	db, err := Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	pool, _ := db.DB()
	t.Cleanup(func() { _ = pool.Close() })
	if err := db.Exec("CREATE TABLE records (id INTEGER PRIMARY KEY, value TEXT NOT NULL)").Error; err != nil {
		t.Fatal(err)
	}
	otherDB := openTestDB(t, "sqlite", dsn)
	connections := []*gorm.DB{db, otherDB}
	const count = 12
	var group sync.WaitGroup
	for i := 0; i < count; i++ {
		group.Add(1)
		go func(n int) {
			defer group.Done()
			if err := connections[n%len(connections)].WithContext(context.Background()).Transaction(func(tx *gorm.DB) error {
				return tx.Exec("INSERT INTO records (id, value) VALUES (?, ?)", n+1, fmt.Sprint(n)).Error
			}); err != nil {
				t.Errorf("transaction %d: %v", n, err)
			}
		}(i)
	}
	group.Wait()
	failure := errors.New("rollback requested")
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("INSERT INTO records (id, value) VALUES (?, ?)", count+1, "rollback").Error; err != nil {
			return err
		}
		return failure
	}); !errors.Is(err, failure) {
		t.Fatalf("rollback error = %v", err)
	}
	var got int64
	if err := db.Table("records").Count(&got).Error; err != nil || got != count {
		t.Fatalf("records = %d, error = %v", got, err)
	}
}

func TestSQLiteConstraints(t *testing.T) {
	db := openTestDB(t, "sqlite", sqliteDSN(t))
	if err := db.Exec("CREATE TABLE parent (id INTEGER PRIMARY KEY)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE child (id INTEGER PRIMARY KEY, parent_id INTEGER NOT NULL REFERENCES parent(id), name TEXT NOT NULL UNIQUE)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO child (id, parent_id, name) VALUES (1, 123, 'invalid')").Error; err == nil {
		t.Fatal("foreign keys are not enforced")
	}
	if err := db.Exec("INSERT INTO parent (id) VALUES (1)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO child (id, parent_id, name) VALUES (1, 1, 'same')").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO child (id, parent_id, name) VALUES (2, 1, 'same')").Error; !errors.Is(err, gorm.ErrDuplicatedKey) {
		t.Fatalf("duplicate error = %v", err)
	}
}

func TestInvalidDatabaseConfiguration(t *testing.T) {
	for _, tc := range []struct{ driver, dsn string }{
		{"sqlite", ""}, {"postgres", "   "}, {"unknown", "test.db"},
		{"sqlite", filepath.Join(t.TempDir(), "missing", "repository.db")},
	} {
		if db, err := OpenWithDriver(tc.driver, tc.dsn); err == nil {
			if pool, poolErr := db.DB(); poolErr == nil {
				_ = pool.Close()
			}
			t.Errorf("accepted driver %q with invalid DSN", tc.driver)
		}
	}
}

func TestInvalidPostgresDSNDoesNotExposePassword(t *testing.T) {
	const password = "test-password-that-should-not-appear"
	db, err := OpenWithDriver("postgres", "postgres://user:"+password+"@localhost:notaport/repository")
	if err == nil {
		if pool, poolErr := db.DB(); poolErr == nil {
			_ = pool.Close()
		}
		t.Fatal("invalid PostgreSQL DSN was accepted")
	}
	if strings.Contains(err.Error(), password) {
		t.Fatal("database error exposed the password")
	}
}

func TestPostgresConnection(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	db := openTestDB(t, "postgres", dsn)
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := pool.PingContext(ctx); err != nil {
		t.Fatalf("ping PostgreSQL: %v", err)
	}
	if got := pool.Stats().MaxOpenConnections; got != 10 {
		t.Fatalf("PostgreSQL max open connections = %d, want 10", got)
	}
}
