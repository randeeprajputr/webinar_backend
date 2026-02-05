package ads

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/aura-webinar/backend/internal/middleware"
	"github.com/aura-webinar/backend/internal/models"
	"github.com/aura-webinar/backend/internal/webinars"
	"github.com/aura-webinar/backend/pkg/response"
	"github.com/aura-webinar/backend/pkg/storage"
)

// GenerateUploadURLRequest is the body for POST /webinars/:id/ads/generate-upload-url.
type GenerateUploadURLRequest struct {
	Filename    string `json:"filename" binding:"required"`
	ContentType string `json:"content_type"`
	FileSize    int64  `json:"file_size" binding:"required,gt=0"`
}

// CreateAdvertisementRequest is the body for POST /webinars/:id/ads (after client uploads via presigned URL).
type CreateAdvertisementRequest struct {
	Filename string `json:"filename" binding:"required"`
	S3Key    string `json:"s3_key" binding:"required"`
	FileType string `json:"file_type" binding:"required"`
	FileSize int64  `json:"file_size" binding:"required,gt=0"`
	Duration int    `json:"duration"`
}

// AdvertisementHandler handles advertisement HTTP endpoints (S3-backed ads).
type AdvertisementHandler struct {
	adRepo      *AdvertisementRepository
	webinarRepo *webinars.Repository
	s3          *storage.S3
	hub         HubBroadcaster
	rotators    *RotatorRegistry
	logger      *zap.Logger
}

// HubBroadcaster broadcasts ad_changed to webinar clients.
type HubBroadcaster interface {
	BroadcastToWebinarAndPublish(webinarID uuid.UUID, event string, payload interface{})
}

// NewAdvertisementHandler creates an advertisement handler.
func NewAdvertisementHandler(adRepo *AdvertisementRepository, webinarRepo *webinars.Repository, s3 *storage.S3, hub HubBroadcaster, rotators *RotatorRegistry, logger *zap.Logger) *AdvertisementHandler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &AdvertisementHandler{adRepo: adRepo, webinarRepo: webinarRepo, s3: s3, hub: hub, rotators: rotators, logger: logger}
}

// GenerateUploadURL handles POST /webinars/:id/ads/generate-upload-url (admin only). Presigned upload; prefer UploadAd for public buckets.
func (h *AdvertisementHandler) GenerateUploadURL(c *gin.Context) {
	if h.s3 == nil {
		response.Internal(c, "S3 not configured")
		return
	}
	webinarID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid webinar id")
		return
	}
	_ = c.MustGet(middleware.ContextUserID).(uuid.UUID)

	ok, err := h.webinarRepo.IsAdminOrSpeaker(c.Request.Context(), webinarID, c.MustGet(middleware.ContextUserID).(uuid.UUID))
	if err != nil || !ok {
		response.Forbidden(c, "only admin or speaker can manage ads")
		return
	}

	var req GenerateUploadURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}
	if req.FileSize > storage.MaxAdFileSize {
		response.BadRequest(c, "file size exceeds 10MB limit")
		return
	}
	if !storage.ValidateAdFileType(req.ContentType, req.Filename) {
		response.BadRequest(c, "invalid file type: only image (jpg, png, webp, gif) and mp4 video allowed")
		return
	}

	contentType := storage.ContentTypeForFilename(req.Filename)
	if req.ContentType != "" {
		if _, ok := storage.AllowedAdTypes[req.ContentType]; ok {
			contentType = req.ContentType
		}
	}

	key := storage.AdKey(webinarID.String(), req.Filename)
	expire := h.s3.PresignExpire()
	url, err := h.s3.GeneratePresignedUploadURL(c.Request.Context(), h.s3.UploadAdPresignedBucket(), key, contentType, expire)
	if err != nil {
		h.logger.Error("generate presigned upload URL failed", zap.Error(err), zap.String("webinar_id", webinarID.String()), zap.String("bucket", h.s3.UploadAdPresignedBucket()))
		response.Internal(c, "S3 upload unavailable. Ensure AWS credentials (AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY) and bucket are configured.")
		return
	}

	response.OK(c, gin.H{
		"upload_url":  url,
		"s3_key":      key,
		"content_type": contentType,
		"expires_in":  int(expire.Seconds()),
	})
}

