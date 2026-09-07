package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/vincent119/ReleaseHub/Server/internal/infrastructure/database"
	platform "github.com/vincent119/ReleaseHub/Server/internal/platform/domain"
)

// OIDCGroupRepository persists provider-derived viewer memberships atomically.
type OIDCGroupRepository struct{ db *gorm.DB }

// NewOIDCGroupRepository creates the OIDC group repository.
func NewOIDCGroupRepository(db *gorm.DB) (*OIDCGroupRepository, error) {
	if db == nil {
		return nil, fmt.Errorf("database is required")
	}
	return &OIDCGroupRepository{db: db}, nil
}

// SyncOIDCGroups replaces memberships for one issuer without creating unknown groups.
func (r *OIDCGroupRepository) SyncOIDCGroups(ctx context.Context, userID uuid.UUID, issuer string, providerGroups []string) error {
	return database.WithinTransaction(ctx, r.db, func(tx *gorm.DB) error {
		target, err := r.resolveMappedGroups(ctx, tx, issuer, providerGroups)
		if err != nil {
			return err
		}
		current, err := r.currentGroups(ctx, tx, userID, issuer)
		if err != nil {
			return err
		}
		if slices.Equal(current, target) {
			return nil
		}
		if err := tx.WithContext(ctx).Model(&groupMembershipModel{}).
			Where("user_id = ? AND source = ? AND issuer = ?", userID, "oidc", issuer).
			Update("active", false).Error; err != nil {
			return fmt.Errorf("deactivate OIDC group memberships: %w", err)
		}
		for _, groupID := range target {
			if err := tx.WithContext(ctx).Exec(`
INSERT INTO authorization_group_memberships (id, group_id, user_id, source, issuer, active)
VALUES (?, ?, ?, 'oidc', ?, true)
ON CONFLICT (group_id, user_id, issuer) WHERE source = 'oidc'
DO UPDATE SET active = true, updated_at = excluded.updated_at`, uuid.New(), groupID, userID, issuer).Error; err != nil {
				return fmt.Errorf("activate OIDC group membership: %w", err)
			}
		}
		return appendOIDCGroupSyncRecords(ctx, tx, userID, len(current), len(target))
	})
}

func (r *OIDCGroupRepository) resolveMappedGroups(ctx context.Context, tx *gorm.DB, issuer string, providerGroups []string) ([]uuid.UUID, error) {
	if len(providerGroups) == 0 {
		return nil, nil
	}
	var groupIDs []uuid.UUID
	err := tx.WithContext(ctx).Table("authorization_oidc_group_mappings AS mapping").
		Select("mapping.group_id").
		Joins("JOIN authorization_groups auth_group ON auth_group.id = mapping.group_id").
		Where("mapping.issuer = ? AND mapping.provider_group IN ? AND mapping.active AND auth_group.oidc_viewer_only AND auth_group.disabled_at IS NULL", issuer, providerGroups).
		Order("mapping.group_id").Scan(&groupIDs).Error
	if err != nil {
		return nil, fmt.Errorf("resolve OIDC group mappings: %w", err)
	}
	return groupIDs, nil
}

func (r *OIDCGroupRepository) currentGroups(ctx context.Context, tx *gorm.DB, userID uuid.UUID, issuer string) ([]uuid.UUID, error) {
	var groupIDs []uuid.UUID
	if err := tx.WithContext(ctx).Model(&groupMembershipModel{}).
		Select("group_id").Where("user_id = ? AND source = ? AND issuer = ? AND active", userID, "oidc", issuer).
		Order("group_id").Scan(&groupIDs).Error; err != nil {
		return nil, fmt.Errorf("load OIDC group memberships: %w", err)
	}
	return groupIDs, nil
}

func appendOIDCGroupSyncRecords(ctx context.Context, tx *gorm.DB, userID uuid.UUID, previousCount, currentCount int) error {
	now := time.Now().UTC()
	metadata := map[string]any{"previous_count": previousCount, "current_count": currentCount}
	if err := database.AppendAudit(ctx, tx, database.AuditRecord{
		OccurredAt: now, ActorID: &userID, Action: "authorization.oidc_groups.sync",
		ResourceType: "user", ResourceID: userID.String(), Metadata: metadata,
	}); err != nil {
		return err
	}
	payload, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshal OIDC group sync event: %w", err)
	}
	event, err := platform.NewEvent("OIDCGroupMembershipsSynchronized", "user", userID.String(), payload, now)
	if err != nil {
		return err
	}
	return database.AppendOutbox(ctx, tx, event)
}

type groupMembershipModel struct {
	ID      uuid.UUID `gorm:"type:uuid;primaryKey"`
	GroupID uuid.UUID `gorm:"type:uuid"`
	UserID  uuid.UUID `gorm:"type:uuid"`
	Source  string
	Issuer  *string
	Active  bool
}

func (groupMembershipModel) TableName() string { return "authorization_group_memberships" }
