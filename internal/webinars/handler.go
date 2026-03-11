package webinars

import (
	"encoding/json"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/aura-webinar/backend/internal/middleware"
	"github.com/aura-webinar/backend/internal/models"
	"github.com/aura-webinar/backend/internal/realtime"
	"github.com/aura-webinar/backend/pkg/response"
)

func parseTime(s string) (time.Time, error) {
	return time.Parse(time.RFC3339, s)
}

// CreateRequest is the body for POST /webinars.
type CreateRequest struct {
	Title           string   `json:"title" binding:"required"`
	Description     string   `json:"description"`
	StartsAt        string   `json:"starts_at" binding:"required"`
	EndsAt          *string  `json:"ends_at"`
	SpeakerIDs      []string `json:"speaker_ids"`      // optional; platform user IDs to add as speakers
	MaxAudience     *int     `json:"max_audience"`     // optional; nil = unlimited
	Category        string   `json:"category"`
	BannerImageURL  string   `json:"banner_image_url"`
}

// AddSpeakerRequest is the body for POST /webinars/:id/speakers.
type AddSpeakerRequest struct {
	UserID string `json:"user_id" binding:"required,uuid"`
}

// UpdateRegistrationFormRequest is the body for PUT /webinars/:id/registration-form.
type UpdateRegistrationFormRequest struct {
	AudienceFormConfig []models.FormFieldConfig `json:"audience_form_config"`
}

// InviteSpeakerRequest is the body for POST /webinars/:id/speakers/invite.
type InviteSpeakerRequest struct {
	Email string `json:"email" binding:"required,email"`
}

// Handler handles webinar HTTP endpoints.
type Handler struct {
	repo   *Repository
	logger *zap.Logger
}

// NewHandler creates a webinar handler.
func NewHandler(repo *Repository, logger *zap.Logger) *Handler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Handler{repo: repo, logger: logger}
}

// Create handles POST /webinars (admin only).
func (h *Handler) Create(c *gin.Context) {
	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	userID := c.MustGet(middleware.ContextUserID).(uuid.UUID)

	startsAt, err := parseTime(req.StartsAt)
	if err != nil {
		response.BadRequest(c, "invalid starts_at")
		return
	}
	var endsAt *time.Time
	if req.EndsAt != nil {
		t, err := parseTime(*req.EndsAt)
		if err != nil {
			response.BadRequest(c, "invalid ends_at")
			return
		}
		endsAt = &t
	}

	w := &models.Webinar{
		Title:          req.Title,
		Description:    req.Description,
		StartsAt:       startsAt,
		EndsAt:         endsAt,
		CreatedBy:      userID,
		MaxAudience:    req.MaxAudience,
		Category:       req.Category,
		BannerImageURL: req.BannerImageURL,
	}
	if err := h.repo.Create(c.Request.Context(), w); err != nil {
		response.Internal(c, "failed to create webinar")
		return
	}
	for _, idStr := range req.SpeakerIDs {
		speakerID, err := uuid.Parse(idStr)
		if err != nil {
			continue
		}
		_ = h.repo.AddSpeaker(c.Request.Context(), w.ID, speakerID)
	}
	response.Created(c, w)
}

// GetByID handles GET /webinars/:id.
func (h *Handler) GetByID(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		response.BadRequest(c, "invalid webinar id")
		return
	}
	w, err := h.repo.GetByID(c.Request.Context(), id)
	if err != nil {
		response.NotFound(c, "webinar not found")
		return
	}
	response.OK(c, w)
}

// AddSpeaker handles POST /webinars/:id/speakers (admin or creator).
func (h *Handler) AddSpeaker(c *gin.Context) {
	webinarID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid webinar id")
		return
	}
	userID := c.MustGet(middleware.ContextUserID).(uuid.UUID)

	ok, err := h.repo.IsAdminOrSpeaker(c.Request.Context(), webinarID, userID)
	if err != nil || !ok {
		response.Forbidden(c, "only admin or webinar creator can add speakers")
		return
	}

	var req AddSpeakerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}
	speakerID, err := uuid.Parse(req.UserID)
	if err != nil {
		response.BadRequest(c, "invalid user_id")
		return
	}
	if err := h.repo.AddSpeaker(c.Request.Context(), webinarID, speakerID); err != nil {
		response.Internal(c, "failed to add speaker")
		return
	}
	response.Created(c, gin.H{"webinar_id": webinarID, "user_id": speakerID})
}

// List handles GET /webinars.
// Query ?mine=1: only webinars created by the current user (admin dashboard).
// Query ?as_speaker=1: only webinars where the current user is a speaker (speaker dashboard).
func (h *Handler) List(c *gin.Context) {
	userID := c.MustGet(middleware.ContextUserID).(uuid.UUID)
	if c.Query("as_speaker") == "1" {
		list, err := h.repo.ListBySpeakerID(c.Request.Context(), userID)
		if err != nil {
			h.logger.Error("list webinars by speaker failed", zap.Error(err))
			response.Internal(c, "failed to list webinars")
			return
		}
		response.OK(c, list)
		return
	}
	var createdBy *uuid.UUID
	if c.Query("mine") == "1" {
		createdBy = &userID
	}
	list, err := h.repo.List(c.Request.Context(), createdBy, nil)
	if err != nil {
		h.logger.Error("list webinars failed", zap.Error(err))
		response.Internal(c, "failed to list webinars")
		return
	}
	response.OK(c, list)
}

