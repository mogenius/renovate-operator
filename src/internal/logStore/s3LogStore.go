package logStore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"renovate-operator/internal/objectstore"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/go-logr/logr"
)

// s3LogStore is the S3-backed LogStore implementation.
type s3LogStore struct {
	client *s3.Client
	bucket string
	prefix string
	// tagging is the URL-encoded tag set sent with every PutObject; nil sends none.
	tagging *string
	logger  logr.Logger
}

// newS3LogStore creates an s3LogStore from the given config and log options.
func newS3LogStore(ctx context.Context, cfg objectstore.S3Config, opts S3LogOptions, logger logr.Logger) (*s3LogStore, error) {
	client, err := objectstore.NewS3Client(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("creating S3 client: %w", err)
	}
	prefix := opts.Prefix
	if prefix == "" {
		prefix = "renovate-logs"
	}
	return &s3LogStore{
		client:  client,
		bucket:  cfg.Bucket,
		prefix:  strings.TrimSuffix(prefix, "/"),
		tagging: encodeTagging(opts.Tags),
		logger:  logger,
	}, nil
}

// encodeTagging renders tags in the URL query format S3 expects for x-amz-tagging.
// A nil result omits the header, which keeps stores without tagging support working.
func encodeTagging(tags map[string]string) *string {
	if len(tags) == 0 {
		return nil
	}
	values := url.Values{}
	for k, v := range tags {
		values.Set(k, v)
	}
	return aws.String(values.Encode())
}

func (s *s3LogStore) Save(namespace, renovateJob, project, logs string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(buildS3Key(s.prefix, namespace, renovateJob, project)),
		Body:        bytes.NewReader([]byte(logs)),
		ContentType: aws.String("text/plain; charset=utf-8"),
		Tagging:     s.tagging,
	})
	if err != nil {
		s.logger.Error(err, "failed to save logs to S3", "bucket", s.bucket)
	}
}

func (s *s3LogStore) Get(namespace, renovateJob, project string) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(buildS3Key(s.prefix, namespace, renovateJob, project)),
	})
	if err != nil {
		if _, ok := errors.AsType[*types.NoSuchKey](err); ok {
			return "", false
		}
		s.logger.Error(err, "failed to get logs from S3", "bucket", s.bucket)
		return "", false
	}
	defer func() { _ = out.Body.Close() }()

	data, err := io.ReadAll(out.Body)
	if err != nil {
		s.logger.Error(err, "failed to read log body from S3")
		return "", false
	}
	return string(data), true
}

// buildS3Key constructs the S3 object key as {prefix}/{namespace}/{renovateJob}/{project}.log.
// project may contain slashes (e.g. "org/repo"), producing a browsable bucket hierarchy.
func buildS3Key(prefix, namespace, renovateJob, project string) string {
	return fmt.Sprintf("%s/%s/%s/%s.log", prefix, namespace, renovateJob, project)
}
