package application

import (
	"errors"
	"testing"
)

func TestExtractManifestImagesReadsContainerVariants(t *testing.T) {
	t.Parallel()
	images, err := ExtractManifestImages([]string{`
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      initContainers:
        - name: setup
          image: 123456789012.dkr.ecr.ap-northeast-1.amazonaws.com/platform/setup:v1
      containers:
        - name: api
          image: 123456789012.dkr.ecr.ap-northeast-1.amazonaws.com/platform/api:v2
        - name: duplicate
          image: 123456789012.dkr.ecr.ap-northeast-1.amazonaws.com/platform/api:v2
---
apiVersion: v1
kind: Pod
spec:
  ephemeralContainers:
    - name: debug
      image: 123456789012.dkr.ecr.ap-northeast-1.amazonaws.com/platform/debug:v3
`})
	if err != nil {
		t.Fatalf("ExtractManifestImages() error = %v", err)
	}
	if len(images) != 3 || images[0] != "123456789012.dkr.ecr.ap-northeast-1.amazonaws.com/platform/api:v2" || images[1] != "123456789012.dkr.ecr.ap-northeast-1.amazonaws.com/platform/setup:v1" || images[2] != "123456789012.dkr.ecr.ap-northeast-1.amazonaws.com/platform/debug:v3" {
		t.Fatalf("ExtractManifestImages() = %#v", images)
	}
}

func TestExtractManifestImagesFailsClosed(t *testing.T) {
	t.Parallel()
	if _, err := ExtractManifestImages([]string{"apiVersion: v1\nkind: ConfigMap\n"}); !errors.Is(err, ErrNoImageReferences) {
		t.Fatalf("missing image error = %v", err)
	}
	if _, err := ExtractManifestImages([]string{"spec:\n  containers:\n    - image: 1\n"}); err == nil {
		t.Fatal("non-string image must fail")
	}
}
