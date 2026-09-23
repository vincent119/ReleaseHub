package application

import (
	"errors"
	"strings"
)

func candidateTargetRevisions(content candidateTargetContent) ([]string, error) {
	if len(content.application.Sources) <= 1 {
		return []string{}, nil
	}
	if len(content.application.ResolvedRevisions) != len(content.application.Sources) {
		return nil, errors.New("candidate multi-source revision vector is incomplete")
	}
	revisions := make([]string, len(content.application.ResolvedRevisions))
	for index, revision := range content.application.ResolvedRevisions {
		revisions[index] = strings.TrimSpace(revision)
		if revisions[index] == "" {
			return nil, errors.New("candidate multi-source revision vector is incomplete")
		}
	}
	return revisions, nil
}
