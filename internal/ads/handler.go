package ads

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/aura-webinar/backend/internal/middleware"
	"github.com/aura-webinar/backend/internal/models"
	"github.com/aura-webinar/backend/internal/realtime"
	"github.com/aura-webinar/backend/internal/webinars"
	"github.com/aura-webinar/backend/pkg/response"
)

// CreateRequest is the body for POST /webinars/:id/ads.
type CreateRequest struct {
	Title    string `json:"title" binding:"required"`
	Content  string `json:"content"`
	ImageURL string `json:"image_url"`
	LinkURL  string `json:"link_url"`
}

// Handler handles ad HTTP endpoints.
type Handler struct {
	repo        *Repository
	webinarRepo *webinars.Repository
	hub         *realtime.Hub
}

// NewHandler creates an ads handler.
func NewHandler(repo *Repository, webinarRepo *webinars.Repository, hub *realtime.Hub) *Handler {
	return &Handler{repo: repo, webinarRepo: webinarRepo, hub: hub}
}

// Create handles POST /webinars/:id/ads (speaker/admin).
func (h *Handler) Create(c *gin.Context) {
	webinarID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid webinar id")
		return
	}
	userID := c.MustGet(middleware.ContextUserID).(uuid.UUID)

	ok, err := h.webinarRepo.IsAdminOrSpeaker(c.Request.Context(), webinarID, userID)
	if err != nil || !ok {
		response.Forbidden(c, "only admin or speaker can create ads")
		return
	}

	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	a := &models.Ad{
		WebinarID: webinarID,
		Title:     req.Title,
		Content:   req.Content,
		ImageURL:  req.ImageURL,
		LinkURL:   req.LinkURL,
	}
	if err := h.repo.Create(c.Request.Context(), a); err != nil {
		response.Internal(c, "failed to create ad")
		return
	}
	response.Created(c, a)
}

// Activate handles PATCH /ads/:id/activate (speaker/admin). Also broadcasts rotate_ad for display.
func (h *Handler) Activate(c *gin.Context) {
	adID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid ad id")
		return
	}
	userID := c.MustGet(middleware.ContextUserID).(uuid.UUID)

	a, err := h.repo.GetByID(c.Request.Context(), adID)
	if err != nil {
		response.NotFound(c, "ad not found")
		return
	}
	ok, err := h.webinarRepo.IsAdminOrSpeaker(c.Request.Context(), a.WebinarID, userID)
	if err != nil || !ok {
		response.Forbidden(c, "only admin or speaker can activate ad")
		return
	}
	if err := h.repo.Activate(c.Request.Context(), adID); err != nil {
		response.Internal(c, "failed to activate ad")
		return
	}

	h.hub.BroadcastToWebinarAndPublish(a.WebinarID, "rotate_ad", map[string]interface{}{
		"id": a.ID, "title": a.Title, "content": a.Content, "image_url": a.ImageURL, "link_url": a.LinkURL,
	})
	response.OK(c, gin.H{"id": a.ID, "active": true})
}
