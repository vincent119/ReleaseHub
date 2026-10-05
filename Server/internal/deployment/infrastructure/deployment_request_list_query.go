package infrastructure

import (
	"strings"
	"time"

	"github.com/google/uuid"

	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
)

func requestListArguments(scope authz.Scope, filter deployapp.DeploymentRequestListFilter) []any {
	// Raw requires heterogeneous bindings; no user input is interpolated into SQL.
	var updatedAt *time.Time
	var id uuid.UUID
	if filter.After != nil {
		updatedAt, id = &filter.After.UpdatedAt, filter.After.ID
	}
	pattern := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(filter.Search) + "%"
	return []any{scope.OrganizationID, scope.ProjectID, scope.EnvironmentID,
		filter.Search, pattern, string(filter.Status), string(filter.Status),
		updatedAt, updatedAt, id, filter.Limit + 1}
}
