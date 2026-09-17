package domain

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
)

func TestQueryFilterNormalize(t *testing.T) {
	now := time.Date(2026, 9, 17, 8, 0, 0, 0, time.FixedZone("test", 8*60*60))
	project, err := authz.NewProjectScope(uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("create scope: %v", err)
	}
	filter, err := (QueryFilter{Scope: project, Actor: " Admin ", Limit: 0}).Normalize(now)
	if err != nil {
		t.Fatalf("normalize filter: %v", err)
	}
	if filter.Limit != DefaultLimit || filter.Actor != "Admin" || filter.OccurredTo.Location() != time.UTC || filter.OccurredTo.Sub(filter.OccurredFrom) != DefaultQueryRange {
		t.Fatalf("normalized filter = %#v", filter)
	}
	invalid := []QueryFilter{
		{Scope: project, OccurredFrom: now, OccurredTo: now},
		{Scope: project, OccurredFrom: now.Add(-MaximumQueryRange - time.Second), OccurredTo: now},
		{Scope: project, Limit: MaximumLimit + 1},
		{Scope: authz.Scope{Kind: authz.ScopeProject}, Limit: 1},
	}
	for _, candidate := range invalid {
		if _, normalizeErr := candidate.Normalize(now); !errors.Is(normalizeErr, ErrInvalidFilter) {
			t.Fatalf("normalize error = %v for %#v", normalizeErr, candidate)
		}
	}
}

func TestQueryFilterFingerprintIncludesFiltersNotLimit(t *testing.T) {
	now := time.Now().UTC()
	base, err := (QueryFilter{Scope: authz.NewPlatformScope(), Action: "workflow.published", Limit: 20}).Normalize(now)
	if err != nil {
		t.Fatal(err)
	}
	withLimit := base
	withLimit.Limit = 100
	if base.Fingerprint() != withLimit.Fingerprint() {
		t.Fatal("limit should not change cursor fingerprint")
	}
	changed := base
	changed.Action = "workflow.created"
	if base.Fingerprint() == changed.Fingerprint() {
		t.Fatal("action should change cursor fingerprint")
	}
}
