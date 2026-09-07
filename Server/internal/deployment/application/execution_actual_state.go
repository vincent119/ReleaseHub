package application

import (
	"context"
	"slices"
	"strings"

	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

func (s *ExecutionControlService) readActualStates(ctx context.Context, nodes []DeploymentExecutionNode) ([]ActualState, error) {
	result := make([]ActualState, 0, len(nodes))
	for _, node := range nodes {
		state, err := s.actualStates.ReadActualState(ctx, node)
		if err != nil {
			return nil, err
		}
		result = append(result, state)
	}
	return result, nil
}

func actualStatesEqual(left, right []ActualState) bool {
	if len(left) != len(right) {
		return false
	}
	left = slices.Clone(left)
	right = slices.Clone(right)
	slices.SortFunc(left, compareActualState)
	slices.SortFunc(right, compareActualState)
	for index := range left {
		if left[index].ApplicationID != right[index].ApplicationID || left[index].Revision != right[index].Revision ||
			!actualImagesEqual(left[index].Images, right[index].Images) {
			return false
		}
	}
	return true
}

func compareActualState(left, right ActualState) int {
	return strings.Compare(left.ApplicationID.String(), right.ApplicationID.String())
}

func actualImagesEqual(left, right []deploydomain.DeploymentRequestImageSnapshot) bool {
	if len(left) != len(right) {
		return false
	}
	left, right = slices.Clone(left), slices.Clone(right)
	slices.SortFunc(left, compareActualImage)
	slices.SortFunc(right, compareActualImage)
	return slices.EqualFunc(left, right, func(a, b deploydomain.DeploymentRequestImageSnapshot) bool {
		return a.ImageReference == b.ImageReference && a.Digest == b.Digest
	})
}

func compareActualImage(left, right deploydomain.DeploymentRequestImageSnapshot) int {
	if left.ImageReference < right.ImageReference {
		return -1
	}
	if left.ImageReference > right.ImageReference {
		return 1
	}
	return 0
}
