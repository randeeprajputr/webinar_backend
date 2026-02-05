package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const (
	// QueueRecordings is the Redis list key for recording upload jobs.
	QueueRecordings = "worker:recordings"
	// QueueEmails is the Redis list key for email jobs.
	QueueEmails = "worker:emails"
	// QueueAnalytics is the Redis list key for analytics processing jobs.
	QueueAnalytics = "worker:analytics"
	// QueueDLQ is the dead-letter queue for failed jobs after retries.
	QueueDLQ = "worker:dlq"
	// MaxRetries is the number of times to retry a job before moving to DLQ.
	MaxRetries = 3
	// RetryBackoff is the delay between retries.
	RetryBackoff = 10 * time.Second
)

// JobType identifies the job kind.
type JobType string

const (
	JobTypeRecordingUpload JobType = "recording_upload"
	JobTypeEmail           JobType = "email"
	JobTypeAnalytics       JobType = "analytics"
)

// RecordingUploadPayload is the payload for recording upload jobs.
type RecordingUploadPayload struct {
	RecordingID uuid.UUID `json:"recording_id"`
	WebinarID   uuid.UUID `json:"webinar_id"`
	OriginalURL string   `json:"original_url"`
}

// EmailPayload is the payload for email jobs.
type EmailPayload struct {
	EmailType      string    `json:"email_type"`
	WebinarID      uuid.UUID `json:"webinar_id"`
	RegistrationID uuid.UUID `json:"registration_id"`
	RecipientEmail string    `json:"recipient_email"`
	Subject        string    `json:"subject"`
	BodyHTML       string    `json:"body_html"`
}

// AnalyticsPayload is the payload for analytics processing jobs.
type AnalyticsPayload struct {
	WebinarID      uuid.UUID `json:"webinar_id"`
	StreamSessionID uuid.UUID `json:"stream_session_id"`
}

// Job is a generic job envelope.
type Job struct {
	ID        string          `json:"id"`
	Type      JobType         `json:"type"`
	Payload   json.RawMessage `json:"payload"`
	Attempt   int             `json:"attempt"`
	CreatedAt time.Time       `json:"created_at"`
}

// Queue enqueues and dequeues jobs via Redis.
type Queue struct {
	client *redis.Client
	logger *zap.Logger
}

// NewQueue creates a new Redis-backed job queue.
func NewQueue(client *redis.Client, logger *zap.Logger) *Queue {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Queue{client: client, logger: logger}
}

// EnqueueRecordingUpload enqueues a recording upload job.
func (q *Queue) EnqueueRecordingUpload(ctx context.Context, payload RecordingUploadPayload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	job := Job{
		ID:        uuid.New().String(),
		Type:      JobTypeRecordingUpload,
		Payload:   body,
		Attempt:   0,
		CreatedAt: time.Now(),
	}
	raw, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("marshal job: %w", err)
	}
	if err := q.client.RPush(ctx, QueueRecordings, raw).Err(); err != nil {
		return fmt.Errorf("rpush: %w", err)
	}
	q.logger.Debug("enqueued recording upload job", zap.String("job_id", job.ID), zap.String("recording_id", payload.RecordingID.String()))
	return nil
}

// EnqueueEmail enqueues an email job.
func (q *Queue) EnqueueEmail(ctx context.Context, payload EmailPayload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	job := Job{
		ID:        uuid.New().String(),
		Type:      JobTypeEmail,
		Payload:   body,
		Attempt:   0,
		CreatedAt: time.Now(),
	}
	raw, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("marshal job: %w", err)
	}
	if err := q.client.RPush(ctx, QueueEmails, raw).Err(); err != nil {
		return fmt.Errorf("rpush: %w", err)
	}
	q.logger.Debug("enqueued email job", zap.String("job_id", job.ID), zap.String("email_type", payload.EmailType))
	return nil
}

// EnqueueAnalytics enqueues an analytics processing job.
func (q *Queue) EnqueueAnalytics(ctx context.Context, payload AnalyticsPayload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	job := Job{
		ID:        uuid.New().String(),
		Type:      JobTypeAnalytics,
		Payload:   body,
		Attempt:   0,
		CreatedAt: time.Now(),
	}
	raw, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("marshal job: %w", err)
	}
	if err := q.client.RPush(ctx, QueueAnalytics, raw).Err(); err != nil {
		return fmt.Errorf("rpush: %w", err)
	}
	q.logger.Debug("enqueued analytics job", zap.String("job_id", job.ID), zap.String("webinar_id", payload.WebinarID.String()))
	return nil
}

// Dequeue blocks until a job is available or ctx is done. Returns job and key (queue name).
func (q *Queue) Dequeue(ctx context.Context) (*Job, string, error) {
	result, err := q.client.BLPop(ctx, 0, QueueRecordings).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, "", nil
		}
		return nil, "", err
	}
	if len(result) < 2 {
		return nil, "", nil
	}
	var job Job
	if err := json.Unmarshal([]byte(result[1]), &job); err != nil {
		q.logger.Warn("invalid job payload", zap.String("raw", result[1]), zap.Error(err))
		return nil, "", nil
	}
	return &job, result[0], nil
}

// Retry re-enqueues a job with incremented attempt. If attempt >= MaxRetries, pushes to DLQ instead.
func (q *Queue) Retry(ctx context.Context, job *Job) error {
	job.Attempt++
	raw, err := json.Marshal(job)
	if err != nil {
		return err
	}
	if job.Attempt >= MaxRetries {
		if err := q.client.RPush(ctx, QueueDLQ, raw).Err(); err != nil {
			q.logger.Error("dlq push failed", zap.Error(err), zap.String("job_id", job.ID))
			return err
		}
		q.logger.Warn("job moved to DLQ", zap.String("job_id", job.ID), zap.Int("attempt", job.Attempt))
		return nil
	}
	if err := q.client.RPush(ctx, QueueRecordings, raw).Err(); err != nil {
		return err
	}
	q.logger.Info("job retried", zap.String("job_id", job.ID), zap.Int("attempt", job.Attempt))
	return nil
}
