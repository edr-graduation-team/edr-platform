// Package database provides PostgreSQL connection management and auto-migration.
package database

import (
	"embed"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres" // postgres driver
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/sirupsen/logrus"
)

// migrationsFS embeds all .sql files from the migrations directory into the
// compiled binary. This ensures migrations are always available regardless of
// the working directory or Docker layer setup.
//
//go:embed migrations/*.sql
var migrationsFS embed.FS

// RunMigrations applies all pending database migrations using golang-migrate.
// It uses the embedded SQL files so no external scripts or files are needed.
//
// Behaviour:
//   - If migrations are already up to date, logs "Database schema is up to date".
//   - If migrations are applied, logs the count and new version.
//   - Returns an error only on real migration failures.
func RunMigrations(cfg *PostgresConfig, logger *logrus.Logger) error {
	// Build a stdlib-compatible DSN for golang-migrate (it uses database/sql, not pgx)
	dsn := fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Database, cfg.SSLMode,
	)

	// Create an iofs source from our embedded migrations
	source, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("failed to create migration source from embedded files: %w", err)
	}

	// Create the migrate instance
	m, err := migrate.NewWithSourceInstance("iofs", source, dsn)
	if err != nil {
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}
	defer m.Close()

	// Handle dirty database: if a previous migration crashed, the schema_migrations
	// table is left in a "dirty" state. Force the version clean so Up() can proceed.
	version, dirty, verErr := m.Version()
	if verErr != nil && !errors.Is(verErr, migrate.ErrNilVersion) {
		return fmt.Errorf("failed to read migration version: %w", verErr)
	}
	if dirty {
		logger.WithField("version", version).Warn("Database is in dirty state — forcing version clean")
		if err := m.Force(int(version)); err != nil {
			return fmt.Errorf("failed to force migration version %d: %w", version, err)
		}
	}

	// Run all pending migrations
	if err := m.Up(); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			logger.Info("Database schema is up to date — no migrations needed")
			return nil
		}
		return fmt.Errorf("migration failed: %w", err)
	}

	// Log the final version
	version, dirty, _ = m.Version()
	logger.WithFields(logrus.Fields{
		"version": version,
		"dirty":   dirty,
	}).Info("Database migrations applied successfully")

	return nil
}
