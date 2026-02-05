package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"go.uber.org/zap"
)

const (
	// MaxAdFileSize is the maximum allowed file size for ad uploads (10MB).
	MaxAdFileSize = 10 * 1024 * 1024
	// FolderAds is the S3 prefix for ad objects.
	FolderAds = "ads"
	// FolderRecordings is the S3 prefix for recording objects.
	FolderRecordings = "recordings"
)

// Allowed ad MIME types and extensions.
var (
	AllowedAdTypes = map[string]string{
		"image/jpeg":      ".jpg",
		"image/jpg":       ".jpg",
		"image/png":       ".png",
		"image/webp":      ".webp",
		"image/gif":       ".gif",
		"video/mp4":       ".mp4",
		"video/quicktime": ".mp4",
	}
	AllowedAdExtensions = map[string]string{
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".png":  "image/png",
		".webp": "image/webp",
		".gif":  "image/gif",
		".mp4":  "video/mp4",
	}
)

// S3Config holds S3 client configuration.
type S3Config struct {
	Region               string
	AccessKeyID          string
	SecretAccessKey      string
	AdsBucket            string
	RecordingsBucket     string
	PresignExpireMinutes int
}

// S3 provides S3 operations with validation and pre-signed URLs.
type S3 struct {
	client    *s3.Client
	uploader  *manager.Uploader
	cfg       S3Config
	logger    *zap.Logger
}

// NewS3 creates an S3 client using credentials from config or .env (AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY).
func NewS3(ctx context.Context, cfg S3Config, logger *zap.Logger) (*S3, error) {
	accessKey := cfg.AccessKeyID
	secretKey := cfg.SecretAccessKey
	if accessKey == "" || secretKey == "" {
		accessKey = os.Getenv("AWS_ACCESS_KEY_ID")
		secretKey = os.Getenv("AWS_SECRET_ACCESS_KEY")
	}
	opts := []func(*config.LoadOptions) error{
		config.WithRegion(cfg.Region),
	}
	if accessKey != "" && secretKey != "" {
		opts = append(opts, config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			accessKey, secretKey, "",
		)))
		if logger != nil {
			logger.Info("S3 client using credentials from .env/config", zap.String("region", cfg.Region), zap.String("recordings_bucket", cfg.RecordingsBucket))
		}
	} else if logger != nil {
		logger.Warn("S3 client using default credential chain (AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY not set)")
	}
	awsCfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}
	client := s3.NewFromConfig(awsCfg)
	uploader := manager.NewUploader(client, func(u *manager.Uploader) {
		u.PartSize = 5 * 1024 * 1024 // 5MB parts for streaming
	})
	return &S3{
		client:   client,
		uploader: uploader,
		cfg:      cfg,
		logger:   logger,
	}, nil
}

// ValidateAdFileType returns true if the content type and/or extension are allowed for ads.
func ValidateAdFileType(contentType, filename string) bool {
	ext := strings.ToLower(path.Ext(filename))
	if contentType != "" {
		if _, ok := AllowedAdTypes[strings.ToLower(contentType)]; ok {
			return true
		}
	}
	if ext != "" {
		if _, ok := AllowedAdExtensions[ext]; ok {
			return true
		}
	}
	return false
}

// ContentTypeForFilename returns the MIME type for an ad filename extension.
func ContentTypeForFilename(filename string) string {
	ext := strings.ToLower(path.Ext(filename))
	if ct, ok := AllowedAdExtensions[ext]; ok {
		return ct
	}
	return "application/octet-stream"
}

// AdKey returns the S3 object key for an ad: ads/{webinar_id}/{filename}.
func AdKey(webinarID, filename string) string {
	return path.Join(FolderAds, webinarID, path.Base(filename))
}

// RecordingKey returns the S3 object key: recordings/{webinar_id}/{recording_id}.mp4.
func RecordingKey(webinarID, recordingID string) string {
	return path.Join(FolderRecordings, webinarID, recordingID+".mp4")
}

