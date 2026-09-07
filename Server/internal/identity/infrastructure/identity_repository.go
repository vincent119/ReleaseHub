package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identity "github.com/vincent119/ReleaseHub/Server/internal/identity/domain"
)

// IdentityRepository persists OIDC identity bindings and local users.
type IdentityRepository struct{ db *gorm.DB }

func NewIdentityRepository(db *gorm.DB) (*IdentityRepository, error) {
	if db == nil {
		return nil, fmt.Errorf("database is required")
	}
	return &IdentityRepository{db: db}, nil
}

func (r *IdentityRepository) FindOrCreateIdentity(ctx context.Context, issuer, subject, username string) (identity.User, error) {
	var resolved userModel
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var binding oidcIdentityModel
		if err := tx.Where("issuer = ? AND subject = ?", issuer, subject).Take(&binding).Error; err == nil {
			return tx.Where("id = ?", binding.UserID).Take(&resolved).Error
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		resolved = userModel{ID: uuid.New(), Username: username}
		if err := tx.Create(&resolved).Error; err != nil {
			return fmt.Errorf("create user: %w", err)
		}
		if err := tx.Create(&oidcIdentityModel{ID: uuid.New(), UserID: resolved.ID, Issuer: issuer, Subject: subject}).Error; err != nil {
			return fmt.Errorf("create OIDC identity: %w", err)
		}
		return nil
	})
	if err != nil {
		return identity.User{}, err
	}
	return identity.User{ID: resolved.ID, Username: resolved.Username, Disabled: resolved.DisabledAt != nil}, nil
}

func (r *IdentityRepository) FindUser(ctx context.Context, userID uuid.UUID) (identity.User, error) {
	var model userModel
	if err := r.db.WithContext(ctx).Where("id = ?", userID).Take(&model).Error; err != nil {
		return identity.User{}, fmt.Errorf("find user: %w", err)
	}
	return identity.User{ID: model.ID, Username: model.Username, Disabled: model.DisabledAt != nil}, nil
}

func (r *IdentityRepository) FindUserByIdentity(ctx context.Context, issuer, subject string) (identity.User, error) {
	var model userModel
	err := r.db.WithContext(ctx).Table("users").Select("users.*").Joins("JOIN oidc_identities ON oidc_identities.user_id = users.id").Where("oidc_identities.issuer = ? AND oidc_identities.subject = ?", issuer, subject).Take(&model).Error
	if err != nil {
		return identity.User{}, fmt.Errorf("find user by OIDC identity: %w", err)
	}
	return identity.User{ID: model.ID, Username: model.Username, Disabled: model.DisabledAt != nil}, nil
}

type userModel struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey"`
	Username   string
	DisabledAt *time.Time
}

func (userModel) TableName() string { return "users" }

type oidcIdentityModel struct {
	ID      uuid.UUID `gorm:"type:uuid;primaryKey"`
	UserID  uuid.UUID `gorm:"type:uuid"`
	Issuer  string
	Subject string
}

func (oidcIdentityModel) TableName() string { return "oidc_identities" }
