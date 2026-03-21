package repo

import (
	"DDDance/internal/pkg/utils/log"
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"

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