// UploadAd handles POST /webinars/:id/ads/upload (admin only). Server-side upload to public bucket; no presigned URL, no CORS.
func (h *AdvertisementHandler) UploadAd(c *gin.Context) {
	if h.s3 == nil {
		response.Internal(c, "S3 not configured")
		return
	}
	webinarID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid webinar id")
		return
	}
	ok, err := h.webinarRepo.IsAdminOrSpeaker(c.Request.Context(), webinarID, c.MustGet(middleware.ContextUserID).(uuid.UUID))
	if err != nil || !ok {
		response.Forbidden(c, "only admin or speaker can manage ads")
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		response.BadRequest(c, "missing file (form field: file)")
		return
	}
	if file.Size > storage.MaxAdFileSize {
		response.BadRequest(c, "file size exceeds 10MB limit")
		return
	}
	if !storage.ValidateAdFileType(file.Header.Get("Content-Type"), file.Filename) {
		response.BadRequest(c, "invalid file type: only image (jpg, png, webp, gif) and mp4 video allowed")
		return
	}

	contentType := storage.ContentTypeForFilename(file.Filename)
	if file.Header.Get("Content-Type") != "" {
		if _, ok := storage.AllowedAdTypes[file.Header.Get("Content-Type")]; ok {
			contentType = file.Header.Get("Content-Type")
		}
	}

	key := storage.AdKey(webinarID.String(), file.Filename)
	rc, err := file.Open()
	if err != nil {
		h.logger.Error("open uploaded file failed", zap.Error(err))
		response.Internal(c, "failed to read file")
		return
	}
	defer rc.Close()

	_, err = h.s3.Upload(c.Request.Context(), h.s3.UploadAdPresignedBucket(), key, contentType, rc, file.Size, true)
	if err != nil {
		h.logger.Error("S3 upload failed", zap.Error(err), zap.String("webinar_id", webinarID.String()), zap.String("key", key))
		response.Internal(c, "failed to upload file to storage")
		return
	}
	// Public bucket: return public URL (no signing, no encryption)
	fileURL := h.s3.PublicObjectURL(h.s3.UploadAdPresignedBucket(), key)

	response.OK(c, gin.H{
		"s3_key":       key,
		"file_url":     fileURL,
		"content_type": contentType,
		"file_size":    file.Size,
		"filename":     file.Filename,
	})
}

// CreateAdvertisement handles POST /webinars/:id/ads (admin only). Call after client uploads file to presigned URL.
func (h *AdvertisementHandler) CreateAdvertisement(c *gin.Context) {
	if h.s3 == nil {
		response.Internal(c, "S3 not configured")
		return
	}
	webinarID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid webinar id")
		return
	}

	ok, err := h.webinarRepo.IsAdminOrSpeaker(c.Request.Context(), webinarID, c.MustGet(middleware.ContextUserID).(uuid.UUID))
	if err != nil || !ok {
		response.Forbidden(c, "only admin or speaker can manage ads")
		return
	}

	var req CreateAdvertisementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}
	if req.FileSize > storage.MaxAdFileSize {
		response.BadRequest(c, "file size exceeds 10MB limit")
		return
	}
	if !storage.ValidateAdFileType(req.FileType, req.Filename) {
		response.BadRequest(c, "invalid file type")
		return
	}

	// Public bucket: use public URL (no signing)
	fileURL := ""
	if h.s3 != nil {
		fileURL = h.s3.PublicObjectURL(h.s3.UploadAdPresignedBucket(), req.S3Key)
	}
	if fileURL == "" {
		fileURL = "s3://" + h.s3.UploadAdPresignedBucket() + "/" + req.S3Key
	}

	a := &models.Advertisement{
		WebinarID: webinarID,
		FileURL:   fileURL,
		FileType:  req.FileType,
		FileSize:  req.FileSize,
		Duration:  req.Duration,
		S3Key:     req.S3Key,
		IsActive:  true,
	}
	if err := h.adRepo.CreateAdvertisement(c.Request.Context(), a); err != nil {
		response.Internal(c, "failed to create advertisement")
		return
	}

	// Ensure playlist exists
	_, _ = h.adRepo.GetOrCreatePlaylist(c.Request.Context(), webinarID, 30)
	if h.rotators != nil {
		h.rotators.Reload(webinarID)
	}

	response.Created(c, a)
}

