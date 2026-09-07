package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	argoapp "github.com/vincent119/ReleaseHub/Server/internal/argocd/application"
	argodomain "github.com/vincent119/ReleaseHub/Server/internal/argocd/domain"
)

// CommandRepository persists idempotency records for onboarding commands.
type CommandRepository struct{ db *gorm.DB }

// NewCommandRepository creates the PostgreSQL onboarding command adapter.
func NewCommandRepository(db *gorm.DB) (*CommandRepository, error) {
	if db == nil {
		return nil, errors.New("database is required")
	}
	return &CommandRepository{db: db}, nil
}

// Reserve creates one command record or returns the existing record for an idempotent replay.
func (r *CommandRepository) Reserve(ctx context.Context, command argoapp.OnboardingCommand) (argoapp.OnboardingCommand, bool, error) {
	model := (commandModel{}).fromDomain(command)
	result := r.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "actor_id"}, {Name: "operation"}, {Name: "idempotency_key"}}, DoNothing: true}).Create(&model)
	if result.Error != nil {
		return argoapp.OnboardingCommand{}, false, fmt.Errorf("reserve onboarding command: %w", result.Error)
	}
	if result.RowsAffected == 1 {
		return command, false, nil
	}
	var existing commandModel
	if err := r.db.WithContext(ctx).Where("actor_id = ? AND operation = ? AND idempotency_key = ?", command.ActorID, string(command.Operation), command.IdempotencyKey).Take(&existing).Error; err != nil {
		return argoapp.OnboardingCommand{}, false, fmt.Errorf("load existing onboarding command: %w", err)
	}
	resolved := existing.domain()
	if resolved.RequestFingerprint != command.RequestFingerprint || resolved.ApplicationID != command.ApplicationID {
		return argoapp.OnboardingCommand{}, true, argoapp.ErrIdempotencyKeyReuse
	}
	return resolved, true, nil
}

// Complete records a stable command outcome only while the command remains accepted.
func (r *CommandRepository) Complete(ctx context.Context, commandID uuid.UUID, status argodomain.OnboardingStatus, version uint64, responseCode string, now time.Time) (argoapp.OnboardingCommand, error) {
	query := r.db.WithContext(ctx).Model(&commandModel{}).Where("id = ? AND state = ?", commandID, argoapp.CommandAccepted).Updates(map[string]any{
		"state": argoapp.CommandCompleted, "onboarding_status": string(status), "onboarding_version": version,
		"response_code": responseCode, "completed_at": now.UTC(), "updated_at": now.UTC(),
	})
	if query.Error != nil {
		return argoapp.OnboardingCommand{}, fmt.Errorf("complete onboarding command: %w", query.Error)
	}
	var model commandModel
	if err := r.db.WithContext(ctx).Where("id = ?", commandID).Take(&model).Error; err != nil {
		return argoapp.OnboardingCommand{}, fmt.Errorf("load completed onboarding command: %w", err)
	}
	return model.domain(), nil
}

type commandModel struct {
	ID                 uuid.UUID `gorm:"type:uuid;primaryKey"`
	ActorID            uuid.UUID `gorm:"type:uuid"`
	ApplicationID      uuid.UUID `gorm:"type:uuid"`
	Operation          string
	IdempotencyKey     string
	RequestFingerprint string
	State              string
	OnboardingStatus   string
	OnboardingVersion  *uint64
	ResponseCode       string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	CompletedAt        *time.Time
}

func (commandModel) TableName() string { return "onboarding_commands" }

func (m commandModel) fromDomain(command argoapp.OnboardingCommand) commandModel {
	return commandModel{ID: command.ID, ActorID: command.ActorID, ApplicationID: command.ApplicationID, Operation: string(command.Operation), IdempotencyKey: command.IdempotencyKey, RequestFingerprint: command.RequestFingerprint, State: string(command.State), OnboardingStatus: string(command.OnboardingStatus), OnboardingVersion: command.OnboardingVersion, ResponseCode: command.ResponseCode, CreatedAt: command.CreatedAt, UpdatedAt: command.UpdatedAt, CompletedAt: command.CompletedAt}
}

func (m commandModel) domain() argoapp.OnboardingCommand {
	return argoapp.OnboardingCommand{ID: m.ID, ActorID: m.ActorID, ApplicationID: m.ApplicationID, Operation: argoapp.CommandOperation(m.Operation), IdempotencyKey: m.IdempotencyKey, RequestFingerprint: m.RequestFingerprint, State: argoapp.CommandState(m.State), OnboardingStatus: argodomain.OnboardingStatus(m.OnboardingStatus), OnboardingVersion: m.OnboardingVersion, ResponseCode: m.ResponseCode, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt, CompletedAt: m.CompletedAt}
}
