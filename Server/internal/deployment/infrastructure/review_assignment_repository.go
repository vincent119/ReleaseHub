package infrastructure

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

// ReviewAssignmentRepository resolves role membership at review-task creation time.
type ReviewAssignmentRepository struct{ db *gorm.DB }

var _ deployapp.ReviewAssignmentResolver = (*ReviewAssignmentRepository)(nil)

// NewReviewAssignmentRepository creates the PostgreSQL assignment resolver.
func NewReviewAssignmentRepository(db *gorm.DB) (*ReviewAssignmentRepository, error) {
	if db == nil {
		return nil, errors.New("database is required")
	}
	return &ReviewAssignmentRepository{db: db}, nil
}

// Resolve snapshots direct users and active role members at Environment scope.
func (r *ReviewAssignmentRepository) Resolve(ctx context.Context, policy deploydomain.ReviewPolicy, scope authz.Scope) (deploydomain.ReviewAssignment, error) {
	assignment := deploydomain.ReviewAssignment{
		UserIDs: policy.UserIDs, RoleMembers: make(map[uuid.UUID][]uuid.UUID, len(policy.RoleIDs)),
	}
	for _, roleID := range policy.RoleIDs {
		members, err := r.roleMembers(ctx, roleID, scope)
		if err != nil {
			return deploydomain.ReviewAssignment{}, err
		}
		assignment.RoleMembers[roleID] = members
	}
	return assignment, nil
}

func (r *ReviewAssignmentRepository) roleMembers(ctx context.Context, roleID uuid.UUID, scope authz.Scope) ([]uuid.UUID, error) {
	var members []uuid.UUID
	err := r.db.WithContext(ctx).Raw(reviewRoleMembersQuery,
		roleID, scope.OrganizationID, scope.ProjectID, scope.EnvironmentID,
	).Scan(&members).Error
	if err != nil {
		return nil, fmt.Errorf("resolve workflow review role members: %w", err)
	}
	return members, nil
}

const reviewRoleMembersQuery = `
SELECT DISTINCT membership.user_id
FROM authorization_group_role_bindings binding
JOIN authorization_groups access_group ON access_group.id = binding.group_id
JOIN authorization_roles role ON role.id = binding.role_id
JOIN authorization_group_memberships membership ON membership.group_id = binding.group_id
JOIN users account ON account.id = membership.user_id
WHERE binding.role_id = ?
  AND binding.active
  AND membership.active
  AND access_group.disabled_at IS NULL
  AND role.active
  AND account.disabled_at IS NULL
  AND binding.organization_id = ?
  AND binding.project_id = ?
  AND (
    binding.scope_kind = 'project'
    OR (binding.scope_kind = 'environment' AND binding.environment_id = ?)
  )
ORDER BY membership.user_id`
