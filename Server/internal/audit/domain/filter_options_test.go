package domain

import (
	"errors"
	"testing"
	"time"

	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
)

func TestFilterOptionFilterNormalize(t *testing.T) {
	now := time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC)
	filter, err := (FilterOptionFilter{Scope: authz.NewPlatformScope(), Field: FilterOptionActor, Search: " admin "}).Normalize(now)
	if err != nil {
		t.Fatal(err)
	}
	if filter.Search != "admin" || filter.Limit != DefaultLimit || filter.OccurredTo != now || filter.OccurredFrom != now.Add(-DefaultQueryRange) {
		t.Fatalf("normalized filter = %#v", filter)
	}
	for _, invalid := range []FilterOptionFilter{
		{Scope: authz.NewPlatformScope(), Field: "metadata"},
		{Scope: authz.NewPlatformScope(), Field: FilterOptionAction, Limit: MaximumFilterOptionLimit + 1},
		{Scope: authz.NewPlatformScope(), Field: FilterOptionAction, Search: string(make([]byte, 256))},
	} {
		if _, err := invalid.Normalize(now); !errors.Is(err, ErrInvalidFilter) {
			t.Fatalf("invalid filter %#v error = %v", invalid, err)
		}
	}
}
