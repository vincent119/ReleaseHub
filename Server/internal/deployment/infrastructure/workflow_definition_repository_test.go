package infrastructure

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
)

func TestWorkflowCreateErrorMapsOnlyNameConstraint(t *testing.T) {
	tests := []struct {
		name       string
		constraint string
		want       error
	}{
		{name: "workflow name", constraint: "release_workflows_name_idx", want: deployapp.ErrWorkflowNameConflict},
		{name: "other unique constraint", constraint: "release_workflow_versions_workflow_id_version_number_key", want: deployapp.ErrWorkflowConflict},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := workflowCreateError(&pgconn.PgError{Code: "23505", ConstraintName: test.constraint})
			if !errors.Is(err, test.want) {
				t.Fatalf("workflow create error = %v, want %v", err, test.want)
			}
		})
	}
}
