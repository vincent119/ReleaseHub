package database

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// WithinTransaction executes work atomically and rolls it back when work returns an error.
func WithinTransaction(ctx context.Context, db *gorm.DB, work func(*gorm.DB) error) error {
	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return work(tx)
	}); err != nil {
		return fmt.Errorf("run database transaction: %w", err)
	}
	return nil
}
