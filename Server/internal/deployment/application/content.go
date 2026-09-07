package application

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	argodomain "github.com/vincent119/ReleaseHub/Server/internal/argocd/domain"
	deploydomain "github.com/vincent119/ReleaseHub/Server/internal/deployment/domain"
	"k8s.io/apimachinery/pkg/util/yaml"
)

// HashTargetManifests computes a stable hash independent of manifest ordering and YAML formatting.
func HashTargetManifests(manifests []string) (string, error) {
	documents := make([]string, 0, len(manifests))
	for index, manifest := range manifests {
		values, err := decodeManifestDocuments(manifest, index)
		if err != nil {
			return "", err
		}
		documents = append(documents, values...)
	}
	if len(documents) == 0 {
		return "", errors.New("target manifests are empty")
	}
	slices.Sort(documents)
	return hashJSON(documents)
}

func decodeManifestDocuments(manifest string, index int) ([]string, error) {
	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(manifest), 4096)
	var result []string
	for {
		document, done, err := decodeManifestDocument(decoder, index)
		if err != nil {
			return nil, err
		}
		if done {
			return result, nil
		}
		if document != "" {
			result = append(result, document)
		}
	}
}

func decodeManifestDocument(decoder *yaml.YAMLOrJSONDecoder, index int) (string, bool, error) {
	var document map[string]any
	if err := decoder.Decode(&document); errors.Is(err, io.EOF) {
		return "", true, nil
	} else if err != nil {
		return "", false, fmt.Errorf("decode target manifest %d: %w", index, err)
	}
	if len(document) == 0 {
		return "", false, nil
	}
	encoded, err := json.Marshal(document)
	return string(encoded), false, err
}

// NormalizeResourceDiffs creates deterministic, redacted evidence for modified resources only.
func NormalizeResourceDiffs(values []argodomain.ResourceDiff) ([]deploydomain.ResourceDiffEvidence, string, error) {
	result := make([]deploydomain.ResourceDiffEvidence, 0, len(values))
	for _, value := range values {
		if !value.Modified {
			continue
		}
		evidence, err := normalizeResourceDiff(value)
		if err != nil {
			return nil, "", err
		}
		result = append(result, evidence)
	}
	slices.SortFunc(result, func(left, right deploydomain.ResourceDiffEvidence) int {
		return strings.Compare(diffKey(left), diffKey(right))
	})
	return normalizedDiffResult(result)
}

func normalizedDiffResult(result []deploydomain.ResourceDiffEvidence) ([]deploydomain.ResourceDiffEvidence, string, error) {
	hash, err := hashJSON(result)
	if err != nil {
		return nil, "", fmt.Errorf("hash resource differences: %w", err)
	}
	return result, hash, nil
}

func normalizeResourceDiff(value argodomain.ResourceDiff) (deploydomain.ResourceDiffEvidence, error) {
	liveHash, err := hashJSONText(value.NormalizedLiveState)
	if err != nil {
		return deploydomain.ResourceDiffEvidence{}, fmt.Errorf("normalize live resource %s/%s: %w", value.Kind, value.Name, err)
	}
	predictedHash, err := hashJSONText(value.PredictedLiveState)
	if err != nil {
		return deploydomain.ResourceDiffEvidence{}, fmt.Errorf("normalize predicted resource %s/%s: %w", value.Kind, value.Name, err)
	}
	return deploydomain.ResourceDiffEvidence{
		Group: value.Group, Kind: value.Kind, Namespace: value.Namespace, Name: value.Name,
		NormalizedLiveHash: liveHash, PredictedLiveHash: predictedHash,
	}, nil
}

func diffKey(value deploydomain.ResourceDiffEvidence) string {
	return strings.Join([]string{value.Group, value.Kind, value.Namespace, value.Name, value.NormalizedLiveHash, value.PredictedLiveHash}, "\x00")
}

func hashJSONText(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return hashJSON(nil)
	}
	var document any
	if err := json.Unmarshal([]byte(value), &document); err != nil {
		return "", err
	}
	return hashJSON(document)
}

func hashJSON(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}
