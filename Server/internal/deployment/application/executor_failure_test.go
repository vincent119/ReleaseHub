package application

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	argodomain "github.com/vincent119/ReleaseHub/Server/internal/argocd/domain"
)

func TestDeploymentExecutorFailurePreservesObservationAndCause(t *testing.T) {
	for _, observed := range []bool{false, true} {
		t.Run(map[bool]string{false: "without observation", true: "revision mismatch"}[observed], func(t *testing.T) {
			executor, _, repository := newExecutorFixture(t, "sha256:approved")
			execution := nodeExecution{executionID: repository.snapshot.ID, target: repository.snapshot.Targets[0], operationID: "operation-1"}
			occurredAt := time.Date(2026, 10, 2, 12, 0, 0, 0, time.FixedZone("test", 8*60*60))
			executor.now = func() time.Time { return occurredAt }
			cause := errors.New("node failure")
			application := argodomain.Application{SyncStatus: "Synced", HealthStatus: "Healthy", ResolvedRevision: "observed-revision"}
			code := "watch_failed"
			var err error
			if observed {
				code = "target_revision_mismatch"
				err = executor.failRevisionMismatch(context.Background(), execution, application, cause)
			} else {
				application = argodomain.Application{}
				err = executor.failNode(context.Background(), execution, code, cause)
			}
			var failure nodeFailure
			if !errors.As(err, &failure) || !errors.Is(err, cause) || err.Error() != "execute deployment node api: node failure" {
				t.Fatalf("failure wrapping changed: %v", err)
			}
			want := ExecutionNodeUpdate{
				ExecutionID: execution.executionID, ApplicationID: execution.target.Preflight.Snapshot.ApplicationID,
				Status: "Failed", OperationID: execution.operationID, SyncStatus: application.SyncStatus,
				HealthStatus: application.HealthStatus, ActualRevision: application.ResolvedRevision,
				ErrorCode: code, ErrorMessage: cause.Error(), UpdatedAt: occurredAt.UTC(),
			}
			if !reflect.DeepEqual(repository.updates, []ExecutionNodeUpdate{want}) {
				t.Fatalf("failed node persistence changed: %#v", repository.updates)
			}
		})
	}
}

func TestDeploymentExecutorFailureReturnsPersistenceError(t *testing.T) {
	executor, _, repository := newExecutorFixture(t, "sha256:approved")
	sentinel := errors.New("persistence unavailable")
	rejecting := &failedNodeRepository{executionRepositoryStub: repository, err: sentinel}
	executor.repository = rejecting
	execution := nodeExecution{executionID: repository.snapshot.ID, target: repository.snapshot.Targets[0]}
	cause := errors.New("node failure")
	err := executor.failRevisionMismatch(context.Background(), execution, argodomain.Application{}, cause)
	var failure nodeFailure
	if !errors.Is(err, sentinel) || errors.Is(err, cause) || errors.As(err, &failure) || err.Error() != "persist failed deployment node api: persistence unavailable" {
		t.Fatalf("persistence failure wrapping changed: %v", err)
	}
	if rejecting.calls != 1 || len(repository.updates) != 0 {
		t.Fatalf("failed persistence was retried or appended: calls=%d updates=%#v", rejecting.calls, repository.updates)
	}
}

type failedNodeRepository struct {
	*executionRepositoryStub
	err   error
	calls int
}

func (r *failedNodeRepository) UpdateNode(context.Context, ExecutionNodeUpdate) error {
	r.calls++
	return r.err
}
