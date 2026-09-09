package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	identityapp "github.com/vincent119/ReleaseHub/Server/internal/identity/application"
	identity "github.com/vincent119/ReleaseHub/Server/internal/identity/domain"
	"github.com/vincent119/ReleaseHub/Server/internal/infrastructure/database"
	platform "github.com/vincent119/ReleaseHub/Server/internal/platform/domain"
)

const platformAdministratorRoleID = "00000000-0000-0000-0000-000000000100"

// LocalAuthRepository persists bootstrap-manager credentials and authorization membership.
type LocalAuthRepository struct{ db *gorm.DB }

func NewLocalAuthRepository(db *gorm.DB) (*LocalAuthRepository, error) {
	if db == nil {
		return nil, fmt.Errorf("database is required")
	}
	return &LocalAuthRepository{db: db}, nil
}

func (r *LocalAuthRepository) BootstrapManager(ctx context.Context, passwordHash []byte) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user userModel
		if err := tx.Where("username = ?", "admin").Attrs(userModel{ID: uuid.New(), Username: "admin"}).FirstOrCreate(&user).Error; err != nil {
			return fmt.Errorf("create manager user: %w", err)
		}
		var credential localCredentialModel
		if err := tx.Where("user_id = ?", user.ID).Attrs(localCredentialModel{UserID: user.ID, PasswordHash: passwordHash, MustChangePassword: true}).FirstOrCreate(&credential).Error; err != nil {
			return fmt.Errorf("create manager credential: %w", err)
		}
		var group authorizationGroupModel
		query := tx.Where("owner_kind = ? AND owner_id IS NULL AND lower(name) = lower(?)", "platform", "platform_administrators")
		if err := query.First(&group).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			group = authorizationGroupModel{ID: uuid.New(), OwnerKind: "platform", Name: "platform_administrators", SystemKey: localStringPointer("platform_administrators")}
			if err := tx.Create(&group).Error; err != nil {
				return fmt.Errorf("create manager group: %w", err)
			}
		} else if err != nil {
			return fmt.Errorf("find manager group: %w", err)
		}
		var membership authorizationMembershipModel
		membershipAttrs := authorizationMembershipModel{ID: uuid.New(), GroupID: group.ID, UserID: user.ID, Source: "manual", Active: true}
		if err := tx.Where("group_id = ? AND user_id = ? AND source = ?", group.ID, user.ID, "manual").Attrs(membershipAttrs).FirstOrCreate(&membership).Error; err != nil {
			return fmt.Errorf("create manager membership: %w", err)
		}
		roleID := uuid.MustParse(platformAdministratorRoleID)
		var binding authorizationPlatformBindingModel
		bindingAttrs := authorizationPlatformBindingModel{ID: uuid.New(), GroupID: group.ID, RoleID: roleID, Active: true}
		if err := tx.Where("group_id = ? AND role_id = ?", group.ID, roleID).Attrs(bindingAttrs).FirstOrCreate(&binding).Error; err != nil {
			return fmt.Errorf("create manager role binding: %w", err)
		}
		return nil
	})
}

func (r *LocalAuthRepository) FindLocalCredential(ctx context.Context, username string) (identity.LocalCredential, error) {
	return r.findLocalCredential(ctx, "users.username = ?", username)
}

func (r *LocalAuthRepository) FindLocalCredentialByUserID(ctx context.Context, userID uuid.UUID) (identity.LocalCredential, error) {
	return r.findLocalCredential(ctx, "users.id = ?", userID)
}

