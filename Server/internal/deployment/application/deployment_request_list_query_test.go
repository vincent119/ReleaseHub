package application

import (
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

func TestRequestListQueryValidatesLimitWithoutClamping(t *testing.T) {
	scope := requestListTestScope(t)
	tests := []struct {
		name    string
		limit   *int
		want    int
		wantErr bool
	}{
		{name: "omitted defaults to twenty", want: 20},
		{name: "minimum", limit: requestListInt(1), want: 1},
		{name: "twenty", limit: requestListInt(20), want: 20},
		{name: "fifty", limit: requestListInt(50), want: 50},
		{name: "maximum", limit: requestListInt(100), want: 100},
		{name: "negative", limit: requestListInt(-1), want: -1, wantErr: true},
		{name: "zero", limit: requestListInt(0), want: 0, wantErr: true},
		{name: "above maximum", limit: requestListInt(101), want: 101, wantErr: true},
		{name: "integer maximum", limit: requestListInt(math.MaxInt), want: math.MaxInt, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			filter, err := normalizeRequestListQuery(scope, DeploymentRequestListQuery{Limit: test.limit})
			require.Equal(t, test.want, filter.Limit)
			if test.wantErr {
				require.ErrorIs(t, err, ErrRequestQueryInvalid)
				require.NotErrorIs(t, err, ErrRequestInvalid)
				return
			}
			require.NoError(t, err)
			require.Nil(t, filter.After)
		})
	}
}

