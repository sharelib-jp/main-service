package migrations

import (
	"testing"
	"testing/fstest"

	"github.com/sharelib-jp/main-service/internal/database"
)

func TestApplyVersionedMigrations(t *testing.T) {
	db, err := database.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	pool, _ := db.DB()
	defer pool.Close()
	files := fstest.MapFS{"sqlite/001_first.sql": &fstest.MapFile{Data: []byte("CREATE TABLE records (id INTEGER PRIMARY KEY); INSERT INTO records VALUES (1);")}}
	for i := 0; i < 2; i++ {
		if err := apply(db, files); err != nil {
			t.Fatal(err)
		}
	}
	var count int64
	if err := db.Table("records").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("count = %d, err = %v", count, err)
	}
	files["sqlite/001_first.sql"] = &fstest.MapFile{Data: []byte("SELECT 1;")}
	if err := apply(db, files); err == nil {
		t.Fatal("changed migration accepted")
	}
	files["sqlite/001_first.sql"] = &fstest.MapFile{Data: []byte("CREATE TABLE records (id INTEGER PRIMARY KEY); INSERT INTO records VALUES (1);")}
	files["sqlite/002_bad.sql"] = &fstest.MapFile{Data: []byte("CREATE TABLE rolled_back (id INTEGER); INVALID SQL;")}
	if err := apply(db, files); err == nil {
		t.Fatal("invalid migration accepted")
	}
	if db.Migrator().HasTable("rolled_back") {
		t.Fatal("failed migration was not rolled back")
	}
}
