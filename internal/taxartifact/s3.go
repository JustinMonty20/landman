package taxartifact

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

type s3Store struct {
	client *s3.Client
	bucket string
}

// NewS3 selects one bucket. The caller configures its client and credentials.
func NewS3(client *s3.Client, bucket string) (Store, error) {
	if client == nil || bucket == "" {
		return nil, errors.New("S3 client and bucket are required")
	}
	return &s3Store{client: client, bucket: bucket}, nil
}

func (s *s3Store) Mode() string { return "s3" }

func (s *s3Store) Put(ctx context.Context, source string, body io.Reader) (Artifact, error) {
	file, artifact, err := stage(ctx, os.TempDir(), source, body)
	if err != nil {
		return Artifact{}, err
	}
	defer func() {
		file.Close()
		os.Remove(file.Name())
	}()
	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(artifact.key),
		Body:          file,
		ContentLength: aws.Int64(artifact.size),
		IfNoneMatch:   aws.String("*"),
	})
	if err != nil && !alreadyExists(err) {
		return Artifact{}, fmt.Errorf("retain tax artifact in S3: %w", err)
	}
	if err := Verify(ctx, s, artifact); err != nil {
		return Artifact{}, fmt.Errorf("verify retained tax artifact: %w", err)
	}
	artifact.mode = s.Mode()
	artifact.verified = true
	return artifact, nil
}

func (s *s3Store) Get(ctx context.Context, artifact Artifact) (io.ReadCloser, error) {
	if err := validateArtifact(artifact); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	response, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(artifact.key),
	})
	if err != nil {
		return nil, err
	}
	return checkedBody(ctx, response.Body, artifact), nil
}

func alreadyExists(err error) bool {
	var apiErr smithy.APIError
	return errors.As(err, &apiErr) && apiErr.ErrorCode() == "PreconditionFailed"
}
