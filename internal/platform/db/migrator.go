package db

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Pool = pgxpool.Pool

type Migrator struct {
	pool *pgxpool.Pool
	dir  string
}

type Migration struct {
	Version  string
	Name     string
	UpPath   string
	DownPath string
}

func NewMigrator(pool *pgxpool.Pool, dir string) Migrator {
	if dir == "" {
		dir = "migrations"
	}

	return Migrator{
		pool: pool,
		dir:  dir,
	}
}

func (m Migrator) Up(ctx context.Context) (int, error) {
	if err := m.ensureSchemaMigrations(ctx); err != nil {
		return 0, err
	}

	migrations, err := collectMigrations(m.dir)
	if err != nil {
		return 0, err
	}

	appliedVersions, err := m.appliedVersions(ctx)
	if err != nil {
		return 0, err
	}

	appliedCount := 0
	for _, migration := range migrations {
		if _, ok := appliedVersions[migration.Version]; ok {
			continue
		}

		statements, err := readSQLStatements(migration.UpPath)
		if err != nil {
			return appliedCount, fmt.Errorf("read migration %s: %w", migration.UpPath, err)
		}

		if err := runStatementsTx(ctx, m.pool, statements, func(tx pgx.Tx) error {
			_, err := tx.Exec(
				ctx,
				`INSERT INTO schema_migrations (version, name) VALUES ($1, $2)`,
				migration.Version,
				migration.Name,
			)
			return err
		}); err != nil {
			return appliedCount, fmt.Errorf("apply migration %s: %w", migration.UpPath, err)
		}

		appliedCount++
	}

	return appliedCount, nil
}

func (m Migrator) Fresh(ctx context.Context) (int, error) {
	resetStatements := []string{
		`DROP SCHEMA IF EXISTS public CASCADE`,
		`CREATE SCHEMA public`,
	}

	if err := runStatementsTx(ctx, m.pool, resetStatements, nil); err != nil {
		return 0, fmt.Errorf("reset public schema: %w", err)
	}

	return m.Up(ctx)
}

func (m Migrator) Down(ctx context.Context) (int, error) {
	if err := m.ensureSchemaMigrations(ctx); err != nil {
		return 0, err
	}

	lastApplied, ok, err := m.lastApplied(ctx)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, nil
	}

	migrations, err := collectMigrations(m.dir)
	if err != nil {
		return 0, err
	}

	var target Migration
	found := false
	for _, migration := range migrations {
		if migration.Version == lastApplied.Version {
			target = migration
			found = true
			break
		}
	}

	if !found {
		return 0, fmt.Errorf("migration %s not found on disk", lastApplied.Version)
	}

	if target.DownPath == "" {
		return 0, fmt.Errorf("migration %s does not define a .down.sql file", target.Version)
	}

	statements, err := readSQLStatements(target.DownPath)
	if err != nil {
		return 0, fmt.Errorf("read migration %s: %w", target.DownPath, err)
	}

	if err := runStatementsTx(ctx, m.pool, statements, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `DELETE FROM schema_migrations WHERE version = $1`, target.Version)
		return err
	}); err != nil {
		return 0, fmt.Errorf("rollback migration %s: %w", target.DownPath, err)
	}

	return 1, nil
}

