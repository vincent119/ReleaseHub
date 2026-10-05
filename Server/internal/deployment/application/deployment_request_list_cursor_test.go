package application

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

func TestRequestListCursorRoundTripsFullUTCPosition(t *testing.T) {
	scope := requestListTestScope(t)
	filter := DeploymentRequestListFilter{Limit: 20, Search: "Release 中", Status: deploydomain.DeploymentRequestApproved}
	fingerprint := requestListFingerprint(scope, filter)
	tests := []struct {
		name string
		at   time.Time
	}{
		{name: "whole second", at: time.Date(2026, 10, 1, 2, 3, 4, 0, time.UTC)},
		{name: "database microseconds", at: time.Date(2026, 10, 1, 2, 3, 4, 987654000, time.UTC)},
		{name: "full nanoseconds", at: time.Date(2026, 10, 1, 2, 3, 4, 987654321, time.UTC)},
		{name: "normalizes source offset", at: time.Date(2026, 10, 1, 10, 3, 4, 987654321, time.FixedZone("Taipei", 8*60*60))},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := deploydomain.DeploymentRequestSummary{ID: uuid.New(), UpdatedAt: test.at}
			cursor, err := encodeRequestListCursor(value, fingerprint)
			require.NoError(t, err)
			require.LessOrEqual(t, len(cursor), 1024)
			position, err := decodeRequestListCursor(&cursor, fingerprint)
			require.NoError(t, err)
			require.Equal(t, value.ID, position.ID)
			require.Equal(t, value.UpdatedAt.UTC(), position.UpdatedAt)
			require.Equal(t, value.UpdatedAt.Nanosecond(), position.UpdatedAt.Nanosecond())
			require.Equal(t, time.UTC, position.UpdatedAt.Location())
			encoded, err := base64.RawURLEncoding.Strict().DecodeString(cursor)
			require.NoError(t, err)
			var document map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(encoded, &document))
			require.Len(t, document, 4)
			require.JSONEq(t, `1`, string(document["version"]))
			require.JSONEq(t, fmt.Sprintf("%q", value.ID.String()), string(document["id"]))
			require.JSONEq(t, fmt.Sprintf("%q", value.UpdatedAt.UTC().Format(time.RFC3339Nano)), string(document["updatedAt"]))
			require.JSONEq(t, fmt.Sprintf("%q", fingerprint), string(document["context"]))
		})
	}
}

func TestRequestListCursorDistinguishesOmittedFromEmptyAndEnforcesLength(t *testing.T) {
	fingerprint := requestListFingerprint(requestListTestScope(t), DeploymentRequestListFilter{Limit: 20})
	raw := requestListTestCursorJSON(fingerprint)
	maximumCursor := base64.RawURLEncoding.EncodeToString([]byte(raw + strings.Repeat(" ", 768-len(raw))))
	require.Len(t, maximumCursor, 1024)
	tests := []struct {
		name    string
		cursor  *string
		wantErr bool
	}{
		{name: "omitted first page"},
		{name: "explicit empty", cursor: requestListString(""), wantErr: true},
		{name: "minimum length invalid document", cursor: requestListString("A"), wantErr: true},
		{name: "maximum length valid document", cursor: &maximumCursor},
		{name: "over maximum", cursor: requestListString(strings.Repeat("A", 1025)), wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			position, err := decodeRequestListCursor(test.cursor, fingerprint)
			if test.wantErr {
				require.ErrorIs(t, err, ErrRequestQueryInvalid)
				require.Nil(t, position)
				return
			}
			require.NoError(t, err)
			if test.cursor == nil {
				require.Nil(t, position)
				return
			}
			require.NotNil(t, position)
		})
	}
}

func TestRequestListCursorRejectsNoncanonicalBase64URL(t *testing.T) {
	fingerprint := requestListFingerprint(requestListTestScope(t), DeploymentRequestListFilter{Limit: 20})
	raw := requestListTestCursorJSON(fingerprint)
	for len(raw)%3 != 1 {
		raw += " "
	}
	valid := base64.RawURLEncoding.EncodeToString([]byte(raw))
	alphabet := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	last := strings.IndexByte(alphabet, valid[len(valid)-1])
	noncanonical := valid[:len(valid)-1] + string(alphabet[last|1])
	tests := []struct {
		name   string
		cursor string
	}{
		{name: "padding", cursor: valid + "=="},
		{name: "standard alphabet plus", cursor: "+" + valid[1:]},
		{name: "standard alphabet slash", cursor: "/" + valid[1:]},
		{name: "newline", cursor: valid[:4] + "\n" + valid[4:]},
		{name: "carriage return", cursor: valid[:4] + "\r" + valid[4:]},
		{name: "leading whitespace", cursor: " " + valid},
		{name: "trailing whitespace", cursor: valid + " "},
		{name: "nonzero trailing bits", cursor: noncanonical},
		{name: "invalid punctuation", cursor: "!" + valid},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			position, err := decodeRequestListCursor(&test.cursor, fingerprint)
			require.ErrorIs(t, err, ErrRequestQueryInvalid)
			require.Nil(t, position)
		})
	}
	position, err := decodeRequestListCursor(&valid, fingerprint)
	require.NoError(t, err)
	require.NotNil(t, position)
}

