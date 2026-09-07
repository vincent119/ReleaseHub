package domain

import "github.com/google/uuid"

// User is the local account identified by immutable OIDC identity bindings.
type User struct {
	ID       uuid.UUID
	Username string
	Disabled bool
}

// OIDCIdentity is the provider identity binding for one local user.
type OIDCIdentity struct {
	UserID  uuid.UUID
	Issuer  string
	Subject string
}
