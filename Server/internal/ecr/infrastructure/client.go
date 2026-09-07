// Package infrastructure adapts AWS ECR read-only metadata APIs to digest-resolution contracts.
package infrastructure

import (
	"context"
	"errors"
	"fmt"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
	"github.com/aws/smithy-go"

	"github.com/vincent119/ReleaseHub/Server/internal/config"
	ecrdomain "github.com/vincent119/ReleaseHub/Server/internal/ecr/domain"
)

type imageDescriber interface {
	DescribeImages(context.Context, *ecr.DescribeImagesInput, ...func(*ecr.Options)) (*ecr.DescribeImagesOutput, error)
}

// Client owns the read-only ECR client created through the Pod Identity or IRSA credential chain.
type Client struct {
	client    imageDescriber
	accountID string
}

// NewClient creates a read-only ECR adapter without accepting static credentials.
func NewClient(ctx context.Context, cfg config.AWSConfig) (*Client, error) {
	if err := cfg.ValidateECR(); err != nil {
		return nil, err
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.Region))
	if err != nil {
		return nil, fmt.Errorf("load AWS default configuration: %w", err)
	}
	return &Client{client: ecr.NewFromConfig(awsCfg), accountID: cfg.AccountID}, nil
}

func newClientForTest(client imageDescriber, accountID string) (*Client, error) {
	if client == nil || accountID == "" {
		return nil, errors.New("invalid ecr client dependencies")
	}
	return &Client{client: client, accountID: accountID}, nil
}

// ResolveDigest queries only the exact image identifier supplied by the target manifest.
func (c *Client) ResolveDigest(ctx context.Context, reference ecrdomain.ImageReference) (string, error) {
	identifier := ecrtypes.ImageIdentifier{}
	if reference.Tag != "" {
		identifier.ImageTag = &reference.Tag
	} else {
		identifier.ImageDigest = &reference.RequestedDigest
	}
	response, err := c.client.DescribeImages(ctx, &ecr.DescribeImagesInput{
		RegistryId: &c.accountID, RepositoryName: &reference.Repository, ImageIds: []ecrtypes.ImageIdentifier{identifier},
	})
	if err != nil {
		return "", mapECRReadError(err)
	}
	if len(response.ImageDetails) != 1 || response.ImageDetails[0].ImageDigest == nil || *response.ImageDetails[0].ImageDigest == "" {
		return "", ecrdomain.ErrImageNotFound
	}
	digest := *response.ImageDetails[0].ImageDigest
	if reference.RequestedDigest != "" && reference.RequestedDigest != digest {
		return "", ecrdomain.ErrDigestMismatch
	}
	return digest, nil
}

func mapECRReadError(err error) error {
	var apiError smithy.APIError
	if errors.As(err, &apiError) {
		switch apiError.ErrorCode() {
		case "ImageNotFoundException", "RepositoryNotFoundException":
			return ecrdomain.ErrImageNotFound
		}
	}
	return ecrdomain.ErrRegistryUnavailable
}
