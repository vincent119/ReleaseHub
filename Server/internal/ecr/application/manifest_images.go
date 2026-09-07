package application

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"k8s.io/apimachinery/pkg/util/yaml"
)

var ErrNoImageReferences = errors.New("target manifests contain no container image references")

// ExtractManifestImages reads only Kubernetes container image fields from Argo CD rendered manifests.
func ExtractManifestImages(manifests []string) ([]string, error) {
	images := make([]string, 0)
	seen := make(map[string]struct{})
	for index, manifest := range manifests {
		decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(manifest), 4096)
		for {
			var document map[string]any
			err := decoder.Decode(&document)
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("decode target manifest %d: %w", index, err)
			}
			if err := collectContainerImages(document, &images, seen); err != nil {
				return nil, fmt.Errorf("read target manifest %d: %w", index, err)
			}
		}
	}
	if len(images) == 0 {
		return nil, ErrNoImageReferences
	}
	return images, nil
}

func collectContainerImages(value any, images *[]string, seen map[string]struct{}) error {
	object, isObject := value.(map[string]any)
	if !isObject {
		return nil
	}
	for _, key := range []string{"containers", "initContainers", "ephemeralContainers"} {
		if err := appendContainerImages(object[key], images, seen); err != nil {
			return err
		}
	}
	for _, child := range object {
		if err := collectContainerImages(child, images, seen); err != nil {
			return err
		}
	}
	return nil
}

func appendContainerImages(value any, images *[]string, seen map[string]struct{}) error {
	containers, isContainers := value.([]any)
	if !isContainers {
		return nil
	}
	for _, value := range containers {
		container, isContainer := value.(map[string]any)
		if !isContainer {
			return errors.New("container entry must be an object")
		}
		rawImage, hasImage := container["image"]
		if !hasImage {
			continue
		}
		image, isImage := rawImage.(string)
		image = strings.TrimSpace(image)
		if !isImage || image == "" {
			return errors.New("container image must be a non-empty string")
		}
		if _, exists := seen[image]; exists {
			continue
		}
		seen[image] = struct{}{}
		*images = append(*images, image)
	}
	return nil
}
