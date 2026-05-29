package agents

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// Presigner issues short-lived GET URLs for agent binaries in S3.
type Presigner interface {
	PresignGetURL(ctx context.Context, platform string) (url string, expires time.Time, err error)
}

// S3Config configures agent object storage and presigned download URLs.
type S3Config struct {
	Bucket      string
	Prefix      string
	Region      string
	PresignTTL  time.Duration
}

// S3Presigner presigns s3:GetObject for one platform at a time.
type S3Presigner struct {
	bucket string
	prefix string
	ttl    time.Duration
	client *s3.PresignClient
}

// NewS3Presigner loads AWS config from the default credential chain.
func NewS3Presigner(cfg S3Config) (*S3Presigner, error) {
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("s3 bucket required")
	}
	prefix := strings.Trim(cfg.Prefix, "/")
	if prefix == "" {
		prefix = "agents/latest"
	}
	ttl := cfg.PresignTTL
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	region := cfg.Region
	if region == "" {
		region = "us-east-1"
	}

	awsCfg, err := config.LoadDefaultConfig(context.Background(), config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}
	client := s3.NewFromConfig(awsCfg)
	return &S3Presigner{
		bucket: cfg.Bucket,
		prefix: prefix,
		ttl:    ttl,
		client: s3.NewPresignClient(client),
	}, nil
}

// ObjectKey returns the S3 key for a validated platform.
func ObjectKey(prefix, platform string) (string, error) {
	if !ValidPlatform(platform) {
		return "", fmt.Errorf("invalid agent platform %q", platform)
	}
	prefix = strings.Trim(prefix, "/")
	return prefix + "/" + platform, nil
}

// PresignGetURL returns a presigned GET URL for the requested platform only.
func (p *S3Presigner) PresignGetURL(ctx context.Context, platform string) (string, time.Time, error) {
	key, err := ObjectKey(p.prefix, platform)
	if err != nil {
		return "", time.Time{}, err
	}
	expires := time.Now().Add(p.ttl)
	out, err := p.client.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(p.bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(p.ttl))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("presign get object: %w", err)
	}
	return out.URL, expires, nil
}
