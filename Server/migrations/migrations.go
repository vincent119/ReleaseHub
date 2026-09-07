// Package migrations embeds and executes versioned SQL migrations.
package migrations

import (
	"embed"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// Files contains all versioned SQL migrations distributed with the Server image.
//
//go:embed *.sql
var Files embed.FS

// Up applies every pending migration. It is only called by the migrate command.
func Up(databaseURL string) error {
	source, err := iofs.New(Files, ".")
	if err != nil {
		return fmt.Errorf("create migration source: %w", err)
	}
	migrator, err := migrate.NewWithSourceInstance("iofs", source, databaseURL)
	if err != nil {
		return fmt.Errorf("create migrator: %w", err)
	}
	defer func() {
		_, _ = migrator.Close()
	}()
	if err := migrator.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}
