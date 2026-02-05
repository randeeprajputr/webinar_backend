package recordings

import (
	"context"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/aura-webinar/backend/internal/middleware"
	"github.com/aura-webinar/backend/internal/models"
	"github.com/aura-webinar/backend/internal/webinars"
	"github.com/aura-webinar/backend/pkg/response"
	"github.com/aura-webinar/backend/pkg/storage"
)

// RecordingService starts/stops in-app recording (SFU speaker view). Optional; nil disables start/stop.
type RecordingService interface {
	StartRecording(ctx context.Context, webinarID, recordingID uuid.UUID) (outputPath string, err error)
	StopRecording(webinarID uuid.UUID) (outputPath string, err error)
	HasActiveRecording(webinarID uuid.UUID) bool
}

// Handler handles recording HTTP endpoints.
type Handler struct {
	repo        *Repository
	webinarRepo *webinars.Repository
	s3          *storage.S3
	recorder    RecordingService // optional: in-app recording from speaker view
	logger      *zap.Logger
}

// NewHandler creates a recordings handler.
func NewHandler(repo *Repository, webinarRepo *webinars.Repository, s3 *storage.S3, logger *zap.Logger) *Handler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Handler{repo: repo, webinarRepo: webinarRepo, s3: s3, logger: logger}
}

// SetRecordingService sets the optional in-app recording service (for start/stop from speaker view).
func (h *Handler) SetRecordingService(s RecordingService) { h.recorder = s }

// ListByWebinar handles GET /webinars/:id/recordings. Only admin/speaker or webinar creator can list.
func (h *Handler) ListByWebinar(c *gin.Context) {
	webinarID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid webinar id")
		return
	}
	userID := c.MustGet(middleware.ContextUserID).(uuid.UUID)

	ok, err := h.webinarRepo.IsAdminOrSpeaker(c.Request.Context(), webinarID, userID)
	if err != nil || !ok {
		w, _ := h.webinarRepo.GetByID(c.Request.Context(), webinarID)
		if w == nil || w.CreatedBy != userID {
			response.Forbidden(c, "not authorized to list recordings")
			return
		}
	}

	list, err := h.repo.ListByWebinar(c.Request.Context(), webinarID)
	if err != nil {
		h.logger.Error("list recordings failed", zap.Error(err), zap.String("webinar_id", webinarID.String()))
		response.Internal(c, "failed to list recordings")
		return
	}
	response.OK(c, list)
}

// GenerateDownloadURL handles GET /recordings/:id/download-url. Returns presigned URL; only authorized users.
func (h *Handler) GenerateDownloadURL(c *gin.Context) {
	recordingID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid recording id")
		return
	}
	userID := c.MustGet(middleware.ContextUserID).(uuid.UUID)

	rec, err := h.repo.GetByID(c.Request.Context(), recordingID)
	if err != nil {
		response.NotFound(c, "recording not found")
		return
	}
	if rec.Status != "completed" || rec.S3Key == "" {
		response.BadRequest(c, "recording not ready for download")
		return
	}

	ok, err := h.webinarRepo.IsAdminOrSpeaker(c.Request.Context(), rec.WebinarID, userID)
	if err != nil || !ok {
		w, _ := h.webinarRepo.GetByID(c.Request.Context(), rec.WebinarID)
		if w == nil || w.CreatedBy != userID {
			response.Forbidden(c, "not authorized to download this recording")
			return
		}
	}

	if h.s3 == nil {
		response.Internal(c, "S3 not configured")
		return
	}
	expire := h.s3.PresignExpire()
	url, err := h.s3.GeneratePresignedDownloadURL(c.Request.Context(), h.s3.UploadRecordingsBucket(), rec.S3Key, expire)
	if err != nil {
		h.logger.Error("presign recording download failed", zap.Error(err), zap.String("recording_id", recordingID.String()))
		response.Internal(c, "failed to generate download URL")
		return
	}
	response.OK(c, gin.H{"download_url": url, "expires_in": int(expire.Seconds())})
}

