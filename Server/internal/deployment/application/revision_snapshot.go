package application

import (
	"slices"
	"strings"

	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
)

func canonicalTargetRevision(snapshot deploydomain.DeploymentRequestApplicationSnapshot) (string, []string) {
	revisions := snapshot.TargetRevisions
	if len(revisions) == 1 && revisions[0] == snapshot.TargetRevision && strings.TrimSpace(snapshot.TargetRevision) != "" {
		return snapshot.TargetRevision, nil
	}
	if len(revisions) > 0 {
		return "", slices.Clone(revisions)
	}
	return snapshot.TargetRevision, nil
}
