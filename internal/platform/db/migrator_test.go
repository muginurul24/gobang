package db

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSplitSQLStatements(t *testing.T) {
	sql := `
		CREATE TABLE users (id text, note text DEFAULT 'semi;colon');
		-- comment ; should be ignored
		INSERT INTO users (id, note) VALUES ('1', $$value;inside$$);
		/* block ; comment */
		CREATE FUNCTION ping() RETURNS text AS $fn$
		BEGIN
			RETURN 'pong;';
		END;
		$fn$ LANGUAGE plpgsql;
	`

	got := splitSQLStatements(sql)
	want := []string{
		"CREATE TABLE users (id text, note text DEFAULT 'semi;colon')",
		"-- comment ; should be ignored\n\t\tINSERT INTO users (id, note) VALUES ('1', $$value;inside$$)",
		"/* block ; comment */\n\t\tCREATE FUNCTION ping() RETURNS text AS $fn$\n\t\tBEGIN\n\t\t\tRETURN 'pong;';\n\t\tEND;\n\t\t$fn$ LANGUAGE plpgsql",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("splitSQLStatements() = %#v, want %#v", got, want)
	}
}

func TestCollectMigrationsSortsUpFiles(t *testing.T) {
	dir := t.TempDir()

	files := map[string]string{
		"000002_create_users.up.sql":      "CREATE TABLE users(id int);",
		"000001_enable_extensions.up.sql": `CREATE EXTENSION IF NOT EXISTS "citext";`,
		"000002_create_users.down.sql":    "DROP TABLE users;",
	}

	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile(%s): %v", name, err)
		}
	}

	got, err := collectMigrations(dir)
	if err != nil {
		t.Fatalf("collectMigrations() error = %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}

	if got[0].Version != "000001" || got[1].Version != "000002" {
		t.Fatalf("versions = [%s %s], want [000001 000002]", got[0].Version, got[1].Version)
	}
}
