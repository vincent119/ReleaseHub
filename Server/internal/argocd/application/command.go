package application

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	argodomain "github.com/vincent119/ReleaseHub/Server/internal/argocd/domain"
)

var (
	// ErrIdempotencyKeyReuse prevents one key from being replayed with a different command payload.
	ErrIdempotencyKeyReuse = errors.New("idempotency key is already used by a different command")
	// ErrCommandInProgress signals that an accepted command must be completed by the original request or recovery worker.
	ErrCommandInProgress = errors.New("onboarding command is still in progress")
)

// CommandOperation identifies an onboarding command with independently idempotent semantics.
type CommandOperation string

const (
	CommandDryRun  CommandOperation = "dry_run"
	CommandConfirm CommandOperation = "confirm"
)

// CommandState describes a persisted command result.
type CommandState string

const (
	CommandAccepted  CommandState = "Accepted"
	CommandCompleted CommandState = "Completed"
)

// OnboardingCommand is the durable record used to prevent unsafe command replay.
type OnboardingCommand struct {
	ID                 uuid.UUID
	ActorID            uuid.UUID
	ApplicationID      uuid.UUID
	Operation          CommandOperation
	IdempotencyKey     string
	RequestFingerprint string
	State              CommandState
	OnboardingStatus   argodomain.OnboardingStatus
	OnboardingVersion  *uint64
	ResponseCode       string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	CompletedAt        *time.Time
}

// NewOnboardingCommand validates an idempotent command identity before it reaches persistence.
func NewOnboardingCommand(actorID, applicationID uuid.UUID, operation CommandOperation, idempotencyKey string, expectedVersion *uint64, now time.Time) (OnboardingCommand, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if actorID == uuid.Nil || applicationID == uuid.Nil || !operation.Valid() || idempotencyKey == "" || len(idempotencyKey) > 255 || now.IsZero() {
		return OnboardingCommand{}, errors.New("invalid onboarding command")
	}
	if operation == CommandConfirm && (expectedVersion == nil || *expectedVersion == 0) {
		return OnboardingCommand{}, errors.New("confirm command requires an onboarding version")
	}
	if operation == CommandDryRun && expectedVersion != nil {
		return OnboardingCommand{}, errors.New("dry-run command cannot contain an onboarding version")
	}
	fingerprint := commandFingerprint(actorID, applicationID, operation, expectedVersion)
	return OnboardingCommand{ID: uuid.New(), ActorID: actorID, ApplicationID: applicationID, Operation: operation, IdempotencyKey: idempotencyKey, RequestFingerprint: fingerprint, State: CommandAccepted, CreatedAt: now.UTC(), UpdatedAt: now.UTC()}, nil
}

// Valid reports whether the operation is defined by the onboarding command protocol.
func (o CommandOperation) Valid() bool { return o == CommandDryRun || o == CommandConfirm }

func commandFingerprint(actorID, applicationID uuid.UUID, operation CommandOperation, expectedVersion *uint64) string {
	version := ""
	if expectedVersion != nil {
		version = strconv.FormatUint(*expectedVersion, 10)
	}
	value := actorID.String() + "\n" + applicationID.String() + "\n" + string(operation) + "\n" + version
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
