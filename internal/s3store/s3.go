package s3store

import (
	"context"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"tempmail/internal/config"
)

type Store struct {
	cfg    *config.Config
	client *s3.Client
}

func New(ctx context.Context, cfg *config.Config) (*Store, error) {
	if !cfg.S3Enabled() {
		return nil, nil
	}
	creds := credentials.NewStaticCredentialsProvider(cfg.S3AccessKeyID, cfg.S3SecretAccessKey, "")
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(cfg.S3Region),
		awsconfig.WithCredentialsProvider(creds),
	)
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = true
		if cfg.S3Endpoint != "" && !strings.HasPrefix(cfg.S3Endpoint, "http") {
			cfg.S3Endpoint = "https://" + cfg.S3Endpoint
		}
		o.BaseEndpoint = aws.String(cfg.S3Endpoint)
	})
	return &Store{cfg: cfg, client: client}, nil
}

func (s *Store) Enabled() bool { return s != nil && s.client != nil }

func (s *Store) GetURL(ctx context.Context, prefix, key string) (string, error) {
	presign := s3.NewPresignClient(s.client)
	out, err := presign.PresignGetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(s.cfg.S3Bucket), Key: aws.String(prefix + "/" + key)}, s3.WithPresignExpires(time.Duration(s.cfg.S3URLExpires)*time.Second))
	if err != nil {
		return "", err
	}
	return out.URL, nil
}

func (s *Store) PutURL(ctx context.Context, prefix, key string) (string, error) {
	presign := s3.NewPresignClient(s.client)
	out, err := presign.PresignPutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(s.cfg.S3Bucket), Key: aws.String(prefix + "/" + key)}, s3.WithPresignExpires(time.Duration(s.cfg.S3URLExpires)*time.Second))
	if err != nil {
		return "", err
	}
	return out.URL, nil
}

func (s *Store) List(ctx context.Context, prefix string) ([]string, error) {
	out, err := s.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String(s.cfg.S3Bucket), Prefix: aws.String(prefix + "/")})
	if err != nil {
		return nil, err
	}
	keys := []string{}
	for _, o := range out.Contents {
		k := aws.ToString(o.Key)
		if after, ok := strings.CutPrefix(k, prefix+"/"); ok && after != "" {
			keys = append(keys, after)
		}
	}
	return keys, nil
}

func (s *Store) Delete(ctx context.Context, prefix, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(s.cfg.S3Bucket), Key: aws.String(prefix + "/" + key)})
	return err
}