// ListPublic handles GET /webinars/list (no auth). Returns all webinars for the audience "Join webinar" page.
func (h *Handler) ListPublic(c *gin.Context) {
	list, err := h.repo.List(c.Request.Context(), nil, nil)
	if err != nil {
		h.logger.Error("list webinars (public) failed", zap.Error(err))
		response.Internal(c, "failed to list webinars")
		return
	}
	response.OK(c, list)
}

// Update handles PATCH /webinars/:id (admin or creator).
func (h *Handler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid webinar id")
		return
	}
	userID := c.MustGet(middleware.ContextUserID).(uuid.UUID)
	w, err := h.repo.GetByID(c.Request.Context(), id)
	if err != nil {
		response.NotFound(c, "webinar not found")
		return
	}
	if w.CreatedBy != userID {
		response.Forbidden(c, "only the creator can update this webinar")
		return
	}
	var req struct {
		Title           *string `json:"title"`
		Description     *string `json:"description"`
		StartsAt        *string `json:"starts_at"`
		EndsAt          *string `json:"ends_at"`
		MaxAudience     *int    `json:"max_audience"`
		Category        *string `json:"category"`
		BannerImageURL  *string `json:"banner_image_url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	title, desc := w.Title, w.Description
	if req.Title != nil {
		title = *req.Title
	}
	if req.Description != nil {
		desc = *req.Description
	}
	var startsAt, endsAt *time.Time
	if req.StartsAt != nil {
		t, err := parseTime(*req.StartsAt)
		if err != nil {
			response.BadRequest(c, "invalid starts_at")
			return
		}
		startsAt = &t
	}
	if req.EndsAt != nil {
		t, err := parseTime(*req.EndsAt)
		if err != nil {
			response.BadRequest(c, "invalid ends_at")
			return
		}
		endsAt = &t
	}
	maxAudience := req.MaxAudience
	if req.MaxAudience != nil && *req.MaxAudience < 0 {
		maxAudience = nil // treat negative as unlimited
	}
	category, bannerURL := w.Category, w.BannerImageURL
	if req.Category != nil {
		category = *req.Category
	}
	if req.BannerImageURL != nil {
		bannerURL = *req.BannerImageURL
	}
	if err := h.repo.Update(c.Request.Context(), id, title, desc, startsAt, endsAt, maxAudience, category, bannerURL); err != nil {
		response.Internal(c, "failed to update webinar")
		return
	}
	updated, _ := h.repo.GetByID(c.Request.Context(), id)
	response.OK(c, updated)
}

// UpdateRegistrationForm handles PUT /webinars/:id/registration-form (admin/creator).
func (h *Handler) UpdateRegistrationForm(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid webinar id")
		return
	}
	userID := c.MustGet(middleware.ContextUserID).(uuid.UUID)
	w, err := h.repo.GetByID(c.Request.Context(), id)
	if err != nil {
		response.NotFound(c, "webinar not found")
		return
	}
	if w.CreatedBy != userID {
		response.Forbidden(c, "only the creator can update the registration form")
		return
	}
	var req UpdateRegistrationFormRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}
	config, err := json.Marshal(req.AudienceFormConfig)
	if err != nil {
		response.Internal(c, "failed to save form config")
		return
	}
	if err := h.repo.UpdateAudienceFormConfig(c.Request.Context(), id, config); err != nil {
		response.Internal(c, "failed to update registration form")
		return
	}
	updated, _ := h.repo.GetByID(c.Request.Context(), id)
	response.OK(c, updated)
}

// Delete handles DELETE /webinars/:id (admin or creator).
func (h *Handler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid webinar id")
		return
	}
	userID := c.MustGet(middleware.ContextUserID).(uuid.UUID)
	w, err := h.repo.GetByID(c.Request.Context(), id)
	if err != nil {
		response.NotFound(c, "webinar not found")
		return
	}
	if w.CreatedBy != userID {
		response.Forbidden(c, "only the creator can delete this webinar")
		return
	}
	if err := h.repo.Delete(c.Request.Context(), id); err != nil {
		response.Internal(c, "failed to delete webinar")
		return
	}
	response.NoContent(c)
}

// AudienceCount returns a handler that returns live audience count for a webinar (from WebSocket hub).
func (h *Handler) AudienceCount(hub *realtime.Hub) gin.HandlerFunc {
	return func(c *gin.Context) {
		webinarID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			response.BadRequest(c, "invalid webinar id")
			return
		}
		count := hub.AudienceCount(webinarID)
		response.OK(c, gin.H{"webinar_id": webinarID, "count": count})
	}
}
