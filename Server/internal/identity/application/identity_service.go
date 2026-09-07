package application

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	identity "github.com/vincent119/ReleaseHub/Server/internal/identity/domain"
)

// IdentityRepository resolves immutable OIDC identity bindings.
type IdentityRepository interface {
	FindOrCreateIdentity(context.Context, string, string, string) (identity.User, error)
	FindUser(context.Context, uuid.UUID) (identity.User, error)
	FindUserByIdentity(context.Context, string, string) (identity.User, error)
}

func (s *IdentityService) FindUserByIdentity(ctx context.Context, issuer, subject string) (identity.User, error) {
	user, err := s.repository.FindUserByIdentity(ctx, issuer, subject)
	if err != nil {
		return identity.User{}, fmt.Errorf("find OIDC identity: %w", err)
	}
	return user, nil
}

// FindUser returns the current local account state for an authenticated session.
func (s *IdentityService) FindUser(ctx context.Context, userID uuid.UUID) (identity.User, error) {
	user, err := s.repository.FindUser(ctx, userID)
	if err != nil {
		return identity.User{}, fmt.Errorf("find user: %w", err)
	}
	if user.Disabled {
		return identity.User{}, fmt.Errorf("user is disabled")
	}
	return user, nil
}

// IdentityService validates OIDC claims before persistence.
type IdentityService struct{ repository IdentityRepository }

// NewIdentityService creates an identity service.
func NewIdentityService(repository IdentityRepository) (*IdentityService, error) {
	if repository == nil {
		return nil, fmt.Errorf("identity repository is required")
	}
	return &IdentityService{repository: repository}, nil
}

// ResolveOIDCIdentity returns an existing identity or creates one without rewriting usernames.
func (s *IdentityService) ResolveOIDCIdentity(ctx context.Context, issuer, subject, username string) (identity.User, error) {
	issuer, subject, username = strings.TrimSpace(issuer), strings.TrimSpace(subject), strings.TrimSpace(username)
	if issuer == "" || subject == "" || username == "" {
		return identity.User{}, fmt.Errorf("issuer, subject, and username are required")
	}
	user, err := s.repository.FindOrCreateIdentity(ctx, issuer, subject, username)
	if err != nil {
		return identity.User{}, fmt.Errorf("resolve OIDC identity: %w", err)
	}
	if user.Disabled {
		return identity.User{}, fmt.Errorf("user is disabled")
	}
	return user, nil
}
