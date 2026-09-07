package infrastructure

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/ecr"
	ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
	"github.com/aws/smithy-go"

	ecrdomain "github.com/vincent119/ReleaseHub/Server/internal/ecr/domain"
)

func TestECRClientUsesExactManifestIdentifier(t *testing.T) {
	t.Parallel()
	reference := mustImageReference(t, "123456789012.dkr.ecr.ap-northeast-1.amazonaws.com/platform/api:v1")
	stub := &imageDescriberStub{response: &ecr.DescribeImagesOutput{ImageDetails: []ecrtypes.ImageDetail{{ImageDigest: stringPointer("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")}}}}
	client, err := newClientForTest(stub, "123456789012")
	if err != nil {
		t.Fatalf("newClientForTest() error = %v", err)
	}
	digest, err := client.ResolveDigest(context.Background(), reference)
	if err != nil || digest == "" {
		t.Fatalf("ResolveDigest() = %q, %v", digest, err)
	}
	if stub.request == nil || stub.request.RepositoryName == nil || *stub.request.RepositoryName != "platform/api" || stub.request.ImageIds[0].ImageTag == nil || *stub.request.ImageIds[0].ImageTag != "v1" || stub.request.ImageIds[0].ImageDigest != nil {
		t.Fatalf("DescribeImages request = %#v", stub.request)
	}
}

func TestECRClientFailsClosedForMissingOrDriftedDigest(t *testing.T) {
	t.Parallel()
	missing := &imageDescriberStub{err: apiErrorStub{code: "ImageNotFoundException"}}
	client, _ := newClientForTest(missing, "123456789012")
	if _, err := client.ResolveDigest(context.Background(), mustImageReference(t, "123456789012.dkr.ecr.ap-northeast-1.amazonaws.com/platform/api:v1")); !errors.Is(err, ecrdomain.ErrImageNotFound) {
		t.Fatalf("missing image error = %v", err)
	}
	drifted := &imageDescriberStub{response: &ecr.DescribeImagesOutput{ImageDetails: []ecrtypes.ImageDetail{{ImageDigest: stringPointer("sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")}}}}
	client, _ = newClientForTest(drifted, "123456789012")
	if _, err := client.ResolveDigest(context.Background(), mustImageReference(t, "123456789012.dkr.ecr.ap-northeast-1.amazonaws.com/platform/api:v1@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")); !errors.Is(err, ecrdomain.ErrDigestMismatch) {
		t.Fatalf("drifted image error = %v", err)
	}
}

type imageDescriberStub struct {
	response *ecr.DescribeImagesOutput
	err      error
	request  *ecr.DescribeImagesInput
}

func (s *imageDescriberStub) DescribeImages(_ context.Context, request *ecr.DescribeImagesInput, _ ...func(*ecr.Options)) (*ecr.DescribeImagesOutput, error) {
	s.request = request
	if s.err != nil {
		return nil, s.err
	}
	return s.response, nil
}

type apiErrorStub struct{ code string }

func (e apiErrorStub) Error() string        { return e.code }
func (e apiErrorStub) ErrorCode() string    { return e.code }
func (e apiErrorStub) ErrorMessage() string { return e.code }
func (e apiErrorStub) ErrorFault() smithy.ErrorFault {
	return smithy.FaultClient
}

func mustImageReference(t *testing.T, value string) ecrdomain.ImageReference {
	t.Helper()
	scope, err := ecrdomain.NewRegistryScope("123456789012", "ap-northeast-1", []string{"platform/api"})
	if err != nil {
		t.Fatalf("NewRegistryScope() error = %v", err)
	}
	reference, err := scope.ParseImageReference(value)
	if err != nil {
		t.Fatalf("ParseImageReference() error = %v", err)
	}
	return reference
}

func stringPointer(value string) *string { return &value }
