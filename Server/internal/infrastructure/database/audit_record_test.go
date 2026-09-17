package database

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestValidateAuditMetadataRejectsSensitiveAndOversizedValues(t *testing.T) {
	deep := map[string]any{"value": "safe"}
	for range maxAuditMetadataDepth + 1 {
		deep = map[string]any{"nested": deep}
	}
	manyKeys := make(map[string]any, maxAuditMetadataKeys+1)
	for index := range maxAuditMetadataKeys + 1 {
		manyKeys[uuid.NewString()] = index
	}
	tests := []struct {
		name     string
		metadata map[string]any
	}{
		{name: "password", metadata: map[string]any{"password": "sensitive"}},
		{name: "mixed case token", metadata: map[string]any{"Access-Token": "sensitive"}},
		{name: "separator variant secret", metadata: map[string]any{"client.secret": "sensitive"}},
		{name: "authorization header", metadata: map[string]any{"Authorization_Header": "sensitive"}},
		{name: "nested cookie", metadata: map[string]any{"request": map[string]any{"session_cookie": "sensitive"}}},
		{name: "nested too deeply", metadata: deep},
		{name: "too many keys", metadata: manyKeys},
		{name: "long string", metadata: map[string]any{"reason": strings.Repeat("x", maxAuditStringBytes+1)}},
		{name: "large array", metadata: map[string]any{"values": make([]any, maxAuditArrayItems+1)}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateAuditMetadata(test.metadata, 1); err == nil {
				t.Fatal("unsafe metadata should be rejected")
			}
		})
	}
}

func TestValidateAuditMetadataAcceptsBoundedEvidence(t *testing.T) {
	metadata := map[string]any{
		"requestId": "request-1",
		"result": map[string]any{
			"authorized": true,
			"count":      2,
		},
		"resources": []any{"application-1", "application-2"},
	}
	if err := validateAuditMetadata(metadata, 1); err != nil {
		t.Fatalf("validate safe metadata: %v", err)
	}
}

func TestValidateAuditScopeRequiresCompleteAncestry(t *testing.T) {
	applicationID, environmentID, projectID, organizationID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	tests := []struct {
		name   string
		record AuditRecord
		valid  bool
	}{
		{name: "resolved application", valid: true, record: AuditRecord{
			ScopeResolution: AuditScopeResolved, OrganizationID: &organizationID, ProjectID: &projectID,
			EnvironmentID: &environmentID, ApplicationID: &applicationID,
		}},
		{name: "application without environment", record: AuditRecord{
			ScopeResolution: AuditScopeResolved, OrganizationID: &organizationID, ProjectID: &projectID,
			ApplicationID: &applicationID,
		}},
		{name: "unresolved with ancestry", record: AuditRecord{
			ScopeResolution: AuditScopeUnresolved, OrganizationID: &organizationID,
		}},
		{name: "platform resolved", valid: true, record: AuditRecord{ScopeResolution: AuditScopeResolved}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateAuditScope(test.record)
			if test.valid && err != nil {
				t.Fatalf("validate scope: %v", err)
			}
			if !test.valid && err == nil {
				t.Fatal("invalid scope should be rejected")
			}
		})
	}
}