func TestRequestListQueryTrimsSearchAndCountsUnicodeCharacters(t *testing.T) {
	scope := requestListTestScope(t)
	tests := []struct {
		name    string
		search  *string
		want    string
		wantErr bool
	}{
		{name: "omitted"},
		{name: "explicit empty", search: requestListString("")},
		{name: "whitespace only", search: requestListString(" \t\n\u3000\u00a0")},
		{name: "trim preserves case and literal wildcard", search: requestListString(" \t\u3000Release 中 %_\u00a0"), want: "Release 中 %_"},
		{name: "unicode boundary", search: requestListString(strings.Repeat("界", 255)), want: strings.Repeat("界", 255)},
		{name: "ascii boundary", search: requestListString(strings.Repeat("a", 255)), want: strings.Repeat("a", 255)},
		{name: "trim before length validation", search: requestListString(" \n" + strings.Repeat("界", 255) + "\t "), want: strings.Repeat("界", 255)},
		{name: "preserves unicode form", search: requestListString("e\u0301"), want: "e\u0301"},
		{name: "unicode over boundary", search: requestListString(strings.Repeat("界", 256)), want: strings.Repeat("界", 256), wantErr: true},
		{name: "ascii over boundary", search: requestListString(strings.Repeat("a", 256)), want: strings.Repeat("a", 256), wantErr: true},
		{name: "invalid utf8", search: requestListString(string([]byte{'a', 0xff})), want: string([]byte{'a', 0xff}), wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			filter, err := normalizeRequestListQuery(scope, DeploymentRequestListQuery{Search: test.search})
			require.Equal(t, test.want, filter.Search)
			if test.wantErr {
				require.ErrorIs(t, err, ErrRequestQueryInvalid)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestRequestListQueryAcceptsOnlyExactRequestStatuses(t *testing.T) {
	scope := requestListTestScope(t)
	statuses := []deploydomain.DeploymentRequestStatus{
		deploydomain.DeploymentRequestCandidate, deploydomain.DeploymentRequestPendingReview,
		deploydomain.DeploymentRequestApproved, deploydomain.DeploymentRequestDeploying,
		deploydomain.DeploymentRequestSucceeded, deploydomain.DeploymentRequestFailed,
		deploydomain.DeploymentRequestPartialFailed, deploydomain.DeploymentRequestBlocked,
		deploydomain.DeploymentRequestSuperseded, deploydomain.DeploymentRequestTerminated,
	}
	for _, status := range statuses {
		t.Run(string(status), func(t *testing.T) {
			filter, err := normalizeRequestListQuery(scope, DeploymentRequestListQuery{Status: requestListString(string(status))})
			require.NoError(t, err)
			require.Equal(t, status, filter.Status)
		})
	}
	t.Run("omitted includes all", func(t *testing.T) {
		filter, err := normalizeRequestListQuery(scope, DeploymentRequestListQuery{})
		require.NoError(t, err)
		require.Empty(t, filter.Status)
	})
	for _, status := range []string{"", " ", "approved", "APPROVED", " Approved ", "Ready", "Unknown", "Candidate,Approved"} {
		t.Run("reject "+status, func(t *testing.T) {
			_, err := normalizeRequestListQuery(scope, DeploymentRequestListQuery{Status: requestListString(status)})
			require.ErrorIs(t, err, ErrRequestQueryInvalid)
		})
	}
}

func TestDeploymentRequestListBoundsPageAndProjectsOnlyReturnedRows(t *testing.T) {
	tests := []struct {
		name  string
		limit *int
		want  int
	}{
		{name: "omitted", want: 20},
		{name: "one", limit: requestListInt(1), want: 1},
		{name: "twenty", limit: requestListInt(20), want: 20},
		{name: "fifty", limit: requestListInt(50), want: 50},
		{name: "hundred", limit: requestListInt(100), want: 100},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scope := requestListTestScope(t)
			values := requestListTestValues(scope, test.want+1)
			invalidScheduledTime := time.Time{}
			values[test.want].ScheduledFor = &invalidScheduledTime
			repository := &requestListRepository{values: values}
			schedules := &requestListScheduleReader{}
			authorizer := requestListAllowedAuthorizer()
			service := requestListTestService(t, repository, schedules, authorizer)
			principal := RequestPrincipal{UserID: uuid.New()}
			ctx := t.Context()
			query := DeploymentRequestListQuery{Limit: test.limit, Search: requestListString(" Release "), Status: requestListString("Approved")}

			page, err := service.List(ctx, principal, scope, query)
			require.NoError(t, err)
			require.Len(t, page.Items, test.want)
			require.True(t, page.HasMore)
			require.NotEmpty(t, page.NextCursor)
			require.Equal(t, 1, repository.calls)
			require.Equal(t, scope, repository.scope)
			require.Equal(t, test.want, repository.filter.Limit)
			require.Equal(t, "Release", repository.filter.Search)
			require.Equal(t, deploydomain.DeploymentRequestApproved, repository.filter.Status)
			require.Nil(t, repository.filter.After)
			require.Equal(t, ctx, repository.ctx)
			require.Equal(t, ctx, schedules.ctx)
			require.Equal(t, ctx, authorizer.ctx)
			require.Equal(t, 1, schedules.calls)
			require.Equal(t, scope.EnvironmentID, schedules.environmentID)
			require.Equal(t, []authz.AuthorizationRequest{{UserID: principal.UserID, Permission: "deployment_request.view", Scope: scope}}, authorizer.requests)
			for index, item := range page.Items {
				require.Equal(t, values[index].ID, item.ID)
				require.Equal(t, deploydomain.DeploymentRequestScheduleReady, item.Schedule.State)
				require.Empty(t, values[index].Schedule.State)
			}
			require.Empty(t, values[test.want].Schedule.State)
			position, err := decodeRequestListCursor(&page.NextCursor, requestListFingerprint(scope, repository.filter))
			require.NoError(t, err)
			require.Equal(t, values[test.want-1].ID, position.ID)
			require.Equal(t, values[test.want-1].UpdatedAt, position.UpdatedAt)
			require.NotEqual(t, values[test.want].ID, position.ID)
		})
	}
}

func TestDeploymentRequestListReturnsEmptyAndTerminalPages(t *testing.T) {
	tests := []struct {
		name  string
		count int
	}{
		{name: "nil repository result", count: -1},
		{name: "empty repository result"},
		{name: "short page", count: 1},
		{name: "exact limit", count: 20},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scope := requestListTestScope(t)
			var values []deploydomain.DeploymentRequestSummary
			if test.count >= 0 {
				values = requestListTestValues(scope, test.count)
			}
			repository := &requestListRepository{values: values}
			service := requestListTestService(t, repository, &requestListScheduleReader{}, requestListAllowedAuthorizer())
			page, err := service.List(t.Context(), RequestPrincipal{UserID: uuid.New()}, scope, DeploymentRequestListQuery{})
			require.NoError(t, err)
			require.NotNil(t, page.Items)
			require.Len(t, page.Items, len(values))
			require.False(t, page.HasMore)
			require.Empty(t, page.NextCursor)
		})
	}
}

