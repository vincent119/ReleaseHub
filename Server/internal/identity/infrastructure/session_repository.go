// Package infrastructure implements identity persistence adapters.
package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/vincent119/ReleaseHub/Server/internal/identity/application"
	identity "github.com/vincent119/ReleaseHub/Server/internal/identity/domain"
)

// SessionRepository is the PostgreSQL implementation of application.SessionRepository.
type SessionRepository struct{ db *gorm.DB }

var _ application.SessionRepository = (*SessionRepository)(nil)

// NewSessionRepository creates a session repository from the process database handle.
func NewSessionRepository(db *gorm.DB) (*SessionRepository, error) {
	if db == nil {
		return nil, fmt.Errorf("database is required")
	}
	return &SessionRepository{db: db}, nil
}

func (r *SessionRepository) CreateSession(ctx context.Context, session identity.Session) error {
	return r.db.WithContext(ctx).Create(sessionModelFromDomain(session)).Error
}

func (r *SessionRepository) FindSessionByTokenHash(ctx context.Context, tokenHash []byte) (identity.Session, error) {
	var model sessionModel
	if err := r.db.WithContext(ctx).Where("token_hash = ?", tokenHash).Take(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return identity.Session{}, fmt.Errorf("session not found")
		}
		return identity.Session{}, fmt.Errorf("find session: %w", err)
	}
	return model.toDomain(), nil
}

func (r *SessionRepository) TouchSession(ctx context.Context, id uuid.UUID, lastSeenAt, idleExpiresAt time.Time) error {
	result := r.db.WithContext(ctx).Model(&sessionModel{}).Where("id = ? AND revoked_at IS NULL", id).Updates(map[string]any{
		"last_seen_at": lastSeenAt.UTC(), "idle_expires_at": idleExpiresAt.UTC(),
	})
	if result.Error != nil {
		return fmt.Errorf("touch session: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("session not found or revoked")
	}
	return nil
}

func (r *SessionRepository) RevokeSession(ctx context.Context, id uuid.UUID, revokedAt time.Time) error {
	return r.db.WithContext(ctx).Model(&sessionModel{}).Where("id = ? AND revoked_at IS NULL", id).Update("revoked_at", revokedAt.UTC()).Error
}

func (r *SessionRepository) RevokeUserSessions(ctx context.Context, userID uuid.UUID, revokedAt time.Time) error {
	if err := r.db.WithContext(ctx).Model(&sessionModel{}).Where("user_id = ? AND revoked_at IS NULL", userID).Update("revoked_at", revokedAt.UTC()).Error; err != nil {
		return fmt.Errorf("revoke user sessions: %w", err)
	}
	return nil
}

func (r *SessionRepository) UpdateProviderCredential(ctx context.Context, sessionID uuid.UUID, ciphertext []byte, verifiedAt time.Time) error {
	result := r.db.WithContext(ctx).Model(&sessionModel{}).Where("id = ? AND revoked_at IS NULL", sessionID).Updates(map[string]any{"refresh_token_ciphertext": ciphertext, "identity_verified_at": verifiedAt.UTC()})
	if result.Error != nil {
		return fmt.Errorf("update provider credential: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("session not found or revoked")
	}
	return nil
}

type sessionModel struct {
	ID                     uuid.UUID `gorm:"type:uuid;primaryKey"`
	UserID                 uuid.UUID `gorm:"type:uuid;not null"`
	TokenHash              []byte    `gorm:"not null"`
	CSRFTokenHash          []byte    `gorm:"not null"`
	RefreshTokenCiphertext []byte    `gorm:"not null"`
	IdentityVerifiedAt     time.Time `gorm:"not null"`
	CreatedAt              time.Time `gorm:"not null"`
	LastSeenAt             time.Time `gorm:"not null"`
	IdleExpiresAt          time.Time `gorm:"not null"`
	AbsoluteExpiresAt      time.Time `gorm:"not null"`
	RevokedAt              *time.Time
}

func (sessionModel) TableName() string { return "sessions" }
func sessionModelFromDomain(session identity.Session) sessionModel {
	return sessionModel{ID: session.ID, UserID: session.UserID, TokenHash: session.TokenHash, CSRFTokenHash: session.CSRFTokenHash, RefreshTokenCiphertext: session.RefreshTokenCiphertext, IdentityVerifiedAt: session.IdentityVerifiedAt, CreatedAt: session.CreatedAt, LastSeenAt: session.LastSeenAt, IdleExpiresAt: session.IdleExpiresAt, AbsoluteExpiresAt: session.AbsoluteExpiresAt, RevokedAt: session.RevokedAt}
}
func (m sessionModel) toDomain() identity.Session {
	return identity.Session{ID: m.ID, UserID: m.UserID, TokenHash: m.TokenHash, CSRFTokenHash: m.CSRFTokenHash, RefreshTokenCiphertext: m.RefreshTokenCiphertext, IdentityVerifiedAt: m.IdentityVerifiedAt, CreatedAt: m.CreatedAt, LastSeenAt: m.LastSeenAt, IdleExpiresAt: m.IdleExpiresAt, AbsoluteExpiresAt: m.AbsoluteExpiresAt, RevokedAt: m.RevokedAt}
}
