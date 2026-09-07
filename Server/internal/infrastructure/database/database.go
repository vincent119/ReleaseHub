// Package database contains PostgreSQL and GORM infrastructure implementations.
package database

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/vincent119/ReleaseHub/Server/internal/config"
)

// Open creates a GORM PostgreSQL connection with process-specific pool settings.
func Open(cfg config.DatabaseConfig, pool config.PoolConfig) (*gorm.DB, error) {
	databaseURL := buildURL(cfg, "postgres")
	db, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, fmt.Errorf("open PostgreSQL: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get PostgreSQL pool: %w", err)
	}
	sqlDB.SetMaxOpenConns(pool.MaxOpenConnections)
	sqlDB.SetMaxIdleConns(pool.MaxIdleConnections)
	sqlDB.SetConnMaxLifetime(pool.ConnectionLifetime)
	return db, nil
}

// Close releases the underlying PostgreSQL connection pool.
func Close(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("get PostgreSQL pool: %w", err)
	}
	if err := sqlDB.Close(); err != nil {
		return fmt.Errorf("close PostgreSQL pool: %w", err)
	}
	return nil
}

// URL returns the pgx5 connection URL for the explicit migrate command.
func URL(cfg config.DatabaseConfig) string {
	return buildURL(cfg, "pgx5")
}

// PostgreSQLURL returns a standard PostgreSQL URL for runtime pgx connections.
func PostgreSQLURL(cfg config.DatabaseConfig) string {
	return buildURL(cfg, "postgres")
}

func buildURL(cfg config.DatabaseConfig, scheme string) string {
	values := url.Values{}
	values.Set("sslmode", cfg.SSLMode)
	values.Set("TimeZone", cfg.Timezone)
	return scheme + "://" + url.UserPassword(cfg.User, cfg.Password).String() + "@" +
		cfg.Server + ":" + strconv.Itoa(cfg.Port) + "/" + url.PathEscape(cfg.Database) + "?" + values.Encode()
}

// Ping verifies the database connection within the caller's deadline.
func Ping(ctx context.Context, db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("get PostgreSQL pool: %w", err)
	}
	return sqlDB.PingContext(ctx)
}
