package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
	"github.com/vincent119/ReleaseHub/Server/internal/infrastructure/database"
)

// DeploymentScheduleRepository persists Environment schedule policies in PostgreSQL.
type DeploymentScheduleRepository struct{ db *gorm.DB }

type deploymentSchedulePut struct {
	mutation deployapp.DeploymentScheduleMutation
	policy   deploydomain.DeploymentSchedulePolicy
	command  deploymentScheduleCommandModel
	stored   *deploydomain.DeploymentSchedulePolicy
}

var _ deployapp.DeploymentScheduleRepository = (*DeploymentScheduleRepository)(nil)

// NewDeploymentScheduleRepository creates the PostgreSQL schedule adapter.
func NewDeploymentScheduleRepository(db *gorm.DB) (*DeploymentScheduleRepository, error) {
	if db == nil {
		return nil, errors.New("database is required")
	}
	return &DeploymentScheduleRepository{db: db}, nil
}

// ResolveEnvironment returns the canonical authorization scope for one Environment ID.
func (r *DeploymentScheduleRepository) ResolveEnvironment(ctx context.Context, environmentID uuid.UUID) (authz.Scope, error) {
	var value struct{ OrganizationID, ProjectID uuid.UUID }
	if environmentID == uuid.Nil {
		return authz.Scope{}, deployapp.ErrDeploymentScheduleNotFound
	}
	err := r.db.WithContext(ctx).Table("environments").Select("organization_id, project_id").Take(&value, "id = ?", environmentID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return authz.Scope{}, deployapp.ErrDeploymentScheduleNotFound
	}
	if err != nil {
		return authz.Scope{}, fmt.Errorf("resolve deployment schedule Environment: %w", err)
	}
	scope, err := authz.NewEnvironmentScope(value.OrganizationID, value.ProjectID, environmentID)
	if err != nil {
		return authz.Scope{}, deployapp.ErrDeploymentScheduleNotFound
	}
	return scope, nil
}

// Get loads one policy or nil when the Environment uses unrestricted defaults.
func (r *DeploymentScheduleRepository) Get(ctx context.Context, environmentID uuid.UUID) (*deploydomain.DeploymentSchedulePolicy, error) {
	return loadDeploymentSchedulePolicy(ctx, r.db, environmentID)
}

// Put applies one idempotent optimistic replacement with Audit and Outbox atomically.
func (r *DeploymentScheduleRepository) Put(ctx context.Context, mutation deployapp.DeploymentScheduleMutation, policy deploydomain.DeploymentSchedulePolicy) (deploydomain.DeploymentSchedulePolicy, error) {
	command, err := newDeploymentScheduleCommand(mutation, policy)
	if err != nil {
		return deploydomain.DeploymentSchedulePolicy{}, err
	}
	var stored deploydomain.DeploymentSchedulePolicy
	err = database.WithinTransaction(ctx, r.db, func(tx *gorm.DB) error {
		return putDeploymentSchedule(ctx, tx, deploymentSchedulePut{
			mutation: mutation, policy: policy, command: command, stored: &stored,
		})
	})
	return stored, err
}

func newDeploymentScheduleCommand(mutation deployapp.DeploymentScheduleMutation, policy deploydomain.DeploymentSchedulePolicy) (deploymentScheduleCommandModel, error) {
	document := deploymentScheduleDocument(policy)
	response, err := json.Marshal(document)
	if err != nil {
		return deploymentScheduleCommandModel{}, fmt.Errorf("marshal deployment schedule response: %w", err)
	}
	fingerprint, err := deploymentScheduleFingerprint(mutation.ExpectedVersion, document)
	return deploymentScheduleCommandModel{
		ActorID: mutation.ActorID, EnvironmentID: policy.EnvironmentID,
		IdempotencyKey: mutation.IdempotencyKey, RequestFingerprint: fingerprint,
		ResponsePolicy: datatypes.JSON(response), CreatedAt: mutation.OccurredAt,
	}, err
}

