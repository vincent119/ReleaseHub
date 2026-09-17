package domain

import (
	"strings"
	"time"

	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
)

const MaximumFilterOptionLimit = 50

// FilterOptionField is one allowlisted Audit Trail autocomplete dimension.
type FilterOptionField string

const (
	FilterOptionAction       FilterOptionField = "action"
	FilterOptionResourceType FilterOptionField = "resourceType"
	FilterOptionActor        FilterOptionField = "actor"
)

// FilterOptionFilter bounds an authorized autocomplete query.
type FilterOptionFilter struct {
	Scope        authz.Scope
	OccurredFrom time.Time
	OccurredTo   time.Time
	Field        FilterOptionField
	Search       string
	Limit        int
}

// Normalize applies the same time bounds as event queries and validates option-specific limits.
func (f FilterOptionFilter) Normalize(now time.Time) (FilterOptionFilter, error) {
	base, err := (QueryFilter{
		Scope: f.Scope, OccurredFrom: f.OccurredFrom, OccurredTo: f.OccurredTo, Limit: DefaultLimit,
	}).Normalize(now)
	if err != nil {
		return FilterOptionFilter{}, err
	}
	f.OccurredFrom, f.OccurredTo = base.OccurredFrom, base.OccurredTo
	switch f.Field {
	case FilterOptionAction, FilterOptionResourceType, FilterOptionActor:
	default:
		return FilterOptionFilter{}, ErrInvalidFilter
	}
	f.Search = strings.TrimSpace(f.Search)
	if len(f.Search) > 255 {
		return FilterOptionFilter{}, ErrInvalidFilter
	}
	if f.Limit == 0 {
		f.Limit = DefaultLimit
	}
	if f.Limit < 1 || f.Limit > MaximumFilterOptionLimit {
		return FilterOptionFilter{}, ErrInvalidFilter
	}
	return f, nil
}
