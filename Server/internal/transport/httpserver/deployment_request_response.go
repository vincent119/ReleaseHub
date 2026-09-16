package httpserver

import (
	"encoding/json"

	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
	contract "github.com/vincent119/ReleaseHub/Server/internal/transport/openapi"
)

func deploymentRequestSummaries(values []deploydomain.DeploymentRequestSummary) []contract.DeploymentRequestSummary {
	result := make([]contract.DeploymentRequestSummary, 0, len(values))
	for _, value := range values {
		result = append(result, contract.DeploymentRequestSummary{Id: value.ID, OrganizationId: value.OrganizationID,
			ProjectId: value.ProjectID, EnvironmentId: value.EnvironmentID, Classification: contract.DeploymentRequestSummaryClassification(value.Classification),
			Status: contract.DeploymentRequestStatus(value.Status), Title: value.Title, ActiveVersionNumber: int64(value.ActiveVersionNumber),
			ApplicationCount: value.ApplicationCount, ScheduledFor: value.ScheduledFor,
			ScheduleState: contract.DeploymentRequestScheduleState(value.Schedule.State), NextEligibleAt: value.Schedule.NextEligibleAt,
			ScheduleReason: contract.DeploymentScheduleReason(value.Schedule.Reason), UpdatedAt: value.UpdatedAt})
	}
	return result
}

func deploymentRequestDetail(value deploydomain.DeploymentRequestDetail) contract.DeploymentRequestVersion {
	version := value.Version
	return contract.DeploymentRequestVersion{Id: version.ID, RequestId: version.RequestID,
		OrganizationId: value.Summary.OrganizationID, ProjectId: value.Summary.ProjectID, EnvironmentId: value.Summary.EnvironmentID,
		VersionNumber: int64(version.VersionNumber), Status: contract.DeploymentRequestStatus(version.Status),
		Classification: contract.DeploymentRequestVersionClassification(value.Summary.Classification), Fingerprint: version.Fingerprint,
		WorkflowVersionId: version.WorkflowVersionID, PlanVersionId: version.PlanVersionID, Title: version.Title,
		ChangeDescription: version.Metadata.ChangeDescription, IssueUrl: version.Metadata.IssueURL, ScheduledFor: version.Metadata.ScheduledFor,
		ScheduleState: contract.DeploymentRequestScheduleState(value.Summary.Schedule.State), NextEligibleAt: value.Summary.Schedule.NextEligibleAt,
		ScheduleReason: contract.DeploymentScheduleReason(value.Summary.Schedule.Reason),
		LockVersion:    int64(version.LockVersion), Applications: deploymentRequestApplications(value.Applications), Reviews: deploymentRequestReviews(value.Reviews),
		WorkflowStateKey: optionalText(value.WorkflowStateKey), ExecutionId: uuidPointer(value.ExecutionID),
		ExecutionStatus: deploymentExecutionStatus(value.ExecutionStatus), Capabilities: value.Capabilities, CreatedAt: version.CreatedAt}
}

func deploymentExecutionStatus(value deploydomain.ExecutionStatus) *contract.DeploymentRequestVersionExecutionStatus {
	if value == "" {
		return nil
	}
	status := contract.DeploymentRequestVersionExecutionStatus(value)
	return &status
}

func optionalText(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func deploymentRequestReviews(values []deploydomain.DeploymentRequestReviewSnapshot) []contract.DeploymentReviewTask {
	result := make([]contract.DeploymentReviewTask, 0, len(values))
	for _, value := range values {
		result = append(result, contract.DeploymentReviewTask{
			Id: value.ID, StateKey: value.StateKey, StageNumber: value.StageNumber,
			PolicyType: contract.DeploymentReviewTaskPolicyType(value.PolicyType), RequiredApprovals: value.RequiredApprovals,
			AllowSelfReview: value.AllowSelfReview, Status: contract.DeploymentReviewTaskStatus(value.Status),
		})
	}
	return result
}

func deploymentRequestApplications(values []deploydomain.DeploymentRequestApplicationSnapshot) []contract.DeploymentRequestApplicationSnapshot {
	result := make([]contract.DeploymentRequestApplicationSnapshot, 0, len(values))
	for _, value := range values {
		result = append(result, contract.DeploymentRequestApplicationSnapshot{Id: value.ID, ApplicationId: value.ApplicationID,
			ApplicationKey: value.ApplicationKey, LiveRevision: value.LiveRevision, TargetRevision: value.TargetRevision,
			TargetRevisions: value.TargetRevisions, ManifestHash: value.ManifestHash, DiffHash: value.DiffHash,
			Diff: requestDiff(value.DiffSnapshot), Order: value.Order, Images: deploymentRequestImages(value.Images)})
	}
	return result
}

func deploymentRequestImages(values []deploydomain.DeploymentRequestImageSnapshot) []contract.DeploymentImageSnapshot {
	result := make([]contract.DeploymentImageSnapshot, 0, len(values))
	for _, value := range values {
		result = append(result, contract.DeploymentImageSnapshot{ImageReference: value.ImageReference,
			Registry: value.Registry, Repository: value.Repository, Tag: value.Tag, Digest: value.Digest})
	}
	return result
}

func requestDiff(value []byte) map[string]interface{} {
	var result map[string]interface{}
	if json.Unmarshal(value, &result) != nil || result == nil {
		return map[string]interface{}{}
	}
	return result
}