func (m Migrator) ensureSchemaMigrations(ctx context.Context) error {
	_, err := m.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version text PRIMARY KEY,
			name text NOT NULL,
			applied_at timestamptz NOT NULL DEFAULT now()
		)
	`)
	if err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}

	return nil
}

func (m Migrator) appliedVersions(ctx context.Context) (map[string]struct{}, error) {
	rows, err := m.pool.Query(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("query schema_migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[string]struct{})
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("scan schema_migrations: %w", err)
		}

		applied[version] = struct{}{}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate schema_migrations: %w", err)
	}

	return applied, nil
}

func (m Migrator) lastApplied(ctx context.Context) (Migration, bool, error) {
	row := m.pool.QueryRow(
		ctx,
		`SELECT version, name FROM schema_migrations ORDER BY version DESC LIMIT 1`,
	)

	var migration Migration
	if err := row.Scan(&migration.Version, &migration.Name); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Migration{}, false, nil
		}

		return Migration{}, false, fmt.Errorf("query last schema migration: %w", err)
	}

	return migration, true, nil
}

func ApplySQLDir(ctx context.Context, pool *pgxpool.Pool, dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}

		return 0, fmt.Errorf("read sql dir %s: %w", dir, err)
	}

	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		paths = append(paths, filepath.Join(dir, entry.Name()))
	}

	sort.Strings(paths)

	applied := 0
	for _, path := range paths {
		statements, err := readSQLStatements(path)
		if err != nil {
			return applied, fmt.Errorf("read sql file %s: %w", path, err)
		}

		if err := runStatementsTx(ctx, pool, statements, nil); err != nil {
			return applied, fmt.Errorf("apply sql file %s: %w", path, err)
		}

		applied++
	}

	return applied, nil
}

func runStatementsTx(ctx context.Context, pool *pgxpool.Pool, statements []string, after func(pgx.Tx) error) error {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	for _, statement := range statements {
		if _, err := tx.Exec(ctx, statement); err != nil {
			return fmt.Errorf("exec statement: %w", err)
		}
	}

	if after != nil {
		if err := after(tx); err != nil {
			return fmt.Errorf("after hook: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

func collectMigrations(dir string) ([]Migration, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}

		return nil, fmt.Errorf("read migrations dir %s: %w", dir, err)
	}

	migrationIndex := make(map[string]Migration)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		switch {
		case strings.HasSuffix(name, ".up.sql"):
			key, version, title, err := parseMigrationName(name, ".up.sql")
			if err != nil {
				return nil, err
			}

			migration := migrationIndex[key]
			migration.Version = version
			migration.Name = title
			migration.UpPath = filepath.Join(dir, name)
			migrationIndex[key] = migration
		case strings.HasSuffix(name, ".down.sql"):
			key, version, title, err := parseMigrationName(name, ".down.sql")
			if err != nil {
				return nil, err
			}

			migration := migrationIndex[key]
			migration.Version = version
			migration.Name = title
			migration.DownPath = filepath.Join(dir, name)
			migrationIndex[key] = migration
		}
	}

	migrations := make([]Migration, 0, len(migrationIndex))
	for _, migration := range migrationIndex {
		if migration.UpPath == "" {
			return nil, fmt.Errorf("migration %s is missing an .up.sql file", migration.Version)
		}

		migrations = append(migrations, migration)
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}

func parseMigrationName(filename string, suffix string) (string, string, string, error) {
	base := strings.TrimSuffix(filename, suffix)
	parts := strings.SplitN(base, "_", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", "", fmt.Errorf("invalid migration name %s", filename)
	}

	return base, parts[0], parts[1], nil
}

func readSQLStatements(path string) ([]string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return splitSQLStatements(string(content)), nil
}

func splitSQLStatements(sql string) []string {
	statements := make([]string, 0)
	var builder strings.Builder

	inSingleQuote := false
	inDoubleQuote := false
	inLineComment := false
	inBlockComment := false
	dollarQuote := ""

	for i := 0; i < len(sql); i++ {
		current := sql[i]

		if inLineComment {
			builder.WriteByte(current)
			if current == '\n' {
				inLineComment = false
			}
			continue
		}

		if inBlockComment {
			builder.WriteByte(current)
			if current == '*' && i+1 < len(sql) && sql[i+1] == '/' {
				builder.WriteByte(sql[i+1])
				i++
				inBlockComment = false
			}
			continue
		}

		if dollarQuote != "" {
			if strings.HasPrefix(sql[i:], dollarQuote) {
				builder.WriteString(dollarQuote)
				i += len(dollarQuote) - 1
				dollarQuote = ""
				continue
			}

			builder.WriteByte(current)
			continue
		}

		if inSingleQuote {
			builder.WriteByte(current)
			if current == '\'' {
				if i+1 < len(sql) && sql[i+1] == '\'' {
					builder.WriteByte(sql[i+1])
					i++
					continue
				}

				inSingleQuote = false
			}
			continue
		}

		if inDoubleQuote {
			builder.WriteByte(current)
			if current == '"' {
				if i+1 < len(sql) && sql[i+1] == '"' {
					builder.WriteByte(sql[i+1])
					i++
					continue
				}

				inDoubleQuote = false
			}
			continue
		}

		if current == '-' && i+1 < len(sql) && sql[i+1] == '-' {
			builder.WriteString("--")
			i++
			inLineComment = true
			continue
		}

		if current == '/' && i+1 < len(sql) && sql[i+1] == '*' {
			builder.WriteString("/*")
			i++
			inBlockComment = true
			continue
		}

		if current == '\'' {
			builder.WriteByte(current)
			inSingleQuote = true
			continue
		}

		if current == '"' {
			builder.WriteByte(current)
			inDoubleQuote = true
			continue
		}

		if current == '$' {
			if tag := scanDollarQuote(sql[i:]); tag != "" {
				builder.WriteString(tag)
				i += len(tag) - 1
				dollarQuote = tag
				continue
			}
		}

		if current == ';' {
			statement := strings.TrimSpace(builder.String())
			if statement != "" {
				statements = append(statements, statement)
			}

			builder.Reset()
			continue
		}

		builder.WriteByte(current)
	}

	statement := strings.TrimSpace(builder.String())
	if statement != "" {
		statements = append(statements, statement)
	}

	return statements
}

func scanDollarQuote(sql string) string {
	if len(sql) < 2 || sql[0] != '$' {
		return ""
	}

	for i := 1; i < len(sql); i++ {
		switch current := sql[i]; {
		case current == '$':
			return sql[:i+1]
		case (current >= 'a' && current <= 'z') || (current >= 'A' && current <= 'Z') || (current >= '0' && current <= '9') || current == '_':
		default:
			return ""
		}
	}

	return ""
}
