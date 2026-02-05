package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/aura-webinar/backend/internal/recordings"
	"github.com/aura-webinar/backend/pkg/queue"
	"github.com/aura-webinar/backend/pkg/storage"
)

// RecordingProcessor processes recording upload jobs: download from provider URL, upload to S3, update DB.
type RecordingProcessor struct {
	recRepo *recordings.Repository
	s3      *storage.S3
	queue   *queue.Queue
	logger  *zap.Logger
}

// NewRecordingProcessor creates a recording upload processor.
func NewRecordingProcessor(recRepo *recordings.Repository, s3 *storage.S3, q *queue.Queue, logger *zap.Logger) *RecordingProcessor {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &RecordingProcessor{recRepo: recRepo, s3: s3, queue: q, logger: logger}
}

// Process executes one recording upload job.
func (p *RecordingProcessor) Process(ctx context.Context, job *queue.Job) error {
	if job.Type != queue.JobTypeRecordingUpload {
		return fmt.Errorf("unknown job type: %s", job.Type)
	}
	var payload queue.RecordingUploadPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal payload: %w", err)
	}

	rec, err := p.recRepo.GetByID(ctx, payload.RecordingID)
	if err != nil || rec == nil {
		return fmt.Errorf("recording not found: %s", payload.RecordingID)
	}
	if rec.Status == "completed" {
		p.logger.Info("recording already completed", zap.String("recording_id", rec.ID.String()))
		return nil
	}

	// Download from provider (streaming)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, payload.OriginalURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download status: %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "video/mp4"
	}
	key := storage.RecordingKey(payload.WebinarID.String(), payload.RecordingID.String())

	// Stream upload to S3 (no full buffer)
	s3URL, err := p.s3.Upload(ctx, p.s3.UploadRecordingsBucket(), key, contentType, resp.Body, resp.ContentLength, false)
	if err != nil {
		return fmt.Errorf("s3 upload: %w", err)
	}

	// Update DB
	if err := p.recRepo.UpdateS3Result(ctx, payload.RecordingID, s3URL, key, resp.ContentLength, rec.Duration); err != nil {
		p.logger.Error("update recording S3 result failed", zap.Error(err), zap.String("recording_id", payload.RecordingID.String()))
		return fmt.Errorf("update db: %w", err)
	}

	p.logger.Info("recording upload completed", zap.String("recording_id", payload.RecordingID.String()), zap.String("s3_key", key))
	return nil
}

// Run starts the worker loop: dequeue, process, retry on error.
func (p *RecordingProcessor) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			p.logger.Info("recording worker stopping")
			return
		default:
		}

		job, _, err := p.queue.Dequeue(ctx)
		if err != nil {
			p.logger.Warn("dequeue error", zap.Error(err))
			time.Sleep(queue.RetryBackoff)
			continue
		}
		if job == nil {
			continue
		}

		p.logger.Debug("processing job", zap.String("job_id", job.ID), zap.String("type", string(job.Type)))
		if err := p.Process(ctx, job); err != nil {
			p.logger.Error("job failed", zap.String("job_id", job.ID), zap.Error(err))
			if reErr := p.queue.Retry(ctx, job); reErr != nil {
				p.logger.Error("retry enqueue failed", zap.Error(reErr))
			}
			time.Sleep(queue.RetryBackoff)
			continue
		}
	}
}
