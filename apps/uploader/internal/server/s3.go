package server

import (
	"context"
	"errors"
	"io"
	"os"
	"path"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type ListEntry struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

type Storage interface {
	Upload(ctx context.Context, clientID, remotePath string, body io.Reader, size int64) (string, error)
	Exists(ctx context.Context, clientID, remotePath string) (bool, error)
	Download(ctx context.Context, clientID, remotePath string) (io.ReadCloser, string, error)
	DeletePrefix(ctx context.Context, clientID, prefix string) (int, error)
	List(ctx context.Context, clientID, prefix string) ([]ListEntry, error)
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

func (c *S3Client) DeletePrefix(ctx context.Context, clientID, prefix string) (int, error) {
	fullPrefix := c.buildKey(clientID, prefix)

	var toDelete []types.ObjectIdentifier
	paginator := s3.NewListObjectsV2Paginator(c.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(c.bucket),
		Prefix: aws.String(fullPrefix),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return 0, err
		}
		for _, obj := range page.Contents {
			toDelete = append(toDelete, types.ObjectIdentifier{Key: obj.Key})
		}
	}

	if len(toDelete) == 0 {
		return 0, nil
	}

	_, err := c.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
		Bucket: aws.String(c.bucket),
		Delete: &types.Delete{Objects: toDelete},
	})
	if err != nil {
		return 0, err
	}

	return len(toDelete), nil
}

func (c *S3Client) List(ctx context.Context, clientID, prefix string) ([]ListEntry, error) {
	fullPrefix := c.buildKey(clientID, prefix)
	clientRoot := c.buildKey(clientID, "") + "/"

	var entries []ListEntry
	paginator := s3.NewListObjectsV2Paginator(c.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(c.bucket),
		Prefix: aws.String(fullPrefix),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, obj := range page.Contents {
			relPath := strings.TrimPrefix(*obj.Key, clientRoot)
			entries = append(entries, ListEntry{
				Path: relPath,
				Size: *obj.Size,
			})
		}
	}

	return entries, nil
}
