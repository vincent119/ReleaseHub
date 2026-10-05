//go:build integration

package database_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

func TestDeploymentRequestListFiltersLatestVersionBeforeLimit(t *testing.T) {
	fixture := newRequestListIntegrationFixture(t)
	for number := 1; number <= 130; number++ {
		seedRequestListIntegrationRecord(t, fixture.db, fixture.ids, requestListIntegrationRecord{
			number: number, updatedAt: fixture.now.Add(time.Duration(number) * time.Microsecond),
		})
	}
	firstVersionID := seedRequestListIntegrationRecord(t, fixture.db, fixture.ids, requestListIntegrationRecord{
		number: 201, title: "Old matching title", status: deploydomain.DeploymentRequestSuperseded,
		aggregateStatus: "Open", updatedAt: fixture.now.Add(-time.Second),
	})
	seedRequestListIntegrationLatestVersion(t, fixture.db, firstVersionID, "全域 LiTeRaL 服務", deploydomain.DeploymentRequestFailed)
	secondVersionID := seedRequestListIntegrationRecord(t, fixture.db, fixture.ids, requestListIntegrationRecord{
		number: 202, title: "全域 LiTeRaL 舊版本", status: deploydomain.DeploymentRequestSuperseded,
		aggregateStatus: "Closed", updatedAt: fixture.now.Add(-2 * time.Second),
	})
	seedRequestListIntegrationLatestVersion(t, fixture.db, secondVersionID, "replacement", deploydomain.DeploymentRequestCandidate)
	first := fixture.list(t, deployapp.DeploymentRequestListQuery{})
	if slices.Contains(requestListIntegrationIDs(first.Items), requestListIntegrationID(201)) {
		t.Fatal("filter target must be outside the original first page")
	}

	for _, test := range []struct {
		name   string
		search string
		status string
		want   []uuid.UUID
	}{
		{name: "trimmed case-insensitive latest title", search: "  literal  ", want: []uuid.UUID{requestListIntegrationID(201)}},
		{name: "latest status beyond first hundred", status: "Failed", want: []uuid.UUID{requestListIntegrationID(201)}},
		{name: "combined filter", search: "服務", status: "Failed", want: []uuid.UUID{requestListIntegrationID(201)}},
		{name: "old title excluded", search: "Old matching title"},
		{name: "old version status excluded", search: "literal", status: "Superseded"},
		{name: "new title replaces old title", search: "replacement", status: "Candidate", want: []uuid.UUID{requestListIntegrationID(202)}},
	} {
		t.Run(test.name, func(t *testing.T) {
			query := deployapp.DeploymentRequestListQuery{}
			if test.search != "" {
				query.Search = &test.search
			}
			if test.status != "" {
				query.Status = &test.status
			}
			page := fixture.list(t, query)
			if got := requestListIntegrationIDs(page.Items); !slices.Equal(got, test.want) || page.HasMore || page.NextCursor != "" {
				t.Fatalf("filtered page = %#v, want IDs %v", page, test.want)
			}
			for _, value := range page.Items {
				if value.ActiveVersionNumber != 2 {
					t.Fatalf("filtered version = %d, want 2", value.ActiveVersionNumber)
				}
			}
		})
	}
	blank := " \t\n "
	if got := fixture.list(t, deployapp.DeploymentRequestListQuery{Search: &blank}); !slices.Equal(requestListIntegrationIDs(got.Items), requestListIntegrationIDs(first.Items)) {
		t.Fatal("blank search changed the first page")
	}
}

func TestDeploymentRequestListFiltersEveryLatestVersionStatus(t *testing.T) {
	fixture := newRequestListIntegrationFixture(t)
	statuses := []deploydomain.DeploymentRequestStatus{
		deploydomain.DeploymentRequestCandidate, deploydomain.DeploymentRequestPendingReview,
		deploydomain.DeploymentRequestApproved, deploydomain.DeploymentRequestDeploying,
		deploydomain.DeploymentRequestSucceeded, deploydomain.DeploymentRequestFailed,
		deploydomain.DeploymentRequestPartialFailed, deploydomain.DeploymentRequestBlocked,
		deploydomain.DeploymentRequestSuperseded, deploydomain.DeploymentRequestTerminated,
	}
	for index, status := range statuses {
		aggregateStatus := "Open"
		if index%2 == 0 {
			aggregateStatus = "Closed"
		}
		versionID := seedRequestListIntegrationRecord(t, fixture.db, fixture.ids, requestListIntegrationRecord{
			number: index + 1, title: "Old version", status: deploydomain.DeploymentRequestSuperseded,
			aggregateStatus: aggregateStatus, updatedAt: fixture.now,
		})
		seedRequestListIntegrationLatestVersion(t, fixture.db, versionID, string(status), status)
	}
	assertCount(t, fixture.db, "deployment_request_versions", 20)
	for index, status := range statuses {
		t.Run(string(status), func(t *testing.T) {
			filter := string(status)
			page := fixture.list(t, deployapp.DeploymentRequestListQuery{Status: &filter})
			if len(page.Items) != 1 || page.Items[0].ID != requestListIntegrationID(index+1) ||
				page.Items[0].Status != status || page.Items[0].Title != filter || page.Items[0].ActiveVersionNumber != 2 || page.HasMore {
				t.Fatalf("status page = %#v", page)
			}
		})
	}
}

