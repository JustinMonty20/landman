package taxartifact

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func TestS3Contract(t *testing.T) {
	endpoint := os.Getenv("TAX_ARTIFACT_TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("set TAX_ARTIFACT_TEST_S3_ENDPOINT for an isolated S3-compatible service")
	}
	if os.Getenv("TAX_ARTIFACT_TEST_S3_ACCESS_KEY") == "" || os.Getenv("TAX_ARTIFACT_TEST_S3_SECRET_KEY") == "" {
		t.Fatal("test service credentials are required")
	}
	config := aws.Config{
		Region: "us-east-1",
		Credentials: credentials.NewStaticCredentialsProvider(
			os.Getenv("TAX_ARTIFACT_TEST_S3_ACCESS_KEY"), os.Getenv("TAX_ARTIFACT_TEST_S3_SECRET_KEY"), ""),
	}
	client := s3.NewFromConfig(config, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(endpoint)
		options.UsePathStyle = true
		options.RetryMaxAttempts = 1
	})
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	bucket := fmt.Sprintf("tax-artifact-test-%x", random)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if _, err := client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		paginator := s3.NewListObjectsV2Paginator(client, &s3.ListObjectsV2Input{Bucket: aws.String(bucket)})
		for paginator.HasMorePages() {
			page, err := paginator.NextPage(cleanupCtx)
			if err != nil {
				t.Errorf("list test artifacts: %v", err)
				return
			}
			for _, object := range page.Contents {
				if _, err := client.DeleteObject(cleanupCtx, &s3.DeleteObjectInput{Bucket: aws.String(bucket), Key: object.Key}); err != nil {
					t.Errorf("remove test artifact: %v", err)
				}
			}
		}
		if _, err := client.DeleteBucket(cleanupCtx, &s3.DeleteBucketInput{Bucket: aws.String(bucket)}); err != nil {
			t.Errorf("remove test bucket: %v", err)
		}
	})
	store, err := NewS3(client, bucket)
	if err != nil {
		t.Fatal(err)
	}
	testContract(t, store)
	const largeSize = 32 << 20
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	large, err := store.Put(ctx, "union_billing", io.LimitReader(zeroReader{}, largeSize))
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatal(err)
	}
	if large.Size() != largeSize || after.TotalAlloc-before.TotalAlloc > 8<<20 {
		t.Fatalf("size=%d, Go allocations=%d for %d-byte S3 stream", large.Size(), after.TotalAlloc-before.TotalAlloc, largeSize)
	}
	artifact, err := store.Put(ctx, "union_billing", strings.NewReader("original"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket), Key: aws.String(artifact.Key()),
		Body: strings.NewReader("corrupted"), ContentLength: aws.Int64(9),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Put(ctx, "union_billing", strings.NewReader("original")); err == nil {
		t.Fatal("S3 accepted a corrupt object as an identical retry")
	}
	if err := Verify(ctx, store, artifact); err == nil {
		t.Fatal("the retry replaced a corrupt S3 object")
	}
}