func putDeploymentSchedule(ctx context.Context, tx *gorm.DB, value deploymentSchedulePut) error {
	replayed, replay, err := reserveDeploymentScheduleCommand(tx.WithContext(ctx), value.command)
	if err != nil {
		return err
	}
	if replay {
		decoded, decodeErr := deploymentSchedulePolicyFromJSON(replayed.ResponsePolicy)
		*value.stored = decoded
		return decodeErr
	}
	if err := applyDeploymentSchedulePolicy(tx.WithContext(ctx), value.mutation, value.policy); err != nil {
		return err
	}
	*value.stored = value.policy
	return appendDeploymentScheduleChange(ctx, tx, value.mutation, value.policy)
}

func reserveDeploymentScheduleCommand(tx *gorm.DB, command deploymentScheduleCommandModel) (deploymentScheduleCommandModel, bool, error) {
	result := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "actor_id"}, {Name: "idempotency_key"}}, DoNothing: true}).Create(&command)
	if result.Error != nil {
		return deploymentScheduleCommandModel{}, false, fmt.Errorf("reserve deployment schedule command: %w", result.Error)
	}
	if result.RowsAffected == 1 {
		return command, false, nil
	}
	var existing deploymentScheduleCommandModel
	if err := tx.Where("actor_id = ? AND idempotency_key = ?", command.ActorID, command.IdempotencyKey).Take(&existing).Error; err != nil {
		return deploymentScheduleCommandModel{}, false, fmt.Errorf("load deployment schedule command: %w", err)
	}
	if existing.EnvironmentID != command.EnvironmentID || existing.RequestFingerprint != command.RequestFingerprint {
		return deploymentScheduleCommandModel{}, false, deployapp.ErrDeploymentScheduleConflict
	}
	return existing, true, nil
}

func applyDeploymentSchedulePolicy(tx *gorm.DB, mutation deployapp.DeploymentScheduleMutation, policy deploydomain.DeploymentSchedulePolicy) error {
	model, err := deploymentSchedulePolicyToModel(policy, mutation)
	if err != nil {
		return err
	}
	if mutation.ExpectedVersion == 0 {
		return createDeploymentSchedulePolicy(tx, model)
	}
	return updateDeploymentSchedulePolicy(tx, mutation.ExpectedVersion, model)
}

func createDeploymentSchedulePolicy(tx *gorm.DB, model deploymentSchedulePolicyModel) error {
	if err := tx.Create(&model).Error; err != nil {
		return deploymentScheduleWriteError(err)
	}
	return nil
}

func updateDeploymentSchedulePolicy(tx *gorm.DB, expected uint64, model deploymentSchedulePolicyModel) error {
	result := tx.Model(&deploymentSchedulePolicyModel{}).
		Where("environment_id = ? AND version = ?", model.EnvironmentID, expected).
		Updates(map[string]any{
			"enabled": model.Enabled, "time_zone": model.TimeZone,
			"weekly_windows": model.WeeklyWindows, "blackouts": model.Blackouts,
			"version": model.Version, "updated_by": model.UpdatedBy, "updated_at": model.UpdatedAt,
		})
	if result.Error != nil {
		return deploymentScheduleWriteError(result.Error)
	}
	if result.RowsAffected != 1 {
		return deployapp.ErrDeploymentScheduleConflict
	}
	return nil
}

func deploymentSchedulePolicyToModel(policy deploydomain.DeploymentSchedulePolicy, mutation deployapp.DeploymentScheduleMutation) (deploymentSchedulePolicyModel, error) {
	document := deploymentScheduleDocument(policy)
	windows, err := json.Marshal(document.WeeklyWindows)
	if err != nil {
		return deploymentSchedulePolicyModel{}, fmt.Errorf("marshal deployment schedule windows: %w", err)
	}
	blackouts, err := json.Marshal(document.Blackouts)
	if err != nil {
		return deploymentSchedulePolicyModel{}, fmt.Errorf("marshal deployment schedule blackouts: %w", err)
	}
	return deploymentSchedulePolicyModel{
		EnvironmentID: policy.EnvironmentID, Enabled: policy.Enabled, TimeZone: policy.TimeZone,
		WeeklyWindows: datatypes.JSON(windows), Blackouts: datatypes.JSON(blackouts), Version: policy.Version,
		CreatedBy: mutation.ActorID, UpdatedBy: mutation.ActorID,
		CreatedAt: mutation.OccurredAt, UpdatedAt: mutation.OccurredAt,
	}, nil
}

