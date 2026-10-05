package application

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

// ErrRequestQueryInvalid distinguishes list validation from metadata validation.
var ErrRequestQueryInvalid = errors.New("deployment request query is invalid")

// DeploymentRequestListQuery preserves omitted and explicitly empty query values.
type DeploymentRequestListQuery struct {
	Limit  *int
	Cursor *string
	Search *string
	Status *string
}

// DeploymentRequestListPosition is the authorized query's exclusive keyset boundary.
type DeploymentRequestListPosition struct {
	UpdatedAt time.Time
	ID        uuid.UUID
}

// DeploymentRequestListFilter carries validated criteria to the persistence port.
type DeploymentRequestListFilter struct {
	Limit  int
	Search string
	Status deploydomain.DeploymentRequestStatus
	After  *DeploymentRequestListPosition
}

// DeploymentRequestListPage contains a bounded page without an invented total count.
type DeploymentRequestListPage struct {
	Items      []deploydomain.DeploymentRequestSummary
	NextCursor string
	HasMore    bool
}

func normalizeRequestListQuery(scope authz.Scope, query DeploymentRequestListQuery) (DeploymentRequestListFilter, error) {
	filter := DeploymentRequestListFilter{Limit: 20}
	if query.Limit != nil {
		filter.Limit = *query.Limit
	}
	if query.Search != nil {
		filter.Search = strings.TrimSpace(*query.Search)
	}
	if query.Status != nil {
		filter.Status = deploydomain.DeploymentRequestStatus(*query.Status)
	}
	if err := validateRequestListFilter(filter, query.Status != nil); err != nil {
		return filter, err
	}
	position, err := decodeRequestListCursor(query.Cursor, requestListFingerprint(scope, filter))
	filter.After = position
	return filter, err
}

func validateRequestListFilter(filter DeploymentRequestListFilter, statusProvided bool) error {
	if filter.Limit < 1 || filter.Limit > 100 || !utf8.ValidString(filter.Search) || utf8.RuneCountInString(filter.Search) > 255 {
		return ErrRequestQueryInvalid
	}
	if statusProvided && !validRequestListStatus(filter.Status) {
		return ErrRequestQueryInvalid
	}
	return nil
}

func validRequestListStatus(value deploydomain.DeploymentRequestStatus) bool {
	switch value {
	case deploydomain.DeploymentRequestCandidate, deploydomain.DeploymentRequestPendingReview,
		deploydomain.DeploymentRequestApproved, deploydomain.DeploymentRequestDeploying,
		deploydomain.DeploymentRequestSucceeded, deploydomain.DeploymentRequestFailed,
		deploydomain.DeploymentRequestPartialFailed, deploydomain.DeploymentRequestBlocked,
		deploydomain.DeploymentRequestSuperseded, deploydomain.DeploymentRequestTerminated:
		return true
	default:
		return false
	}
}

func requestListPage(scope authz.Scope, filter DeploymentRequestListFilter, values []deploydomain.DeploymentRequestSummary) (DeploymentRequestListPage, error) {
	page := DeploymentRequestListPage{HasMore: len(values) > filter.Limit}
	if page.HasMore {
		values = values[:filter.Limit]
	}
	page.Items = make([]deploydomain.DeploymentRequestSummary, len(values))
	copy(page.Items, values)
	if !page.HasMore {
		return page, nil
	}
	cursor, err := encodeRequestListCursor(values[len(values)-1], requestListFingerprint(scope, filter))
	page.NextCursor = cursor
	return page, err
}
