package repo

import (
	"DDDance/internal/pkg/utils/log"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	uuid "github.com/satori/go.uuid"
)

type S3Repository struct {
	client *s3.Client
	bucket string
}

func NewS3Repository(client *s3.Client, bucket string) *S3Repository {
	return &S3Repository{
		client: client,
		bucket: bucket,
	}
}

func (r *S3Repository) UploadDance(ctx context.Context, buffer []byte, fileFormat string, danceExtension string) (string, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	if r.client == nil || r.bucket == "" {
		return "", errors.New("S3 client not configured")
	}

	picID := uuid.NewV4().String()

	danceKey := filepath.Join("videos", picID+danceExtension)

	_, err := r.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(r.bucket),
		Key:         aws.String(danceKey),
		Body:        bytes.NewReader(buffer),
		ContentType: aws.String(fileFormat),
		ACL:         types.ObjectCannedACLPublicRead,
	})
	if err != nil {
		logger.Error("failed to upload dance to S3", "error", err)
		return "", fmt.Errorf("failed to upload dance: %w", err)
	}

	return danceKey, nil
}

func (r *S3Repository) ListDances(ctx context.Context, maxKeys int) ([]string, error) {
    logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

    if r.client == nil || r.bucket == "" {
        return nil, errors.New("S3 client not configured")
    }

    resp, err := r.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
        Bucket:    aws.String(r.bucket),
        Prefix:    aws.String("results/"),
        Delimiter: aws.String("/"),
        MaxKeys:   aws.Int32(int32(maxKeys)),
    })
    if err != nil {
        logger.Error("failed to list dances from S3", "error", err)
        return nil, fmt.Errorf("failed to list dances: %w", err)
    }

    var danceIDs []string
    for _, prefix := range resp.CommonPrefixes {
        if prefix.Prefix == nil {
            continue
        }
        trimmed := strings.TrimPrefix(*prefix.Prefix, "results/")
        trimmed = strings.TrimSuffix(trimmed, "/")
        if trimmed != "" {
            danceIDs = append(danceIDs, trimmed)
        }
    }

    return danceIDs, nil
}

func (r *S3Repository) DownloadFile(ctx context.Context, s3Key string) ([]byte, error) {
    result, err := r.client.GetObject(ctx, &s3.GetObjectInput{
        Bucket: aws.String(r.bucket),
        Key:    aws.String(s3Key),
    })
    if err != nil {
        return nil, fmt.Errorf("failed to get object %s: %w", s3Key, err)
    }
    defer result.Body.Close()

    data, err := io.ReadAll(result.Body)
    if err != nil {
        return nil, fmt.Errorf("failed to read object body: %w", err)
    }
    return data, nil
}

func (r *S3Repository) UploadFileRaw(ctx context.Context, localPath string, s3Key string) error {
    f, err := os.Open(localPath)
    if err != nil {
        return fmt.Errorf("failed to open file: %w", err)
    }
    defer f.Close()

    _, err = r.client.PutObject(ctx, &s3.PutObjectInput{
        Bucket:      aws.String(r.bucket),
        Key:         aws.String(s3Key),
        Body:        f,
        ContentType: aws.String("video/mp4"),
        ACL:         types.ObjectCannedACLPublicRead,
    })
    return err
}

func (r *S3Repository) DeleteFile(ctx context.Context, s3Key string) error {
    _, err := r.client.DeleteObject(ctx, &s3.DeleteObjectInput{
        Bucket: aws.String(r.bucket),
        Key:    aws.String(s3Key),
    })
    return err
}