func TestDeploymentRequestListTreatsSearchAsLiteralAndEnforcesScope(t *testing.T) {
	fixture := newRequestListIntegrationFixture(t)
	titles := []string{
		"literal 100% rollout", "literal 100X rollout", "literal api_name", "literal apiXname",
		`literal C:\deploy\api`, "literal C:deployapi", "literal ' OR 1=1; --", "literal normal",
		"literal 服務", strings.Repeat("字", 255), `literal slash\%_end`,
	}
	for index, title := range titles {
		seedRequestListIntegrationRecord(t, fixture.db, fixture.ids, requestListIntegrationRecord{
			number: index + 1, title: title, updatedAt: fixture.now,
		})
	}
	for index, foreign := range []deploymentSchemaIDs{
		seedRequestListIntegrationScope(t, fixture.db, fixture.ids, false, false),
		seedRequestListIntegrationScope(t, fixture.db, fixture.ids, true, false),
		seedRequestListIntegrationScope(t, fixture.db, fixture.ids, true, true),
	} {
		seedRequestListIntegrationRecord(t, fixture.db, foreign, requestListIntegrationRecord{
			number: 100 + index, title: `literal 100% api_name C:\deploy\api ' OR 1=1; -- 服務`, updatedAt: fixture.now.Add(time.Hour),
		})
	}
	assertCount(t, fixture.db, "deployment_requests", 14)
	for _, test := range []struct {
		name   string
		search string
		ids    []int
	}{
		{name: "percent", search: "%", ids: []int{11, 1}},
		{name: "underscore", search: "_", ids: []int{11, 3}},
		{name: "backslash", search: `\`, ids: []int{11, 5}},
		{name: "escaped wildcard sequence", search: `\%_`, ids: []int{11}},
		{name: "injection text matches only title", search: "' OR 1=1; --", ids: []int{7}},
		{name: "injection does not bypass predicate", search: "%' OR true --"},
		{name: "Chinese", search: "服務", ids: []int{9}},
		{name: "maximum Unicode length", search: strings.Repeat("字", 255), ids: []int{10}},
		{name: "case-insensitive path", search: `c:\DEPLOY\API`, ids: []int{5}},
	} {
		t.Run(test.name, func(t *testing.T) {
			page := fixture.list(t, deployapp.DeploymentRequestListQuery{Search: &test.search})
			expected := make([]uuid.UUID, len(test.ids))
			for index, id := range test.ids {
				expected[index] = requestListIntegrationID(id)
			}
			if got := requestListIntegrationIDs(page.Items); !slices.Equal(got, expected) || page.HasMore {
				t.Fatalf("literal search IDs = %v, want %v", got, expected)
			}
			for _, value := range page.Items {
				if value.OrganizationID != fixture.scope.OrganizationID || value.ProjectID != fixture.scope.ProjectID || value.EnvironmentID != fixture.scope.EnvironmentID {
					t.Fatalf("search returned a request outside the scope: %#v", value)
				}
			}
		})
	}
	for _, field := range []string{"organization", "project", "environment"} {
		t.Run("mismatched "+field, func(t *testing.T) {
			scope := fixture.scope
			switch field {
			case "organization":
				scope.OrganizationID = uuid.New()
			case "project":
				scope.ProjectID = uuid.New()
			case "environment":
				scope.EnvironmentID = uuid.New()
			}
			values, err := fixture.repository.List(t.Context(), scope, deployapp.DeploymentRequestListFilter{Limit: 20, Search: "%"})
			if err != nil || len(values) != 0 {
				t.Fatalf("mismatched scope returned %d requests: %v", len(values), err)
			}
		})
	}
}

func TestDeploymentRequestListRejectsCursorFromDifferentScope(t *testing.T) {
	fixture := newRequestListIntegrationFixture(t)
	for number := 1; number <= 3; number++ {
		seedRequestListIntegrationRecord(t, fixture.db, fixture.ids, requestListIntegrationRecord{number: number, updatedAt: fixture.now})
	}
	limit := 1
	first := fixture.list(t, deployapp.DeploymentRequestListQuery{Limit: &limit})
	foreign := seedRequestListIntegrationScope(t, fixture.db, fixture.ids, false, false)
	seedRequestListIntegrationRecord(t, fixture.db, foreign, requestListIntegrationRecord{number: 4, updatedAt: fixture.now})
	scope := requestListIntegrationScope(t, foreign)
	page, err := fixture.service.List(t.Context(), deployapp.RequestPrincipal{UserID: fixture.ids.userID}, scope,
		deployapp.DeploymentRequestListQuery{Limit: &limit, Cursor: &first.NextCursor})
	if !errors.Is(err, deployapp.ErrRequestQueryInvalid) || len(page.Items) != 0 {
		t.Fatalf("cross-scope cursor page = %#v, error = %v", page, err)
	}
	values, err := fixture.repository.List(t.Context(), scope, deployapp.DeploymentRequestListFilter{Limit: 1})
	if err != nil || len(values) != 1 || values[0].ID != requestListIntegrationID(4) {
		t.Fatalf("foreign scope data = %#v, error = %v", values, err)
	}
}
