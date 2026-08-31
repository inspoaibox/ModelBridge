package db

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMigrationFilesAreSortedAndSQLOnly(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"002.sql", "001.sql", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("-- test"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	files, err := migrationFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || filepath.Base(files[0]) != "001.sql" || filepath.Base(files[1]) != "002.sql" {
		t.Fatalf("unexpected migration files: %v", files)
	}
}
