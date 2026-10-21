package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
)

// Migrations are plain SQL files named NNN_description.sql and applied in
// order inside one transaction each. PRAGMA user_version records the last
// applied number, so a database always states exactly which files it has seen.
//
//go:embed migrations/*.sql
var migrationFS embed.FS

// ErrSchemaTooNew means the file was written by a newer binary. Refusing to
// open is the only safe answer: a downgrade could misread or corrupt tables it
// does not know about.
var ErrSchemaTooNew = errors.New("database schema is newer than this binary")

type migration struct {
	version int
	name    string
	sql     string
}

func loadMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("read migrations: %w", err)
	}
	migrations := make([]migration, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		prefix, _, ok := strings.Cut(name, "_")
		version, convErr := strconv.Atoi(prefix)
		if !ok || convErr != nil || version <= 0 {
			return nil, fmt.Errorf("migration %q must be named NNN_description.sql", name)
		}
		body, err := fs.ReadFile(migrationFS, "migrations/"+name)
		if err != nil {
			return nil, fmt.Errorf("read migration %q: %w", name, err)
		}
		migrations = append(migrations, migration{version: version, name: name, sql: string(body)})
	}
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].version < migrations[j].version })
	for i, m := range migrations {
		if m.version != i+1 {
			return nil, fmt.Errorf("migration %q breaks the sequence: expected version %d", m.name, i+1)
		}
	}
	return migrations, nil
}

// SchemaVersion is the number of migrations this binary ships.
func SchemaVersion() int {
	migrations, err := loadMigrations()
	if err != nil {
		panic(err)
	}
	return len(migrations)
}

func migrate(ctx context.Context, db *sql.DB) error {
	migrations, err := loadMigrations()
	if err != nil {
		return err
	}
	var current int
	if err := db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&current); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if current > len(migrations) {
		return fmt.Errorf("%w: file is at version %d, binary supports %d", ErrSchemaTooNew, current, len(migrations))
	}
	for _, m := range migrations[current:] {
		if err := applyMigration(ctx, db, m); err != nil {
			return fmt.Errorf("apply migration %s: %w", m.name, err)
		}
	}
	return nil
}

func applyMigration(ctx context.Context, db *sql.DB, m migration) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, m.sql); err != nil {
		return err
	}
	// user_version is transactional in SQLite, so a failed migration leaves the
	// version untouched and the next start retries it.
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", m.version)); err != nil {
		return err
	}
	return tx.Commit()
}
