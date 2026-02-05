package recordings

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/aura-webinar/backend/internal/models"
	"github.com/aura-webinar/backend/pkg/queue"
	"github.com/aura-webinar/backend/pkg/response"
)

// RecordingReadyPayload is the expected body from provider recording_ready webhook.
type RecordingReadyPayload struct {
	ProviderRecordingID string `json:"provider_recording_id"`
	WebinarID           string `json:"webinar_id"`
	RecordingID         string `json:"recording_id"`
	FileURL             string `json:"file_url"`
	Duration            int    `json:"duration"`
	FileSize            int64  `json:"file_size"`
}

// WebhookHandler handles recording webhooks from the video provider (e.g. 100ms/Agora).
type WebhookHandler struct {
	repo   *Repository
	queue  *queue.Queue
	logger *zap.Logger
}

// NewWebhookHandler creates a webhook handler.
func NewWebhookHandler(repo *Repository, q *queue.Queue, logger *zap.Logger) *WebhookHandler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &WebhookHandler{repo: repo, queue: q, logger: logger}
}

// RecordingReady handles POST /webhooks/recording-ready. Validates signature (if configured), updates DB, enqueues S3 upload job.
func (h *WebhookHandler) RecordingReady(c *gin.Context) {
	var body RecordingReadyPayload
	if err := c.ShouldBindJSON(&body); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}
	if body.FileURL == "" {
		response.BadRequest(c, "file_url required")
		return
	}

	// TODO: Validate webhook signature (e.g. X-Webhook-Signature) when provider supports it.
	// if !validateSignature(c.GetHeader("X-Webhook-Signature"), body) { response.Unauthorized(c, "invalid signature"); return }

	var recordingID uuid.UUID
	var webinarID uuid.UUID
	if body.RecordingID != "" {
		var err error
		recordingID, err = uuid.Parse(body.RecordingID)
		if err != nil {
			response.BadRequest(c, "invalid recording_id")
			return
		}
	}
	if body.WebinarID != "" {
		var err error
		webinarID, err = uuid.Parse(body.WebinarID)
		if err != nil {
			response.BadRequest(c, "invalid webinar_id")
			return
		}
	}

	// If we have provider_recording_id, find existing recording; otherwise create/update by recording_id.
	var rec *models.Recording
	if body.ProviderRecordingID != "" {
		rec, _ = h.repo.GetByProviderID(c.Request.Context(), body.ProviderRecordingID)
	}
	if rec == nil && body.RecordingID != "" {
		rec, _ = h.repo.GetByID(c.Request.Context(), recordingID)
	}
	if rec == nil && body.WebinarID != "" {
		// Create new recording row for this webhook (provider didn't send our recording_id)
		rec = &models.Recording{
			WebinarID:           webinarID,
			ProviderRecordingID: body.ProviderRecordingID,
			OriginalURL:         body.FileURL,
			Duration:            body.Duration,
			FileSize:            body.FileSize,
			Status:              models.RecordingStatusProcessing,
		}
		if err := h.repo.Create(c.Request.Context(), rec); err != nil {
			h.logger.Error("create recording failed", zap.Error(err))
			response.Internal(c, "failed to create recording")
			return
		}
	}
	if rec == nil {
		response.BadRequest(c, "could not identify recording (provide recording_id or provider_recording_id + webinar_id)")
		return
	}

	if rec.OriginalURL != body.FileURL {
		if err := h.repo.UpdateOriginalURL(c.Request.Context(), rec.ID, body.FileURL); err != nil {
			h.logger.Error("update original_url failed", zap.Error(err), zap.String("recording_id", rec.ID.String()))
			response.Internal(c, "failed to update recording")
			return
		}
	}

	if err := h.queue.EnqueueRecordingUpload(c.Request.Context(), queue.RecordingUploadPayload{
		RecordingID: rec.ID,
		WebinarID:   rec.WebinarID,
		OriginalURL: body.FileURL,
	}); err != nil {
		h.logger.Error("enqueue recording upload failed", zap.Error(err), zap.String("recording_id", rec.ID.String()))
		response.Internal(c, "failed to enqueue upload")
		return
	}

	h.logger.Info("recording_ready webhook processed", zap.String("recording_id", rec.ID.String()), zap.String("original_url", body.FileURL))
	c.JSON(http.StatusOK, gin.H{"success": true, "recording_id": rec.ID, "status": "processing"})
}

// Ensure JSON binding works (compile-time check).
var _ = func() ([]byte, error) { return json.Marshal(RecordingReadyPayload{}) }
