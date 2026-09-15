package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	identity "github.com/vincent119/ReleaseHub/Server/internal/identity/domain"
)

// ErrInvalidLocalUser identifies an invalid local-user creation request.
var ErrInvalidLocalUser = errors.New("invalid local user input")

// ErrLocalUsernameConflict identifies a username that already belongs to any identity provider.
var ErrLocalUsernameConflict = errors.New("local username is unavailable")

// LocalCredentialRepository persists the initial manager and local password state.
type LocalCredentialRepository interface {
	BootstrapManager(context.Context, []byte) error
	FindLocalCredential(context.Context, string) (identity.LocalCredential, error)
	FindLocalCredentialByUserID(context.Context, uuid.UUID) (identity.LocalCredential, error)
	UpdatePassword(context.Context, uuid.UUID, []byte) error
	CreateLocalUser(context.Context, uuid.UUID, string, string, []byte) (identity.User, error)
}

// CreateUser atomically creates a local identity and an initial credential that must be changed on first login.
func (s *LocalAuthService) CreateUser(ctx context.Context, actorID uuid.UUID, requestID, username, password string) (identity.User, error) {
	username = strings.TrimSpace(username)
	if username == "" || len(username) > 128 || strings.ContainsAny(username, " \t\r\n") || len(password) < 8 || len(password) > 72 {
		return identity.User{}, ErrInvalidLocalUser
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return identity.User{}, fmt.Errorf("hash initial password: %w", err)
	}
	user, err := s.repository.CreateLocalUser(ctx, actorID, requestID, username, hash)
	if err != nil {
		return identity.User{}, fmt.Errorf("create local user: %w", err)
	}
	return user, nil
}

// LocalAuthService authenticates the single bootstrap manager account.
type LocalAuthService struct {
	repository LocalCredentialRepository
	sessions   *SessionService
}

func NewLocalAuthService(repository LocalCredentialRepository, sessions *SessionService) (*LocalAuthService, error) {
	if repository == nil || sessions == nil {
		return nil, fmt.Errorf("local credential repository and session service are required")
	}
	return &LocalAuthService{repository: repository, sessions: sessions}, nil
}

func (s *LocalAuthService) Bootstrap(ctx context.Context, password string) error {
	if password == "" {
		return nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash manager password: %w", err)
	}
	if err := s.repository.BootstrapManager(ctx, hash); err != nil {
		return fmt.Errorf("bootstrap manager: %w", err)
	}
	return nil
}

func (s *LocalAuthService) Login(ctx context.Context, username, password string) (identity.User, bool, identity.Session, string, string, error) {
	username = strings.TrimSpace(username)
	if username == "" || len(username) > 128 || password == "" || len(password) > 72 {
		return identity.User{}, false, identity.Session{}, "", "", fmt.Errorf("invalid local credentials")
	}
	credential, err := s.repository.FindLocalCredential(ctx, username)
	if err != nil || credential.Disabled || bcrypt.CompareHashAndPassword(credential.PasswordHash, []byte(password)) != nil {
		return identity.User{}, false, identity.Session{}, "", "", fmt.Errorf("invalid local credentials")
	}
	session, sessionToken, csrfToken, err := s.sessions.CreateLocal(ctx, credential.UserID)
	if err != nil {
		return identity.User{}, false, identity.Session{}, "", "", err
	}
	return identity.User{ID: credential.UserID, Username: credential.Username}, credential.MustChangePassword, session, sessionToken, csrfToken, nil
}

func (s *LocalAuthService) ChangePassword(ctx context.Context, userID uuid.UUID, currentPassword, newPassword string) error {
	if currentPassword == "" || len(currentPassword) > 72 || len(newPassword) < 8 || len(newPassword) > 72 {
		return fmt.Errorf("password length is invalid")
	}
	credential, err := s.repository.FindLocalCredentialByUserID(ctx, userID)
	if err != nil || bcrypt.CompareHashAndPassword(credential.PasswordHash, []byte(currentPassword)) != nil {
		return fmt.Errorf("current password is invalid")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash new password: %w", err)
	}
	if err := s.repository.UpdatePassword(ctx, userID, hash); err != nil {
		return fmt.Errorf("update local password: %w", err)
	}
	return s.sessions.RevokeUserSessions(ctx, userID)
}

func (s *LocalAuthService) MustChangePassword(ctx context.Context, userID uuid.UUID) (bool, error) {
	credential, err := s.repository.FindLocalCredentialByUserID(ctx, userID)
	if err != nil {
		return false, err
	}
	return credential.MustChangePassword, nil
}
