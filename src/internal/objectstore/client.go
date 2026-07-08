package objectstore

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// NewS3Client creates a configured S3 client from the given S3Config.
// When cfg.AccessKeyID and cfg.SecretAccessKey are both set, static credentials
// are used; otherwise the default AWS credential chain applies (env vars, IAM role, etc.).
// When cfg.Endpoint is set, the custom endpoint is used. Set cfg.ForcePathStyle to enable
// path-style addressing (required by some S3-compatible self-hosted stores).
func NewS3Client(ctx context.Context, cfg S3Config) (*s3.Client, error) {
	region := cfg.Region
	if region == "" {
		region = "us-east-1"
	}

	var opts []func(*awsconfig.LoadOptions) error
	opts = append(opts, awsconfig.WithRegion(region))

	if cfg.HasStaticCredentials() {
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("loading AWS config: %w", err)
	}

	var s3Opts []func(*s3.Options)
	if cfg.Endpoint != "" || cfg.ForcePathStyle {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			if cfg.Endpoint != "" {
				o.BaseEndpoint = aws.String(cfg.Endpoint)
			}
			o.UsePathStyle = cfg.ForcePathStyle
		})
	}

	return s3.NewFromConfig(awsCfg, s3Opts...), nil
}
