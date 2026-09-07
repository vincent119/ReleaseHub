package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strings"

	"github.com/google/uuid"
)

type commandReplay struct {
	command string
	key     string
	hash    string
}

func (s *ExecutionControlService) replay(ctx context.Context, principal RequestPrincipal, snapshot ExecutionControlSnapshot, input commandReplay) (DeploymentExecution, bool, error) {
	if err := s.authorize(ctx, principal, permissionForCommand(input.command), snapshot.Scope); err != nil {
		return DeploymentExecution{}, false, err
	}
	return s.repository.Replay(ctx, ExecutionCommandReplay{
		RequestVersionID: snapshot.Execution.RequestVersionID, Command: input.command,
		IdempotencyKey: strings.TrimSpace(input.key), RequestHash: input.hash,
	})
}

func permissionForCommand(command string) string {
	return "deployment_request." + strings.ToLower(command)
}

func retryRequestHash(input RetryExecutionInput) string {
	ids := slices.Clone(input.ApplicationIDs)
	slices.SortFunc(ids, func(left, right uuid.UUID) int { return strings.Compare(left.String(), right.String()) })
	return hashCommandRequest(struct {
		Applications []uuid.UUID `json:"applications"`
		Expected     uint64      `json:"expectedVersion"`
	}{Applications: ids, Expected: input.ExpectedVersion})
}

func terminateRequestHash(input TerminateExecutionInput) string {
	return hashCommandRequest(struct {
		Reason   string `json:"reason"`
		Expected uint64 `json:"expectedVersion"`
	}{Reason: strings.TrimSpace(input.Reason), Expected: input.ExpectedVersion})
}

func unlockRequestHash(input UnlockExecutionInput) string {
	states := canonicalActualStates(input.ActualStates)
	return hashCommandRequest(struct {
		Reason   string        `json:"reason"`
		Expected uint64        `json:"expectedVersion"`
		States   []ActualState `json:"actualStates"`
	}{Reason: strings.TrimSpace(input.Reason), Expected: input.ExpectedVersion, States: states})
}

func canonicalActualStates(values []ActualState) []ActualState {
	result := slices.Clone(values)
	for index := range result {
		result[index].Images = slices.Clone(result[index].Images)
		slices.SortFunc(result[index].Images, compareActualImage)
	}
	slices.SortFunc(result, compareActualState)
	return result
}

func hashCommandRequest(value any) string {
	encoded, _ := json.Marshal(value)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}