// StartRecording handles POST /webinars/:id/recording/start. Starts in-app recording (speaker view). Admin/speaker or creator only.
func (h *Handler) StartRecording(c *gin.Context) {
	if h.recorder == nil {
		response.ServiceUnavailable(c, "recording service not configured")
		return
	}
	webinarID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid webinar id")
		return
	}
	userID := c.MustGet(middleware.ContextUserID).(uuid.UUID)
	ok, err := h.webinarRepo.IsAdminOrSpeaker(c.Request.Context(), webinarID, userID)
	if err != nil || !ok {
		w, _ := h.webinarRepo.GetByID(c.Request.Context(), webinarID)
		if w == nil || w.CreatedBy != userID {
			response.Forbidden(c, "not authorized to start recording")
			return
		}
	}
	if h.recorder.HasActiveRecording(webinarID) {
		response.Conflict(c, "recording already in progress")
		return
	}
	rec, err := h.repo.CreateFromWebinarStart(c.Request.Context(), webinarID, "sfu")
	if err != nil {
		h.logger.Error("create recording row failed", zap.Error(err), zap.String("webinar_id", webinarID.String()))
		response.Internal(c, "failed to start recording")
		return
	}
	_, err = h.recorder.StartRecording(c.Request.Context(), webinarID, rec.ID)
	if err != nil {
		_ = h.repo.UpdateStatus(c.Request.Context(), rec.ID, models.RecordingStatusFailed)
		h.logger.Error("start recording failed", zap.Error(err), zap.String("webinar_id", webinarID.String()))
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, gin.H{"recording_id": rec.ID, "status": models.RecordingStatusRecording})
}

// StopRecording handles POST /webinars/:id/recording/stop. Stops in-app recording and uploads file to S3.
func (h *Handler) StopRecording(c *gin.Context) {
	if h.recorder == nil {
		response.ServiceUnavailable(c, "recording service not configured")
		return
	}
	webinarID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid webinar id")
		return
	}
	userID := c.MustGet(middleware.ContextUserID).(uuid.UUID)
	ok, err := h.webinarRepo.IsAdminOrSpeaker(c.Request.Context(), webinarID, userID)
	if err != nil || !ok {
		w, _ := h.webinarRepo.GetByID(c.Request.Context(), webinarID)
		if w == nil || w.CreatedBy != userID {
			response.Forbidden(c, "not authorized to stop recording")
			return
		}
	}
	path, err := h.recorder.StopRecording(webinarID)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	defer func() { _ = os.Remove(path) }()

	rec, err := h.repo.FindByWebinarStatus(c.Request.Context(), webinarID, models.RecordingStatusRecording)
	if err != nil || rec == nil {
		h.logger.Error("find recording in progress failed", zap.Error(err), zap.String("webinar_id", webinarID.String()))
		response.Internal(c, "recording not found")
		return
	}

	if h.s3 == nil {
		_ = h.repo.UpdateStatus(c.Request.Context(), rec.ID, models.RecordingStatusFailed)
		response.Internal(c, "S3 not configured")
		return
	}
	f, err := os.Open(path)
	if err != nil {
		_ = h.repo.UpdateStatus(c.Request.Context(), rec.ID, models.RecordingStatusFailed)
		h.logger.Error("open recording file failed", zap.Error(err), zap.String("path", path))
		response.Internal(c, "failed to upload recording")
		return
	}
	defer f.Close()
	info, _ := f.Stat()
	// Store recorder output on AWS S3 (recordings bucket): recordings/{webinar_id}/{recording_id}.mp4
	key := storage.RecordingKey(rec.WebinarID.String(), rec.ID.String())
	bucket := h.s3.UploadRecordingsBucket()
	h.logger.Info("S3 upload starting (AWS credentials from .env)", zap.String("bucket", bucket), zap.String("key", key), zap.String("recording_id", rec.ID.String()), zap.Int64("size", info.Size()))
	s3URL, err := h.s3.Upload(c.Request.Context(), bucket, key, "video/mp4", f, info.Size(), false)
	if err != nil {
		_ = h.repo.UpdateStatus(c.Request.Context(), rec.ID, models.RecordingStatusFailed)
		h.logger.Error("upload recording to S3 failed", zap.Error(err), zap.String("recording_id", rec.ID.String()))
		response.Internal(c, "failed to upload recording")
		return
	}
	if err := h.repo.UpdateS3Result(c.Request.Context(), rec.ID, s3URL, key, info.Size(), 0); err != nil {
		h.logger.Error("update recording S3 result failed", zap.Error(err))
	}
	response.OK(c, gin.H{"recording_id": rec.ID, "status": models.RecordingStatusCompleted, "s3_url": s3URL})
}
