package application

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
)

// OIDCGroupRepository replaces provider-derived memberships using configured mappings only.
type OIDCGroupRepository interface {
	SyncOIDCGroups(context.Context, uuid.UUID, string, []string) error
}

// OIDCGroupService synchronizes the normalized releasehub_group claim.
type OIDCGroupService struct{ repository OIDCGroupRepository }

// NewOIDCGroupService creates the OIDC group synchronization use case.
func NewOIDCGroupService(repository OIDCGroupRepository) (*OIDCGroupService, error) {
	if repository == nil {
		return nil, fmt.Errorf("OIDC group repository is required")
	}
	return &OIDCGroupService{repository: repository}, nil
}

// SyncOIDCGroups removes stale provider memberships and maps only known viewer groups.
func (s *OIDCGroupService) SyncOIDCGroups(ctx context.Context, userID uuid.UUID, issuer string, providerGroups []string) error {
	if userID == uuid.Nil || strings.TrimSpace(issuer) == "" {
		return fmt.Errorf("user ID and issuer are required")
	}
	groups := make([]string, 0, len(providerGroups))
	seen := make(map[string]struct{}, len(providerGroups))
	for _, group := range providerGroups {
		group = strings.TrimSpace(group)
		if group == "" {
			continue
		}
		if _, exists := seen[group]; exists {
			continue
		}
		seen[group] = struct{}{}
		groups = append(groups, group)
	}
	slices.Sort(groups)
	if err := s.repository.SyncOIDCGroups(ctx, userID, strings.TrimSpace(issuer), groups); err != nil {
		return fmt.Errorf("synchronize OIDC groups: %w", err)
	}
	return nil
}
