package bootstrap

import (
	"crypto/sha256"

	auditapp "github.com/vincent119/ReleaseHub/Server/internal/audit/application"
	auditdomain "github.com/vincent119/ReleaseHub/Server/internal/audit/domain"
	auditinfra "github.com/vincent119/ReleaseHub/Server/internal/audit/infrastructure"
	authzinfra "github.com/vincent119/ReleaseHub/Server/internal/authorization/infrastructure"
	"github.com/vincent119/ReleaseHub/Server/internal/transport/httpserver"
	"gorm.io/gorm"
)

func buildAuditAndDeploymentHandlerParts(dependencies apiHandlerDependencies, parts *apiHandlerParts) error {
	var err error
	parts.audit, err = newAuditHandlerOptions(dependencies.resources.db, dependencies.policy, dependencies.cfg.Session.EncryptionKey)
	if err != nil {
		return err
	}
	parts.deployment, err = newDeploymentHandlerOptions(dependencies.resources.db, dependencies.policy, dependencies.resources.argoClient)
	return err
}

func buildVersionedHandlerParts(dependencies apiHandlerDependencies, parts *apiHandlerParts) error {
	var err error
	parts.workflow, err = newWorkflowHandlerOptions(dependencies.resources.db, dependencies.policy)
	if err != nil {
		return err
	}
	parts.plan, err = newPlanHandlerOptions(dependencies.resources.db, dependencies.policy)
	if err != nil {
		return err
	}
	return buildAuditAndDeploymentHandlerParts(dependencies, parts)
}

func newAuditHandlerOptions(db *gorm.DB, policy *authzinfra.PolicyEngine, sessionKey string) (httpserver.AuditHandlerOptions, error) {
	repository, err := auditinfra.NewPostgresRepository(db)
	if err != nil {
		return httpserver.AuditHandlerOptions{}, err
	}
	visibility, err := auditinfra.NewVisibilityResolver(db)
	if err != nil {
		return httpserver.AuditHandlerOptions{}, err
	}
	key := sha256.Sum256([]byte("releasehub:audit-cursor:v1:" + sessionKey))
	cursors, err := auditdomain.NewCursorCodec(key[:])
	if err != nil {
		return httpserver.AuditHandlerOptions{}, err
	}
	queries, err := auditapp.NewQueryService(repository, policy, visibility, visibility, cursors)
	if err != nil {
		return httpserver.AuditHandlerOptions{}, err
	}
	return httpserver.AuditHandlerOptions{Queries: queries}, nil
}