func TestRequestListCursorRejectsInvalidJSONSchemaAndPosition(t *testing.T) {
	fingerprint := requestListFingerprint(requestListTestScope(t), DeploymentRequestListFilter{Limit: 20})
	valid := requestListTestCursorJSON(fingerprint)
	tests := []struct {
		name string
		raw  string
	}{
		{name: "invalid json", raw: "{"},
		{name: "null document", raw: "null"},
		{name: "empty object", raw: "{}"},
		{name: "array document", raw: "[]"},
		{name: "unknown field", raw: valid[:len(valid)-1] + `,"extra":true}`},
		{name: "trailing object", raw: valid + " {}"},
		{name: "trailing null", raw: valid + " null"},
		{name: "trailing junk", raw: valid + " invalid"},
		{name: "missing version", raw: strings.Replace(valid, `"version":1,`, "", 1)},
		{name: "zero version", raw: strings.Replace(valid, `"version":1`, `"version":0`, 1)},
		{name: "unknown version", raw: strings.Replace(valid, `"version":1`, `"version":2`, 1)},
		{name: "negative version", raw: strings.Replace(valid, `"version":1`, `"version":-1`, 1)},
		{name: "wrong version type", raw: strings.Replace(valid, `"version":1`, `"version":"1"`, 1)},
		{name: "missing time", raw: strings.Replace(valid, `"updatedAt":"2026-10-01T02:03:04.987654321Z",`, "", 1)},
		{name: "zero time", raw: strings.Replace(valid, "2026-10-01T02:03:04.987654321Z", "0001-01-01T00:00:00Z", 1)},
		{name: "invalid time", raw: strings.Replace(valid, "2026-10-01T02:03:04.987654321Z", "2026-10-01", 1)},
		{name: "non utc time", raw: strings.Replace(valid, "2026-10-01T02:03:04.987654321Z", "2026-10-01T10:03:04.987654321+08:00", 1)},
		{name: "missing id", raw: strings.Replace(valid, `"id":"00000000-0000-4000-8000-000000000001",`, "", 1)},
		{name: "zero id", raw: strings.Replace(valid, "00000000-0000-4000-8000-000000000001", uuid.Nil.String(), 1)},
		{name: "invalid id", raw: strings.Replace(valid, "00000000-0000-4000-8000-000000000001", "not-a-uuid", 1)},
		{name: "missing context", raw: strings.Replace(valid, fmt.Sprintf(`,"context":%q`, fingerprint), "", 1)},
		{name: "empty context", raw: strings.Replace(valid, fingerprint, "", 1)},
		{name: "different context", raw: strings.Replace(valid, fingerprint, strings.Repeat("0", 64), 1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cursor := base64.RawURLEncoding.EncodeToString([]byte(test.raw))
			position, err := decodeRequestListCursor(&cursor, fingerprint)
			require.ErrorIs(t, err, ErrRequestQueryInvalid)
			require.Nil(t, position)
		})
	}
}