// StartPlaylistRequest is the body for POST /webinars/:id/ads/playlist/start.
type StartPlaylistRequest struct {
	RotationInterval int `json:"rotation_interval"`
}

// StartPlaylist handles POST /webinars/:id/ads/playlist/start (admin only).
func (h *AdvertisementHandler) StartPlaylist(c *gin.Context) {
	webinarID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid webinar id")
		return
	}
	ok, err := h.webinarRepo.IsAdminOrSpeaker(c.Request.Context(), webinarID, c.MustGet(middleware.ContextUserID).(uuid.UUID))
	if err != nil || !ok {
		response.Forbidden(c, "only admin or speaker can start playlist")
		return
	}
	var req StartPlaylistRequest
	_ = c.ShouldBindJSON(&req)
	if req.RotationInterval <= 0 {
		req.RotationInterval = 30
	}
	playlist, err := h.adRepo.GetOrCreatePlaylist(c.Request.Context(), webinarID, req.RotationInterval)
	if err != nil {
		response.Internal(c, "failed to get playlist")
		return
	}
	if err := h.adRepo.SetPlaylistRunning(c.Request.Context(), webinarID, true); err != nil {
		response.Internal(c, "failed to set playlist running")
		return
	}
	if h.rotators != nil {
		h.rotators.Start(webinarID, h.adRepo, h.hub, h.s3, playlist.RotationInterval, h.logger)
	}
	response.OK(c, gin.H{"webinar_id": webinarID, "rotation_interval": playlist.RotationInterval, "is_running": true})
}

// StopPlaylist handles POST /webinars/:id/ads/playlist/stop (admin only).
func (h *AdvertisementHandler) StopPlaylist(c *gin.Context) {
	webinarID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid webinar id")
		return
	}
	ok, err := h.webinarRepo.IsAdminOrSpeaker(c.Request.Context(), webinarID, c.MustGet(middleware.ContextUserID).(uuid.UUID))
	if err != nil || !ok {
		response.Forbidden(c, "only admin or speaker can stop playlist")
		return
	}
	if err := h.adRepo.SetPlaylistRunning(c.Request.Context(), webinarID, false); err != nil {
		response.Internal(c, "failed to set playlist stopped")
		return
	}
	if h.rotators != nil {
		h.rotators.Stop(webinarID)
	}
	response.OK(c, gin.H{"webinar_id": webinarID, "is_running": false})
}

