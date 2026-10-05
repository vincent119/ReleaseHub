//go:build integration

package database_test

import (
	"encoding/base64"
	"encoding/json"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
)

func TestDeploymentRequestListTraversesAllRowsWithStableCursor(t *testing.T) {
	fixture := newRequestListIntegrationFixture(t)
	const count = 137
	expected := make([]uuid.UUID, count)
	for number := 1; number <= count; number++ {
		seedRequestListIntegrationRecord(t, fixture.db, fixture.ids, requestListIntegrationRecord{
			number: number, updatedAt: fixture.now.Add(time.Duration(number/5) * time.Microsecond),
		})
		expected[count-number] = requestListIntegrationID(number)
	}
	if got := tableCount(t, fixture.db, "deployment_requests"); got != count {
		t.Fatalf("persisted request count = %d, want %d", got, count)
	}

	for _, limit := range []int{20, 50, 100} {
		t.Run(strconv.Itoa(limit), func(t *testing.T) {
			lookAhead, err := fixture.repository.List(t.Context(), fixture.scope, deployapp.DeploymentRequestListFilter{Limit: limit})
			if err != nil || len(lookAhead) != limit+1 {
				t.Fatalf("repository look-ahead count = %d, error = %v", len(lookAhead), err)
			}
			for index, item := range lookAhead {
				updatedAt := fixture.now.Add(time.Duration((count-index)/5) * time.Microsecond)
				if !item.UpdatedAt.Equal(updatedAt) {
					t.Fatalf("updated time = %s, want %s", item.UpdatedAt, updatedAt)
				}
			}
			assertRequestListIntegrationTraversal(t, fixture, deployapp.DeploymentRequestListQuery{Limit: &limit}, expected, limit)
		})
	}
	t.Run("default", func(t *testing.T) {
		assertRequestListIntegrationTraversal(t, fixture, deployapp.DeploymentRequestListQuery{}, expected, 20)
	})
}

func assertRequestListIntegrationTraversal(t *testing.T, fixture requestListIntegrationFixture, query deployapp.DeploymentRequestListQuery, expected []uuid.UUID, limit int) {
	t.Helper()
	seen := make(map[uuid.UUID]bool, len(expected))
	seenCursors := make(map[string]bool)
	for offset := 0; offset < len(expected); {
		page := fixture.list(t, query)
		end := min(offset+limit, len(expected))
		if got := requestListIntegrationIDs(page.Items); !slices.Equal(got, expected[offset:end]) {
			t.Fatalf("page at %d = %v, want %v", offset, got, expected[offset:end])
		}
		for _, item := range page.Items {
			if seen[item.ID] {
				t.Fatalf("request %s was returned twice", item.ID)
			}
			seen[item.ID] = true
			if item.UpdatedAt.Nanosecond()%1000 != 0 {
				t.Fatalf("request updated time lost microsecond precision: %s", item.UpdatedAt)
			}
		}
		offset = end
		if page.HasMore != (offset < len(expected)) || (page.NextCursor != "") != page.HasMore {
			t.Fatalf("page at %d has inconsistent cursor metadata: %#v", offset, page)
		}
		if page.HasMore {
			assertRequestListIntegrationCursorPosition(t, page)
			if seenCursors[page.NextCursor] {
				t.Fatal("cursor did not advance")
			}
			seenCursors[page.NextCursor] = true
			cursor := page.NextCursor
			query.Cursor = &cursor
		}
	}
	if len(seen) != len(expected) {
		t.Fatalf("traversal count = %d, want %d", len(seen), len(expected))
	}
}

func assertRequestListIntegrationCursorPosition(t *testing.T, page deployapp.DeploymentRequestListPage) {
	t.Helper()
	encoded, err := base64.RawURLEncoding.DecodeString(page.NextCursor)
	if err != nil {
		t.Fatalf("decode server cursor: %v", err)
	}
	var position struct {
		UpdatedAt time.Time `json:"updatedAt"`
		ID        uuid.UUID `json:"id"`
	}
	if err := json.Unmarshal(encoded, &position); err != nil {
		t.Fatalf("read server cursor: %v", err)
	}
	boundary := page.Items[len(page.Items)-1]
	_, offset := position.UpdatedAt.Zone()
	if offset != 0 || position.ID != boundary.ID || !position.UpdatedAt.Equal(boundary.UpdatedAt) {
		t.Fatalf("cursor position = %#v, want last returned row %#v in UTC", position, boundary)
	}
}

func TestDeploymentRequestListPageEdges(t *testing.T) {
	for _, test := range []struct {
		name  string
		count int
		limit int
	}{
		{name: "empty", count: 0, limit: 20},
		{name: "one", count: 1, limit: 1},
		{name: "minimum limit", count: 3, limit: 1},
		{name: "exact page", count: 20, limit: 20},
		{name: "one look-ahead", count: 21, limit: 20},
		{name: "exact final page", count: 40, limit: 20},
		{name: "exact maximum", count: 100, limit: 100},
		{name: "maximum look-ahead", count: 101, limit: 100},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRequestListIntegrationFixture(t)
			expected := make([]uuid.UUID, test.count)
			for number := 1; number <= test.count; number++ {
				seedRequestListIntegrationRecord(t, fixture.db, fixture.ids, requestListIntegrationRecord{number: number, updatedAt: fixture.now})
				expected[test.count-number] = requestListIntegrationID(number)
			}
			query := deployapp.DeploymentRequestListQuery{Limit: &test.limit}
			if test.count == 0 {
				page := fixture.list(t, query)
				if len(page.Items) != 0 || page.HasMore || page.NextCursor != "" {
					t.Fatalf("empty page = %#v", page)
				}
				return
			}
			assertRequestListIntegrationTraversal(t, fixture, query, expected, test.limit)
		})
	}
}

func TestDeploymentRequestListCursorSurvivesDeletedBoundaryRow(t *testing.T) {
	fixture := newRequestListIntegrationFixture(t)
	for number := 1; number <= 45; number++ {
		seedRequestListIntegrationRecord(t, fixture.db, fixture.ids, requestListIntegrationRecord{number: number, updatedAt: fixture.now})
	}
	first := fixture.list(t, deployapp.DeploymentRequestListQuery{})
	if len(first.Items) != 20 || !first.HasMore || first.Items[19].ID != requestListIntegrationID(26) {
		t.Fatalf("first page boundary = %#v", first)
	}
	boundary := first.Items[19].ID
	execDeploymentSQL(t, fixture.db, `DELETE FROM deployment_request_versions WHERE request_id = ?`, boundary)
	execDeploymentSQL(t, fixture.db, `DELETE FROM deployment_requests WHERE id = ?`, boundary)
	assertCountWhere(t, fixture.db, "deployment_requests", "id = ?", boundary, 0)
	expected := make([]uuid.UUID, 25)
	for number := 1; number <= 25; number++ {
		expected[25-number] = requestListIntegrationID(number)
	}
	assertRequestListIntegrationTraversal(t, fixture, deployapp.DeploymentRequestListQuery{Cursor: &first.NextCursor}, expected, 20)
}