func deploymentSchedulePolicyFromModel(model deploymentSchedulePolicyModel) (deploydomain.DeploymentSchedulePolicy, error) {
	var windows []deploymentScheduleWindowDocument
	if err := json.Unmarshal(model.WeeklyWindows, &windows); err != nil {
		return deploydomain.DeploymentSchedulePolicy{}, fmt.Errorf("decode deployment schedule windows: %w", err)
	}
	var blackouts []deploymentScheduleBlackoutDocument
	if err := json.Unmarshal(model.Blackouts, &blackouts); err != nil {
		return deploydomain.DeploymentSchedulePolicy{}, fmt.Errorf("decode deployment schedule blackouts: %w", err)
	}
	return deploymentSchedulePolicyFromDocument(deploymentSchedulePolicyDocument{
		EnvironmentID: model.EnvironmentID, Enabled: model.Enabled, TimeZone: model.TimeZone,
		WeeklyWindows: windows, Blackouts: blackouts, Version: model.Version,
	})
}

func deploymentSchedulePolicyFromJSON(value []byte) (deploydomain.DeploymentSchedulePolicy, error) {
	var document deploymentSchedulePolicyDocument
	if err := json.Unmarshal(value, &document); err != nil {
		return deploydomain.DeploymentSchedulePolicy{}, fmt.Errorf("decode deployment schedule response: %w", err)
	}
	return deploymentSchedulePolicyFromDocument(document)
}

func deploymentSchedulePolicyFromDocument(document deploymentSchedulePolicyDocument) (deploydomain.DeploymentSchedulePolicy, error) {
	windows := make([]deploydomain.DeploymentScheduleWeeklyWindow, 0, len(document.WeeklyWindows))
	for _, window := range document.WeeklyWindows {
		windows = append(windows, deploydomain.DeploymentScheduleWeeklyWindow{
			DayOfWeek: time.Weekday(window.DayOfWeek), StartMinute: window.StartMinute, EndMinute: window.EndMinute,
		})
	}
	blackouts := make([]deploydomain.DeploymentScheduleBlackout, 0, len(document.Blackouts))
	for _, blackout := range document.Blackouts {
		blackouts = append(blackouts, deploydomain.DeploymentScheduleBlackout{StartsAt: blackout.StartsAt, EndsAt: blackout.EndsAt})
	}
	policy, err := deploydomain.NewDeploymentSchedulePolicy(deploydomain.DeploymentSchedulePolicyDraft{
		EnvironmentID: document.EnvironmentID, Enabled: document.Enabled, TimeZone: document.TimeZone,
		WeeklyWindows: windows, Blackouts: blackouts, Version: document.Version,
	})
	if err != nil {
		return deploydomain.DeploymentSchedulePolicy{}, fmt.Errorf("decode deployment schedule policy: %w", err)
	}
	return policy, nil
}

func deploymentScheduleDocument(policy deploydomain.DeploymentSchedulePolicy) deploymentSchedulePolicyDocument {
	windows := make([]deploymentScheduleWindowDocument, 0, len(policy.WeeklyWindows))
	for _, window := range policy.WeeklyWindows {
		windows = append(windows, deploymentScheduleWindowDocument{DayOfWeek: int(window.DayOfWeek), StartMinute: window.StartMinute, EndMinute: window.EndMinute})
	}
	blackouts := make([]deploymentScheduleBlackoutDocument, 0, len(policy.Blackouts))
	for _, blackout := range policy.Blackouts {
		blackouts = append(blackouts, deploymentScheduleBlackoutDocument{StartsAt: blackout.StartsAt.UTC(), EndsAt: blackout.EndsAt.UTC()})
	}
	return deploymentSchedulePolicyDocument{
		EnvironmentID: policy.EnvironmentID, Enabled: policy.Enabled, TimeZone: policy.TimeZone,
		WeeklyWindows: windows, Blackouts: blackouts, Version: policy.Version,
	}
}

func deploymentScheduleWriteError(err error) error {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) && postgresError.Code == "23505" {
		return deployapp.ErrDeploymentScheduleConflict
	}
	return fmt.Errorf("write deployment schedule: %w", err)
}
