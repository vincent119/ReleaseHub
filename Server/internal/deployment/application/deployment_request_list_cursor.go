package application

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"

	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

type requestListCursor struct {
	Version   int       `json:"version"`
	UpdatedAt time.Time `json:"updatedAt"`
	ID        uuid.UUID `json:"id"`
	Context   string    `json:"context"`
}

func requestListFingerprint(scope authz.Scope, filter DeploymentRequestListFilter) string {
	// The length-prefixed search prevents user text from colliding with context separators.
	value := fmt.Appendf(nil, "updatedAt-id-desc-v1/%s/%s/%s/%d/%s/%d:%s",
		scope.OrganizationID, scope.ProjectID, scope.EnvironmentID, filter.Limit,
		filter.Status, len(filter.Search), filter.Search)
	fingerprint := sha256.Sum256(value)
	return hex.EncodeToString(fingerprint[:])
}

func encodeRequestListCursor(value deploydomain.DeploymentRequestSummary, fingerprint string) (string, error) {
	cursor := requestListCursor{Version: 1, UpdatedAt: value.UpdatedAt.UTC(), ID: value.ID, Context: fingerprint}
	encoded, err := json.Marshal(cursor)
	if err != nil {
		return "", fmt.Errorf("encode deployment request cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(encoded), nil
}

func decodeRequestListCursor(value *string, fingerprint string) (*DeploymentRequestListPosition, error) {
	if value == nil {
		return nil, nil
	}
	if len(*value) < 1 || len(*value) > 1024 {
		return nil, ErrRequestQueryInvalid
	}
	encoded, err := base64.RawURLEncoding.Strict().DecodeString(*value)
	if err != nil || base64.RawURLEncoding.EncodeToString(encoded) != *value {
		return nil, ErrRequestQueryInvalid
	}
	cursor, err := readRequestListCursor(encoded)
	if err != nil || !validRequestListCursor(cursor, fingerprint) {
		return nil, ErrRequestQueryInvalid
	}
	return &DeploymentRequestListPosition{UpdatedAt: cursor.UpdatedAt.UTC(), ID: cursor.ID}, nil
}

func readRequestListCursor(encoded []byte) (requestListCursor, error) {
	var cursor requestListCursor
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cursor); err != nil {
		return cursor, err
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		return cursor, ErrRequestQueryInvalid
	}
	return cursor, nil
}

func validRequestListCursor(cursor requestListCursor, fingerprint string) bool {
	_, offset := cursor.UpdatedAt.Zone()
	return cursor.Version == 1 && cursor.Context == fingerprint && cursor.ID != uuid.Nil &&
		!cursor.UpdatedAt.IsZero() && offset == 0
}
