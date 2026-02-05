package server

import (
	"context"
	"errors"
	"io"
	"os"
	"path"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type Storage interface {
	Upload(ctx context.Context, clientID, remotePath string, body io.Reader, size int64) (string, error)
	Exists(ctx context.Context, clientID, remotePath string) (bool, error)
	Download(ctx context.Context, clientID, remotePath string) (io.ReadCloser, string, error)
}

type S3Client struct {
	client     *s3.Client
	bucket     string
	pathPrefix string
}

func NewS3Client(cfg S3Config) *S3Client {
	opts := []func(*s3.Options){
		func(o *s3.Options) {
			o.Region = cfg.Region
			o.Credentials = credentials.NewStaticCredentialsProvider(
				cfg.AccessKeyID,
				cfg.SecretAccessKey,
				"",
			)
		},
	}

	if cfg.Endpoint != "" {
		opts = append(opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
			o.UsePathStyle = true
		})
	}

	client := s3.New(s3.Options{}, opts...)

	return &S3Client{
		client:     client,
		bucket:     cfg.Bucket,
		pathPrefix: cfg.PathPrefix,
	}
}

func (c *S3Client) buildKey(clientID, remotePath string) string {
	return path.Join(c.pathPrefix, clientID, remotePath)
}

func (c *S3Client) Upload(ctx context.Context, clientID, remotePath string, body io.Reader, size int64) (string, error) {
	key := c.buildKey(clientID, remotePath)

	_, err := c.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(c.bucket),
		Key:           aws.String(key),
		Body:          body,
		ContentLength: aws.Int64(size),
	})
	if err != nil {
		return "", err
	}

	return key, nil
}

func (c *S3Client) Exists(ctx context.Context, clientID, remotePath string) (bool, error) {
	key := c.buildKey(clientID, remotePath)

	_, err := c.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var notFound *types.NotFound
		if errors.As(err, &notFound) {
			return false, nil
		}
		var noSuchKey *types.NoSuchKey
		if errors.As(err, &noSuchKey) {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

func (c *S3Client) Download(ctx context.Context, clientID, remotePath string) (io.ReadCloser, string, error) {
	key := c.buildKey(clientID, remotePath)

	result, err := c.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var noSuchKey *types.NoSuchKey
		if errors.As(err, &noSuchKey) {
			return nil, "", os.ErrNotExist
		}
		return nil, "", err
	}

	contentType := "application/octet-stream"
	if result.ContentType != nil {
		contentType = *result.ContentType
	}

	return result.Body, contentType, nil
}