func (r *LocalAuthRepository) findLocalCredential(ctx context.Context, predicate string, value any) (identity.LocalCredential, error) {
	var row struct {
		UserID             uuid.UUID
		Username           string
		PasswordHash       []byte
		MustChangePassword bool
		DisabledAt         *time.Time
	}
	err := r.db.WithContext(ctx).Table("local_credentials").
		Select("users.id AS user_id, users.username, users.disabled_at, local_credentials.password_hash, local_credentials.must_change_password").
		Joins("JOIN users ON users.id = local_credentials.user_id").Where(predicate, value).Take(&row).Error
	if err != nil {
		return identity.LocalCredential{}, fmt.Errorf("find local credential: %w", err)
	}
	return identity.LocalCredential{UserID: row.UserID, Username: row.Username, PasswordHash: row.PasswordHash, MustChangePassword: row.MustChangePassword, Disabled: row.DisabledAt != nil}, nil
}

func (r *LocalAuthRepository) UpdatePassword(ctx context.Context, userID uuid.UUID, passwordHash []byte) error {
	result := r.db.WithContext(ctx).Model(&localCredentialModel{}).Where("user_id = ?", userID).Updates(map[string]any{"password_hash": passwordHash, "must_change_password": false, "updated_at": time.Now().UTC()})
	if result.Error != nil {
		return fmt.Errorf("update password: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("local credential not found")
	}
	return nil
}

// CreateLocalUser atomically persists an identity and its initial credential.
func (r *LocalAuthRepository) CreateLocalUser(ctx context.Context, actorID uuid.UUID, requestID, username string, passwordHash []byte) (identity.User, error) {
	user := userModel{ID: uuid.New(), Username: username}
	now := time.Now().UTC()
	payload, err := json.Marshal(map[string]any{"authenticationMethod": "local"})
	if err != nil {
		return identity.User{}, fmt.Errorf("marshal local user event: %w", err)
	}
	event, err := platform.NewEvent("identity.user.created", "user", user.ID.String(), payload, now)
	if err != nil {
		return identity.User{}, fmt.Errorf("create local user event: %w", err)
	}
	err = database.WithinTransaction(ctx, r.db, func(tx *gorm.DB) error {
		if err := tx.Create(&user).Error; err != nil {
			return err
		}
		if err := tx.Create(&localCredentialModel{UserID: user.ID, PasswordHash: passwordHash, MustChangePassword: true}).Error; err != nil {
			return err
		}
		metadata := map[string]any{"authenticationMethod": "local"}
		if err := database.AppendAudit(ctx, tx, database.AuditRecord{OccurredAt: now, ActorID: &actorID, Action: "identity.user.created", ResourceType: "user", ResourceID: user.ID.String(), RequestID: requestID, Metadata: metadata}); err != nil {
			return err
		}
		return database.AppendOutbox(ctx, tx, event)
	})
	if err != nil {
		var databaseError *pgconn.PgError
		if errors.As(err, &databaseError) && databaseError.Code == "23505" {
			return identity.User{}, fmt.Errorf("%w: %v", identityapp.ErrLocalUsernameConflict, err)
		}
		return identity.User{}, fmt.Errorf("persist local user: %w", err)
	}
	return identity.User{ID: user.ID, Username: user.Username}, nil
}

type localCredentialModel struct {
	UserID             uuid.UUID `gorm:"type:uuid;primaryKey"`
	PasswordHash       []byte
	MustChangePassword bool
}

func (localCredentialModel) TableName() string { return "local_credentials" }

type authorizationGroupModel struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey"`
	OwnerKind string
	OwnerID   *uuid.UUID
	Name      string
	SystemKey *string
}

func (authorizationGroupModel) TableName() string { return "authorization_groups" }

func localStringPointer(value string) *string { return &value }

type authorizationMembershipModel struct {
	ID      uuid.UUID `gorm:"type:uuid;primaryKey"`
	GroupID uuid.UUID
	UserID  uuid.UUID
	Source  string
	Active  bool
}

func (authorizationMembershipModel) TableName() string { return "authorization_group_memberships" }

type authorizationPlatformBindingModel struct {
	ID      uuid.UUID `gorm:"type:uuid;primaryKey"`
	GroupID uuid.UUID
	RoleID  uuid.UUID
	Active  bool
}

func (authorizationPlatformBindingModel) TableName() string {
	return "authorization_platform_role_bindings"
}
