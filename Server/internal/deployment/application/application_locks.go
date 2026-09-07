package application

import (
	"context"
	"time"

	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

type ApplicationLocks interface {
	Acquire(context.Context, deploydomain.ApplicationLockRequest) (deploydomain.ApplicationLockSet, error)
	Heartbeat(context.Context, deploydomain.ApplicationLockSet, time.Time) (deploydomain.ApplicationLockSet, error)
	Release(context.Context, deploydomain.ApplicationLockSet) error
}
