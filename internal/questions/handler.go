package questions

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/aura-webinar/backend/internal/middleware"
	"github.com/aura-webinar/backend/internal/models"
	"github.com/aura-webinar/backend/internal/realtime"
	"github.com/aura-webinar/backend/pkg/response"
)

// CreateRequest is the body for POST /webinars/:id/questions.
type CreateRequest struct {
	Content string `json:"content" binding:"required"`
}

// Handler handles question HTTP and realtime events.
type Handler struct {
	repo   *Repository
	hub    *realtime.Hub
}

// NewHandler creates a questions handler.
func NewHandler(repo *Repository, hub *realtime.Hub) *Handler {
	return &Handler{repo: repo, hub: hub}
}

// Create handles POST /webinars/:id/questions (audience asks question).
func (h *Handler) Create(c *gin.Context) {
	webinarID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid webinar id")
		return
	}
	userID := c.MustGet(middleware.ContextUserID).(uuid.UUID)

	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	q := &models.Question{
		WebinarID: webinarID,
		UserID:    userID,
		Content:   req.Content,
	}
	if err := h.repo.Create(c.Request.Context(), q); err != nil {
		response.Internal(c, "failed to create question")
		return
	}

	// Broadcast via Redis only so all clients (including this instance) get it once (no duplicate delivery).
	h.hub.PublishToWebinarOnly(webinarID, "ask_question", map[string]interface{}{
		"id": q.ID, "webinar_id": webinarID, "user_id": userID, "content": q.Content, "approved": false,
	})
	response.Created(c, q)
}

// Approve handles PATCH /questions/:id/approve (speaker/admin approves question).
func (h *Handler) Approve(c *gin.Context) {
	questionID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid question id")
		return
	}

	q, err := h.repo.GetByID(c.Request.Context(), questionID)
	if err != nil {
		response.NotFound(c, "question not found")
		return
	}
	if err := h.repo.Approve(c.Request.Context(), questionID); err != nil {
		response.Internal(c, "failed to approve question")
		return
	}

	h.hub.PublishToWebinarOnly(q.WebinarID, "approve_question", map[string]interface{}{
		"id": q.ID, "approved": true,
	})
	response.OK(c, gin.H{"id": q.ID, "approved": true})
}