func TestDeploymentRequestListPassesCursorPositionToRepository(t *testing.T) {
	scope := requestListTestScope(t)
	values := requestListTestValues(scope, 3)
	repository := &requestListRepository{values: values}
	schedules := &requestListScheduleReader{}
	service := requestListTestService(t, repository, schedules, requestListAllowedAuthorizer())
	principal := RequestPrincipal{UserID: uuid.New()}
	query := DeploymentRequestListQuery{Limit: requestListInt(2), Search: requestListString(" Release "), Status: requestListString("Approved")}
	first, err := service.List(t.Context(), principal, scope, query)
	require.NoError(t, err)
	require.True(t, first.HasMore)

	repository.values = values[2:]
	query.Cursor = &first.NextCursor
	query.Search = requestListString("Release")
	second, err := service.List(t.Context(), principal, scope, query)
	require.NoError(t, err)
	require.Equal(t, &DeploymentRequestListPosition{ID: values[1].ID, UpdatedAt: values[1].UpdatedAt}, repository.filter.After)
	require.Equal(t, 2, repository.filter.Limit)
	require.Equal(t, "Release", repository.filter.Search)
	require.Equal(t, deploydomain.DeploymentRequestApproved, repository.filter.Status)
	require.Len(t, second.Items, 1)
	require.Equal(t, values[2].ID, second.Items[0].ID)
	require.False(t, second.HasMore)
	require.Empty(t, second.NextCursor)
	require.Equal(t, 2, repository.calls)
	require.Equal(t, 2, schedules.calls)
}

func TestDeploymentRequestListAuthorizesBeforeCursorValidation(t *testing.T) {
	authorizationFailure := errors.New("authorization unavailable")
	tests := []struct {
		name      string
		disabled  bool
		allowed   bool
		authError error
		want      error
	}{
		{name: "denied", want: ErrRequestForbidden},
		{name: "disabled principal", disabled: true, allowed: true, want: ErrRequestForbidden},
		{name: "authorization failure", authError: authorizationFailure, want: authorizationFailure},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scope := requestListTestScope(t)
			repository := &requestListRepository{}
			schedules := &requestListScheduleReader{}
			authorizer := &requestListAuthorizer{
				capabilityAuthorizer: capabilityAuthorizer{allowed: map[authz.Permission]bool{"deployment_request.view": test.allowed}},
				err:                  test.authError,
			}
			service := requestListTestService(t, repository, schedules, authorizer)
			principal := RequestPrincipal{UserID: uuid.New(), Disabled: test.disabled}
			page, err := service.List(t.Context(), principal, scope, DeploymentRequestListQuery{Cursor: requestListString("invalid cursor")})
			require.ErrorIs(t, err, test.want)
			require.NotErrorIs(t, err, ErrRequestQueryInvalid)
			require.Equal(t, DeploymentRequestListPage{}, page)
			require.Len(t, authorizer.requests, 1)
			require.Equal(t, principal.Disabled, authorizer.requests[0].Disabled)
			require.Equal(t, scope, authorizer.requests[0].Scope)
			require.Zero(t, repository.calls)
			require.Zero(t, schedules.calls)
		})
	}
}

