package domain

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

var permissionPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)

// Permission identifies one operation that can be composed into a role.
type Permission string

// NewPermission validates and normalizes a permission key.
func NewPermission(value string) (Permission, error) {
	value = strings.TrimSpace(value)
	if !permissionPattern.MatchString(value) {
		return "", fmt.Errorf("permission must contain at least one dot-separated lowercase segment")
	}
	return Permission(value), nil
}

// AuthorizationRequest is the framework-independent policy evaluation input.
type AuthorizationRequest struct {
	UserID     uuid.UUID
	Disabled   bool
	Permission Permission
	Scope      Scope
}

// Validate rejects incomplete policy evaluation input before an adapter is called.
func (r AuthorizationRequest) Validate() error {
	if r.UserID == uuid.Nil {
		return fmt.Errorf("user ID is required")
	}
	if _, err := NewPermission(string(r.Permission)); err != nil {
		return err
	}
	_, err := newScope(r.Scope.Kind, r.Scope.OrganizationID, r.Scope.ProjectID, r.Scope.EnvironmentID, r.Scope.ApplicationID)
	return err
}
