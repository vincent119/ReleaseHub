package domain

import (
	"errors"
	"testing"
)

func TestRegistryScopeParsesOnlyConfiguredECRImages(t *testing.T) {
	t.Parallel()
	scope := mustRegistryScope(t)
	tests := []struct {
		name       string
		reference  string
		wantTag    string
		wantDigest string
		wantError  error
	}{
		{name: "tag", reference: "123456789012.dkr.ecr.ap-northeast-1.amazonaws.com/platform/api:v1.2.3", wantTag: "v1.2.3"},
		{name: "tag and digest", reference: "123456789012.dkr.ecr.ap-northeast-1.amazonaws.com/platform/api:v1.2.3@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", wantTag: "v1.2.3", wantDigest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		{name: "digest", reference: "123456789012.dkr.ecr.ap-northeast-1.amazonaws.com/platform/api@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", wantDigest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		{name: "other registry", reference: "docker.io/library/nginx:1.27", wantError: ErrRepositoryOutOfScope},
		{name: "other repository", reference: "123456789012.dkr.ecr.ap-northeast-1.amazonaws.com/platform/worker:v1", wantError: ErrRepositoryOutOfScope},
		{name: "invalid", reference: "not a reference", wantError: ErrInvalidImageReference},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value, err := scope.ParseImageReference(test.reference)
			if !errors.Is(err, test.wantError) {
				t.Fatalf("ParseImageReference() error = %v, want %v", err, test.wantError)
			}
			if test.wantError == nil && (value.Tag != test.wantTag || value.RequestedDigest != test.wantDigest) {
				t.Fatalf("ParseImageReference() = %#v", value)
			}
		})
	}
}

func mustRegistryScope(t *testing.T) RegistryScope {
	t.Helper()
	value, err := NewRegistryScope("123456789012", "ap-northeast-1", []string{"platform/api"})
	if err != nil {
		t.Fatalf("NewRegistryScope() error = %v", err)
	}
	return value
}
