// Package domain contains ECR digest-resolution invariants without AWS SDK types.
package domain

import (
	"errors"
	"fmt"
	"strings"

	"github.com/distribution/reference"
)

var (
	// ErrInvalidImageReference means that a target manifest does not contain a supported immutable image reference.
	ErrInvalidImageReference = errors.New("invalid image reference")
	// ErrRepositoryOutOfScope prevents a resolver from querying an unconfigured registry or repository.
	ErrRepositoryOutOfScope = errors.New("image repository is outside the configured ECR scope")
)

// RegistryScope fixes the only ECR registry and repository allow-list that ReleaseHub may query.
type RegistryScope struct {
	AccountID    string
	Region       string
	Repositories map[string]struct{}
}

// ImageReference is a validated private ECR image reference from an Argo CD target manifest.
type ImageReference struct {
	Original        string
	Registry        string
	Repository      string
	Tag             string
	RequestedDigest string
}

// NewRegistryScope creates an immutable exact-match allow-list.
func NewRegistryScope(accountID, region string, repositories []string) (RegistryScope, error) {
	accountID = strings.TrimSpace(accountID)
	region = strings.TrimSpace(region)
	if len(accountID) != 12 || region == "" {
		return RegistryScope{}, ErrRepositoryOutOfScope
	}
	for _, character := range accountID {
		if character < '0' || character > '9' {
			return RegistryScope{}, ErrRepositoryOutOfScope
		}
	}
	allowed := make(map[string]struct{}, len(repositories))
	for _, repository := range repositories {
		value := strings.TrimSpace(repository)
		if value == "" || strings.HasPrefix(value, "/") || strings.Contains(value, "..") {
			return RegistryScope{}, ErrRepositoryOutOfScope
		}
		if _, exists := allowed[value]; exists {
			return RegistryScope{}, ErrRepositoryOutOfScope
		}
		allowed[value] = struct{}{}
	}
	if len(allowed) == 0 {
		return RegistryScope{}, ErrRepositoryOutOfScope
	}
	return RegistryScope{AccountID: accountID, Region: region, Repositories: allowed}, nil
}

// ParseImageReference validates the source image against the configured ECR scope without contacting AWS.
func (s RegistryScope) ParseImageReference(value string) (ImageReference, error) {
	original := strings.TrimSpace(value)
	if original == "" {
		return ImageReference{}, ErrInvalidImageReference
	}
	parsed, err := reference.ParseAnyReference(original)
	if err != nil {
		return ImageReference{}, fmt.Errorf("%w: %v", ErrInvalidImageReference, err)
	}
	named, ok := parsed.(reference.Named)
	if !ok {
		return ImageReference{}, ErrInvalidImageReference
	}
	registry := reference.Domain(named)
	if registry != s.registryHostname() {
		return ImageReference{}, ErrRepositoryOutOfScope
	}
	repository := reference.Path(named)
	if _, allowed := s.Repositories[repository]; !allowed {
		return ImageReference{}, ErrRepositoryOutOfScope
	}
	result := ImageReference{Original: original, Registry: registry, Repository: repository}
	if tagged, hasTag := parsed.(reference.Tagged); hasTag {
		result.Tag = tagged.Tag()
	}
	if digested, hasDigest := parsed.(reference.Digested); hasDigest {
		result.RequestedDigest = digested.Digest().String()
	}
	if result.Tag == "" && result.RequestedDigest == "" {
		return ImageReference{}, ErrInvalidImageReference
	}
	return result, nil
}

func (s RegistryScope) registryHostname() string {
	return s.AccountID + ".dkr.ecr." + s.Region + ".amazonaws.com"
}