// GeneratePresignedUploadURL returns a pre-signed PUT URL for direct upload.
func (s *S3) GeneratePresignedUploadURL(ctx context.Context, bucket, key, contentType string, expires time.Duration) (string, error) {
	presignClient := s3.NewPresignClient(s.client)
	req, err := presignClient.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		ContentType: aws.String(contentType),
	}, func(opts *s3.PresignOptions) {
		opts.Expires = expires
	})
	if err != nil {
		return "", fmt.Errorf("presign put: %w", err)
	}
	return req.URL, nil
}

// GeneratePresignedDownloadURL returns a pre-signed GET URL for download.
func (s *S3) GeneratePresignedDownloadURL(ctx context.Context, bucket, key string, expires time.Duration) (string, error) {
	presignClient := s3.NewPresignClient(s.client)
	req, err := presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}, func(opts *s3.PresignOptions) {
		opts.Expires = expires
	})
	if err != nil {
		return "", fmt.Errorf("presign get: %w", err)
	}
	return req.URL, nil
}

// PresignExpire returns the configured presign duration.
func (s *S3) PresignExpire() time.Duration {
	if s.cfg.PresignExpireMinutes <= 0 {
		return 15 * time.Minute
	}
	return time.Duration(s.cfg.PresignExpireMinutes) * time.Minute
}

// UploadAdPresignedBucket returns the ads bucket name.
func (s *S3) UploadAdPresignedBucket() string { return s.cfg.AdsBucket }

// UploadRecordingsBucket returns the recordings bucket name.
func (s *S3) UploadRecordingsBucket() string { return s.cfg.RecordingsBucket }

// PublicObjectURL returns the public URL for an object (no signing; use when bucket is public).
func (s *S3) PublicObjectURL(bucket, key string) string {
	return fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", bucket, s.cfg.Region, key)
}

// Upload streams a reader to S3 (for server-side uploads, e.g. recording from provider). No encryption is set.
// Set publicRead true for ad images so the object is readable via direct URL when the bucket is intended to be public.
func (s *S3) Upload(ctx context.Context, bucket, key, contentType string, body io.Reader, contentLength int64, publicRead bool) (string, error) {
	var contentLengthPtr *int64
	if contentLength > 0 {
		contentLengthPtr = &contentLength
	}
	input := &s3.PutObjectInput{
		Bucket:        aws.String(bucket),
		Key:           aws.String(key),
		Body:          body,
		ContentType:   aws.String(contentType),
		ContentLength: contentLengthPtr,
	}
	if publicRead {
		input.ACL = types.ObjectCannedACLPublicRead
	}
	_, err := s.uploader.Upload(ctx, input)
	if err != nil {
		return "", fmt.Errorf("upload: %w", err)
	}
	url := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", bucket, s.cfg.Region, key)
	return url, nil
}

// DeleteObject removes an object from S3.
func (s *S3) DeleteObject(ctx context.Context, bucket, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("delete object: %w", err)
	}
	return nil
}

// DeleteAd removes an ad object from the ads bucket.
func (s *S3) DeleteAd(ctx context.Context, key string) error {
	return s.DeleteObject(ctx, s.cfg.AdsBucket, key)
}

// DeleteRecording removes a recording object from the recordings bucket.
func (s *S3) DeleteRecording(ctx context.Context, key string) error {
	return s.DeleteObject(ctx, s.cfg.RecordingsBucket, key)
}

// HeadObject returns object metadata if it exists.
func (s *S3) HeadObject(ctx context.Context, bucket, key string) (*s3.HeadObjectOutput, error) {
	return s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
}

// GetObjectStream returns the object body and content type for streaming (e.g. image proxy). Caller must close the body.
func (s *S3) GetObjectStream(ctx context.Context, bucket, key string) (body io.ReadCloser, contentType string, err error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, "", err
	}
	ct := ""
	if out.ContentType != nil {
		ct = *out.ContentType
	}
	return out.Body, ct, nil
}
