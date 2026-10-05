//go:build integration

package database_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
	deployinfra "github.com/vincent119/ReleaseHub/Server/internal/deployment/infrastructure"
)

type requestListIntegrationFixture struct {
	db         *gorm.DB
	ids        deploymentSchemaIDs
	scope      authz.Scope
	repository *deployinfra.DeploymentRequestRepository
	schedules  *deployinfra.DeploymentScheduleRepository
	service    *deployapp.DeploymentRequestService
	now        time.Time
}

func newRequestListIntegrationFixture(t *testing.T) requestListIntegrationFixture {
	t.Helper()
	db := workflowTestDatabase(t, t.Context())
	ids := seedDeploymentDefinitions(t, db, seedDeploymentSchemaScope(t, db))
	repository, err := deployinfra.NewDeploymentRequestRepository(db)
	if err != nil {
		t.Fatalf("create request repository: %v", err)
	}
	schedules, err := deployinfra.NewDeploymentScheduleRepository(db)
	if err != nil {
		t.Fatalf("create schedule repository: %v", err)
	}
	now := time.Date(2026, 9, 21, 8, 0, 0, 123456000, time.UTC)
	service, err := deployapp.NewDeploymentRequestService(repository, schedules, runtimeIntegrationAuthorizer{}, requestListIntegrationClock{now: now})
	if err != nil {
		t.Fatalf("create request service: %v", err)
	}
	return requestListIntegrationFixture{
		db: db, ids: ids, scope: requestListIntegrationScope(t, ids),
		repository: repository, schedules: schedules, service: service, now: now,
	}
}

func requestListIntegrationScope(t *testing.T, ids deploymentSchemaIDs) authz.Scope {
	t.Helper()
	scope, err := authz.NewEnvironmentScope(ids.organizationID, ids.projectID, ids.environmentID)
	if err != nil {
		t.Fatalf("create request scope: %v", err)
	}
	return scope
}

func (f requestListIntegrationFixture) list(t *testing.T, query deployapp.DeploymentRequestListQuery) deployapp.DeploymentRequestListPage {
	t.Helper()
	page, err := f.service.List(t.Context(), deployapp.RequestPrincipal{UserID: f.ids.userID}, f.scope, query)
	if err != nil {
		t.Fatalf("list request page: %v", err)
	}
	if page.Items == nil {
		t.Fatal("request page items must be a non-nil slice")
	}
	return page
}

type requestListIntegrationRecord struct {
	number          int
	title           string
	status          deploydomain.DeploymentRequestStatus
	aggregateStatus string
	updatedAt       time.Time
	scheduledFor    *time.Time
}

func seedRequestListIntegrationRecord(t *testing.T, db *gorm.DB, ids deploymentSchemaIDs, record requestListIntegrationRecord) uuid.UUID {
	t.Helper()
	requestID := requestListIntegrationID(record.number)
	if record.aggregateStatus == "" {
		record.aggregateStatus = "Open"
	}
	if record.title == "" {
		record.title = fmt.Sprintf("Request %03d", record.number)
	}
	if record.status == "" {
		record.status = deploydomain.DeploymentRequestCandidate
	}
	execDeploymentSQL(t, db, `
		INSERT INTO deployment_requests (
			id, organization_id, project_id, environment_id, status, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`, requestID, ids.organizationID, ids.projectID, ids.environmentID, record.aggregateStatus,
		record.updatedAt.Add(-time.Duration(record.number)*time.Hour), record.updatedAt)
	versionID := uuid.New()
	execDeploymentSQL(t, db, `
		INSERT INTO deployment_request_versions (
			id, request_id, version_number, status, fingerprint,
			workflow_version_id, plan_version_id, title, scheduled_for, source_snapshot, created_by
		) VALUES (?, ?, 1, ?, 'list-version-1', ?, ?, ?, ?, '{}', ?)
	`, versionID, requestID, record.status, ids.workflowVersionID, ids.planVersionID, record.title, record.scheduledFor, ids.userID)
	return versionID
}

func seedRequestListIntegrationLatestVersion(t *testing.T, db *gorm.DB, firstVersionID uuid.UUID, title string, status deploydomain.DeploymentRequestStatus) {
	t.Helper()
	execDeploymentSQL(t, db, `UPDATE deployment_request_versions SET status = 'Superseded' WHERE id = ?`, firstVersionID)
	execDeploymentSQL(t, db, `
		INSERT INTO deployment_request_versions (
			id, request_id, version_number, status, fingerprint,
			workflow_version_id, plan_version_id, title, scheduled_for, source_snapshot, created_by
		)
		SELECT ?, request_id, 2, ?, 'list-version-2', workflow_version_id, plan_version_id,
		       ?, scheduled_for, source_snapshot, created_by
		FROM deployment_request_versions WHERE id = ?
	`, uuid.New(), status, title, firstVersionID)
}

func requestListIntegrationID(number int) uuid.UUID {
	return uuid.MustParse(fmt.Sprintf("00000000-0000-4000-8000-%012x", number))
}

func requestListIntegrationIDs(values []deploydomain.DeploymentRequestSummary) []uuid.UUID {
	ids := make([]uuid.UUID, len(values))
	for index, value := range values {
		ids[index] = value.ID
	}
	return ids
}

func seedRequestListIntegrationScope(t *testing.T, db *gorm.DB, original deploymentSchemaIDs, differentProject, differentOrganization bool) deploymentSchemaIDs {
	t.Helper()
	ids := original
	if differentOrganization {
		ids.organizationID = uuid.New()
		execDeploymentSQL(t, db, `INSERT INTO organizations (id, name) VALUES (?, ?)`, ids.organizationID, ids.organizationID.String())
	}
	if differentProject || differentOrganization {
		ids.projectID = uuid.New()
		execDeploymentSQL(t, db, `INSERT INTO projects (id, organization_id, name) VALUES (?, ?, ?)`, ids.projectID, ids.organizationID, ids.projectID.String())
	}
	ids.environmentID = uuid.New()
	execDeploymentSQL(t, db, `
		INSERT INTO environments (id, organization_id, project_id, name, environment_type)
		VALUES (?, ?, ?, ?, 'Production')
	`, ids.environmentID, ids.organizationID, ids.projectID, ids.environmentID.String())
	return ids
}

type requestListIntegrationClock struct{ now time.Time }

func (clock requestListIntegrationClock) Now() time.Time { return clock.now }

var _ deployapp.WorkflowClock = requestListIntegrationClock{}
var _ deployapp.WorkflowAuthorizer = runtimeIntegrationAuthorizer{}
