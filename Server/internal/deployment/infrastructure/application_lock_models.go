package infrastructure

import (
	"time"

	"github.com/google/uuid"

	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

type applicationLockModel struct {
	ApplicationID uuid.UUID `gorm:"type:uuid;primaryKey"`
	ExecutionID   uuid.UUID
	OwnerToken    uuid.UUID
	FencingToken  uint64
	AcquiredAt    time.Time
	HeartbeatAt   time.Time
}

func (applicationLockModel) TableName() string {
	return "deployment_application_locks"
}

func applicationLockFromModel(model applicationLockModel, duration time.Duration) deploydomain.ApplicationLockLease {
	return deploydomain.ApplicationLockLease{
		ApplicationID: model.ApplicationID, ExecutionID: model.ExecutionID,
		OwnerToken: model.OwnerToken, FencingToken: model.FencingToken,
		HeartbeatAt: model.HeartbeatAt, ExpiresAt: model.HeartbeatAt.Add(duration),
	}
}
