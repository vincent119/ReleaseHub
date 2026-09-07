package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

func loadSuccessfulHistoryApplications(ctx context.Context, db *gorm.DB, row deploymentHistoryRow) ([]deploydomain.DeploymentRequestApplicationSnapshot, error) {
	applications, err := loadRequestApplications(ctx, db, row.RequestVersionID)
	if err != nil {
		return nil, err
	}
	var nodes []deploymentExecutionNodeModel
	if err := db.WithContext(ctx).Where("execution_id = ?", row.ExecutionID).Find(&nodes).Error; err != nil {
		return nil, fmt.Errorf("load successful deployment history nodes: %w", err)
	}
	byApplication := make(map[uuid.UUID]deploymentExecutionNodeModel, len(nodes))
	for _, node := range nodes {
		byApplication[node.ApplicationID] = node
	}
	for index := range applications {
		applications[index] = successfulApplicationSnapshot(applications[index], byApplication[applications[index].ApplicationID])
	}
	return applications, nil
}

func successfulApplicationSnapshot(value deploydomain.DeploymentRequestApplicationSnapshot, node deploymentExecutionNodeModel) deploydomain.DeploymentRequestApplicationSnapshot {
	value.LiveRevision = node.ActualRevision
	value.Images = historyActualImageSnapshots(node.ActualImages)
	return value
}

func historyActualImageSnapshots(value []byte) []deploydomain.DeploymentRequestImageSnapshot {
	var images []deploydomain.DeploymentRequestImageSnapshot
	if json.Unmarshal(value, &images) != nil || images == nil {
		return []deploydomain.DeploymentRequestImageSnapshot{}
	}
	return images
}