func TestRequestListCursorBindsScopeAndNormalizedQuery(t *testing.T) {
	scope := requestListTestScope(t)
	filter := DeploymentRequestListFilter{Limit: 20, Search: "Release é", Status: deploydomain.DeploymentRequestApproved}
	value := requestListTestValues(scope, 1)[0]
	cursor, err := encodeRequestListCursor(value, requestListFingerprint(scope, filter))
	require.NoError(t, err)
	organization, project, environment := scope, scope, scope
	organization.OrganizationID, project.ProjectID, environment.EnvironmentID = uuid.New(), uuid.New(), uuid.New()
	tests := []struct {
		name    string
		scope   authz.Scope
		limit   *int
		search  *string
		status  *string
		wantErr bool
	}{
		{name: "same query", scope: scope, limit: requestListInt(20), search: requestListString("Release é"), status: requestListString("Approved")},
		{name: "same trimmed search", scope: scope, search: requestListString(" \tRelease é\u3000"), status: requestListString("Approved")},
		{name: "other organization", scope: organization, search: requestListString("Release é"), status: requestListString("Approved"), wantErr: true},
		{name: "other project", scope: project, search: requestListString("Release é"), status: requestListString("Approved"), wantErr: true},
		{name: "other environment", scope: environment, search: requestListString("Release é"), status: requestListString("Approved"), wantErr: true},
		{name: "other limit", scope: scope, limit: requestListInt(50), search: requestListString("Release é"), status: requestListString("Approved"), wantErr: true},
		{name: "other search", scope: scope, search: requestListString("Other"), status: requestListString("Approved"), wantErr: true},
		{name: "different search case", scope: scope, search: requestListString("release é"), status: requestListString("Approved"), wantErr: true},
		{name: "different unicode form", scope: scope, search: requestListString("Release e\u0301"), status: requestListString("Approved"), wantErr: true},
		{name: "omitted search", scope: scope, status: requestListString("Approved"), wantErr: true},
		{name: "empty search", scope: scope, search: requestListString(""), status: requestListString("Approved"), wantErr: true},
		{name: "other status", scope: scope, search: requestListString("Release é"), status: requestListString("Failed"), wantErr: true},
		{name: "omitted status", scope: scope, search: requestListString("Release é"), wantErr: true},
		{name: "empty status", scope: scope, search: requestListString("Release é"), status: requestListString(""), wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			query := DeploymentRequestListQuery{Limit: test.limit, Cursor: &cursor, Search: test.search, Status: test.status}
			got, err := normalizeRequestListQuery(test.scope, query)
			if test.wantErr {
				require.ErrorIs(t, err, ErrRequestQueryInvalid)
				require.Nil(t, got.After)
				return
			}
			require.NoError(t, err)
			require.Equal(t, &DeploymentRequestListPosition{ID: value.ID, UpdatedAt: value.UpdatedAt}, got.After)
		})
	}
}

func TestRequestListCursorTreatsEmptySearchAsOmitted(t *testing.T) {
	scope := requestListTestScope(t)
	filter := DeploymentRequestListFilter{Limit: 20}
	value := requestListTestValues(scope, 1)[0]
	cursor, err := encodeRequestListCursor(value, requestListFingerprint(scope, filter))
	require.NoError(t, err)
	for _, search := range []*string{nil, requestListString(""), requestListString(" \t\u3000")} {
		got, err := normalizeRequestListQuery(scope, DeploymentRequestListQuery{Cursor: &cursor, Search: search})
		require.NoError(t, err)
		require.Empty(t, got.Search)
		require.Equal(t, &DeploymentRequestListPosition{ID: value.ID, UpdatedAt: value.UpdatedAt}, got.After)
	}
}

func requestListTestCursorJSON(fingerprint string) string {
	return fmt.Sprintf(`{"version":1,"updatedAt":"2026-10-01T02:03:04.987654321Z","id":"00000000-0000-4000-8000-000000000001","context":%q}`, fingerprint)
}

func FuzzRequestListCursorRejectsInvalidInputWithoutPanicking(f *testing.F) {
	fingerprint := strings.Repeat("a", 64)
	valid := base64.RawURLEncoding.EncodeToString([]byte(requestListTestCursorJSON(fingerprint)))
	for _, cursor := range []string{"", "A", "invalid cursor", valid, valid + "=", strings.Repeat("A", 1025)} {
		f.Add(cursor)
	}
	f.Fuzz(func(t *testing.T, cursor string) {
		position, err := decodeRequestListCursor(&cursor, fingerprint)
		if err != nil {
			require.ErrorIs(t, err, ErrRequestQueryInvalid)
			require.Nil(t, position)
			return
		}
		require.NotNil(t, position)
		require.NotEqual(t, uuid.Nil, position.ID)
		require.False(t, position.UpdatedAt.IsZero())
		require.Equal(t, time.UTC, position.UpdatedAt.Location())
		encoded, err := encodeRequestListCursor(deploydomain.DeploymentRequestSummary{ID: position.ID, UpdatedAt: position.UpdatedAt}, fingerprint)
		require.NoError(t, err)
		roundTrip, err := decodeRequestListCursor(&encoded, fingerprint)
		require.NoError(t, err)
		require.Equal(t, position, roundTrip)
	})
}
