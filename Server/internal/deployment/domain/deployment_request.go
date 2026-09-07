package domain

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// DeploymentRequestStatus identifies the lifecycle of an immutable request version.
type DeploymentRequestStatus string

const (
	DeploymentRequestCandidate     DeploymentRequestStatus = "Candidate"
	DeploymentRequestPendingReview DeploymentRequestStatus = "PendingReview"
	DeploymentRequestApproved      DeploymentRequestStatus = "Approved"
	DeploymentRequestDeploying     DeploymentRequestStatus = "Deploying"
	DeploymentRequestSucceeded     DeploymentRequestStatus = "Succeeded"
	DeploymentRequestFailed        DeploymentRequestStatus = "Failed"
	DeploymentRequestPartialFailed DeploymentRequestStatus = "PartialFailed"
	DeploymentRequestBlocked       DeploymentRequestStatus = "Blocked"
	DeploymentRequestSuperseded    DeploymentRequestStatus = "Superseded"
	DeploymentRequestTerminated    DeploymentRequestStatus = "Terminated"
)

// DeploymentRequestMetadata is optional reviewed information that must produce a new version.
type DeploymentRequestMetadata struct {
	ChangeDescription string
	IssueURL          string
	ScheduledFor      *time.Time
}

// NewDeploymentRequestMetadata validates user-editable request metadata.
func NewDeploymentRequestMetadata(value DeploymentRequestMetadata) (DeploymentRequestMetadata, error) {
	value.ChangeDescription = strings.TrimSpace(value.ChangeDescription)
	value.IssueURL = strings.TrimSpace(value.IssueURL)
	if len(value.ChangeDescription) > 10000 || len(value.IssueURL) > 2048 {
		return DeploymentRequestMetadata{}, errors.New("deployment request metadata exceeds its limit")
	}
	if value.ScheduledFor != nil {
		at := value.ScheduledFor.UTC()
		value.ScheduledFor = &at
	}
	return value, nil
}

// DeploymentRequestVersionSummary is the immutable request version projection needed by D3.
type DeploymentRequestVersionSummary struct {
	ID                uuid.UUID
	RequestID         uuid.UUID
	VersionNumber     uint64
	Status            DeploymentRequestStatus
	Fingerprint       string
	WorkflowVersionID uuid.UUID
	PlanVersionID     uuid.UUID
	Title             string
	Metadata          DeploymentRequestMetadata
	LockVersion       uint64
	SourceSnapshot    []byte
	CreatedAt         time.Time
}

// DeploymentRequestSummary identifies a request and the version currently visible to operators.
type DeploymentRequestSummary struct {
	ID                  uuid.UUID
	OrganizationID      uuid.UUID
	ProjectID           uuid.UUID
	EnvironmentID       uuid.UUID
	Classification      string
	Status              DeploymentRequestStatus
	Title               string
	ActiveVersionNumber uint64
	ApplicationCount    int
	UpdatedAt           time.Time
}

// DeploymentRequestImageSnapshot preserves one immutable image selection.
type DeploymentRequestImageSnapshot struct {
	ID             uuid.UUID
	ImageReference string
	Registry       string
	Repository     string
	Tag            string
	Digest         string
}

// DeploymentRequestApplicationSnapshot preserves one Application's reviewed content.
type DeploymentRequestApplicationSnapshot struct {
	ID              uuid.UUID
	ApplicationID   uuid.UUID
	ApplicationKey  string
	LiveRevision    string
	TargetRevision  string
	TargetRevisions []string
	ManifestHash    string
	DiffHash        string
	DiffSnapshot    []byte
	SourceSnapshot  []byte
	Order           int
	Images          []DeploymentRequestImageSnapshot
}

// DeploymentRequestReviewSnapshot exposes the immutable review task projection.
type DeploymentRequestReviewSnapshot struct {
	ID                uuid.UUID
	StateKey          string
	StageNumber       int
	PolicyType        ReviewPolicyType
	RequiredApprovals int
	AllowSelfReview   bool
	Status            ReviewTaskStatus
}

// DeploymentRequestDetail combines the request and one immutable Version.
type DeploymentRequestDetail struct {
	Summary          DeploymentRequestSummary
	Version          DeploymentRequestVersionSummary
	Applications     []DeploymentRequestApplicationSnapshot
	Reviews          []DeploymentRequestReviewSnapshot
	WorkflowStateKey string
	ExecutionID      uuid.UUID
	ExecutionStatus  ExecutionStatus
	Capabilities     []string
}

// CanBeSuperseded reports whether a later reviewed version may replace this version.
func (v DeploymentRequestVersionSummary) CanBeSuperseded() bool {
	return v.Status != DeploymentRequestDeploying && v.Status != DeploymentRequestSucceeded &&
		v.Status != DeploymentRequestFailed && v.Status != DeploymentRequestSuperseded &&
		v.Status != DeploymentRequestTerminated
}