func TestDeploymentRequestListRejectsInvalidQueriesBeforeRepository(t *testing.T) {
	tests := []struct {
		name  string
		query DeploymentRequestListQuery
	}{
		{name: "invalid limit", query: DeploymentRequestListQuery{Limit: requestListInt(101)}},
		{name: "invalid search", query: DeploymentRequestListQuery{Search: requestListString(strings.Repeat("界", 256))}},
		{name: "empty status", query: DeploymentRequestListQuery{Status: requestListString("")}},
		{name: "unknown status", query: DeploymentRequestListQuery{Status: requestListString("Ready")}},
		{name: "empty cursor", query: DeploymentRequestListQuery{Cursor: requestListString("")}},
		{name: "invalid cursor", query: DeploymentRequestListQuery{Cursor: requestListString("invalid cursor")}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &requestListRepository{}
			schedules := &requestListScheduleReader{}
			authorizer := requestListAllowedAuthorizer()
			service := requestListTestService(t, repository, schedules, authorizer)
			page, err := service.List(t.Context(), RequestPrincipal{UserID: uuid.New()}, requestListTestScope(t), test.query)
			require.ErrorIs(t, err, ErrRequestQueryInvalid)
			require.Equal(t, DeploymentRequestListPage{}, page)
			require.Len(t, authorizer.requests, 1)
			require.Zero(t, repository.calls)
			require.Zero(t, schedules.calls)
		})
	}
}

func TestDeploymentRequestListPropagatesRepositoryAndScheduleFailures(t *testing.T) {
	repositoryFailure := errors.New("request persistence unavailable")
	scheduleFailure := errors.New("schedule persistence unavailable")
	tests := []struct {
		name          string
		repositoryErr error
		scheduleErr   error
		invalidTime   bool
		empty         bool
		want          error
		scheduleCalls int
	}{
		{name: "repository failure", repositoryErr: repositoryFailure, want: repositoryFailure},
		{name: "schedule load failure", scheduleErr: scheduleFailure, want: scheduleFailure, scheduleCalls: 1},
		{name: "schedule load failure on empty page", scheduleErr: scheduleFailure, empty: true, want: scheduleFailure, scheduleCalls: 1},
		{name: "schedule projection failure", invalidTime: true, want: deploydomain.ErrInvalidDeploymentSchedule, scheduleCalls: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scope := requestListTestScope(t)
			values := requestListTestValues(scope, 1)
			if test.empty {
				values = nil
			}
			if test.invalidTime {
				invalidTime := time.Time{}
				values[0].ScheduledFor = &invalidTime
			}
			repository := &requestListRepository{values: values, err: test.repositoryErr}
			schedules := &requestListScheduleReader{requestScheduleReader: requestScheduleReader{err: test.scheduleErr}}
			service := requestListTestService(t, repository, schedules, requestListAllowedAuthorizer())
			_, err := service.List(t.Context(), RequestPrincipal{UserID: uuid.New()}, scope, DeploymentRequestListQuery{})
			require.ErrorIs(t, err, test.want)
			require.Equal(t, 1, repository.calls)
			require.Equal(t, test.scheduleCalls, schedules.calls)
		})
	}
}

func TestDeploymentRequestListPropagatesCursorEncodingFailure(t *testing.T) {
	scope := requestListTestScope(t)
	values := requestListTestValues(scope, 2)
	values[0].UpdatedAt = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)
	repository := &requestListRepository{values: values}
	schedules := &requestListScheduleReader{}
	service := requestListTestService(t, repository, schedules, requestListAllowedAuthorizer())
	page, err := service.List(t.Context(), RequestPrincipal{UserID: uuid.New()}, scope, DeploymentRequestListQuery{Limit: requestListInt(1)})
	require.Error(t, err)
	require.Equal(t, DeploymentRequestListPage{}, page)
	require.Equal(t, 1, repository.calls)
	require.Zero(t, schedules.calls)
}
