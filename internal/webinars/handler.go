package webinars

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

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
	Title       string    `json:"title" binding:"required"`
	Description string    `json:"description"`
	StartsAt    string    `json:"starts_at" binding:"required"`
	EndsAt      *string   `json:"ends_at"`
	SpeakerIDs  []string  `json:"speaker_ids"` // optional; platform user IDs to add as speakers
}

// AddSpeakerRequest is the body for POST /webinars/:id/speakers.
type AddSpeakerRequest struct {
	UserID string `json:"user_id" binding:"required,uuid"`
}

// Handler handles webinar HTTP endpoints.
type Handler struct {
	repo *Repository
}

// NewHandler creates a webinar handler.
func NewHandler(repo *Repository) *Handler {
	return &Handler{repo: repo}
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
		Title:       req.Title,
		Description: req.Description,
		StartsAt:    startsAt,
		EndsAt:      endsAt,
		CreatedBy:   userID,
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

// List handles GET /webinars. Query ?mine=1 returns only webinars created by the current user.
func (h *Handler) List(c *gin.Context) {
	var createdBy *uuid.UUID
	if c.Query("mine") == "1" {
		uid := c.MustGet(middleware.ContextUserID).(uuid.UUID)
		createdBy = &uid
	}
	list, err := h.repo.List(c.Request.Context(), createdBy, nil)
	if err != nil {
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
		Title       *string `json:"title"`
		Description *string `json:"description"`
		StartsAt    *string `json:"starts_at"`
		EndsAt      *string `json:"ends_at"`
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
	if err := h.repo.Update(c.Request.Context(), id, title, desc, startsAt, endsAt); err != nil {
		response.Internal(c, "failed to update webinar")
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