// GetAdImage streams the ad image from S3 (proxy). Use when direct S3 URL fails (CORS/403). Admin/speaker only.
func (h *AdvertisementHandler) GetAdImage(c *gin.Context) {
	webinarID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid webinar id")
		return
	}
	adID, err := uuid.Parse(c.Param("adId"))
	if err != nil {
		response.BadRequest(c, "invalid ad id")
		return
	}
	userID, ok := c.Get(middleware.ContextUserID)
	if !ok {
		response.Unauthorized(c, "unauthorized")
		return
	}
	a, err := h.adRepo.GetAdvertisementByID(c.Request.Context(), adID)
	if err != nil {
		response.NotFound(c, "ad not found")
		return
	}
	if a.WebinarID != webinarID {
		response.NotFound(c, "ad not found")
		return
	}
	ok, err = h.webinarRepo.IsAdminOrSpeaker(c.Request.Context(), a.WebinarID, userID.(uuid.UUID))
	if err != nil || !ok {
		response.Forbidden(c, "forbidden")
		return
	}
	if a.S3Key == "" {
		response.NotFound(c, "ad has no image")
		return
	}
	if h.s3 == nil {
		response.ServiceUnavailable(c, "storage unavailable")
		return
	}
	body, contentType, err := h.s3.GetObjectStream(c.Request.Context(), h.s3.UploadAdPresignedBucket(), a.S3Key)
	if err != nil {
		h.logger.Warn("ad image get failed", zap.Error(err), zap.String("s3_key", a.S3Key))
		response.NotFound(c, "image not found")
		return
	}
	defer body.Close()
	if contentType != "" {
		c.Header("Content-Type", contentType)
	} else {
		c.Header("Content-Type", a.FileType)
	}
	c.Header("Cache-Control", "private, max-age=300")
	c.Status(http.StatusOK)
	_, _ = io.Copy(c.Writer, body)
}

// ListAdvertisements handles GET /webinars/:id/ads. Fills file_url with public URL when S3 key present (public bucket).
func (h *AdvertisementHandler) ListAdvertisements(c *gin.Context) {
	webinarID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid webinar id")
		return
	}

	list, err := h.adRepo.ListByWebinar(c.Request.Context(), webinarID)
	if err != nil {
		response.Internal(c, "failed to list ads")
		return
	}
	if h.s3 != nil {
		for i := range list {
			if list[i].S3Key != "" {
				list[i].FileURL = h.s3.PublicObjectURL(h.s3.UploadAdPresignedBucket(), list[i].S3Key)
			}
		}
	}
	response.OK(c, list)
}

// ToggleAdvertisement handles PATCH /ads/:id/toggle (admin only).
func (h *AdvertisementHandler) ToggleAdvertisement(c *gin.Context) {
	adID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid ad id")
		return
	}

	a, err := h.adRepo.GetAdvertisementByID(c.Request.Context(), adID)
	if err != nil {
		response.NotFound(c, "ad not found")
		return
	}
	ok, err := h.webinarRepo.IsAdminOrSpeaker(c.Request.Context(), a.WebinarID, c.MustGet(middleware.ContextUserID).(uuid.UUID))
	if err != nil || !ok {
		response.Forbidden(c, "only admin or speaker can toggle ad")
		return
	}

	active, err := h.adRepo.ToggleActive(c.Request.Context(), adID)
	if err != nil {
		response.Internal(c, "failed to toggle ad")
		return
	}

	if h.hub != nil {
		// Broadcast current ad so clients can refresh
		h.hub.BroadcastToWebinarAndPublish(a.WebinarID, "ad_changed", map[string]interface{}{
			"ad_id": adID, "file_url": a.FileURL, "type": a.FileType, "active": active,
		})
	}
	response.OK(c, gin.H{"id": adID, "active": active})
}

// DeleteAdvertisement handles DELETE /ads/:id (admin only).
func (h *AdvertisementHandler) DeleteAdvertisement(c *gin.Context) {
	adID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid ad id")
		return
	}

	a, err := h.adRepo.GetAdvertisementByID(c.Request.Context(), adID)
	if err != nil {
		response.NotFound(c, "ad not found")
		return
	}
	ok, err := h.webinarRepo.IsAdminOrSpeaker(c.Request.Context(), a.WebinarID, c.MustGet(middleware.ContextUserID).(uuid.UUID))
	if err != nil || !ok {
		response.Forbidden(c, "only admin or speaker can delete ad")
		return
	}

	if a.S3Key != "" && h.s3 != nil {
		_ = h.s3.DeleteAd(c.Request.Context(), a.S3Key)
	}
	if err := h.adRepo.DeleteAdvertisement(c.Request.Context(), adID); err != nil {
		response.Internal(c, "failed to delete ad")
		return
	}
	response.NoContent(c)
}
