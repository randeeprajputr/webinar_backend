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

// ListByWebinar handles GET /webinars/:id/questions (admin/speaker list with votes/answered).
func (h *Handler) ListByWebinar(c *gin.Context) {
	webinarID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid webinar id")
		return
	}
	list, err := h.repo.ListByWebinar(c.Request.Context(), webinarID)
	if err != nil {
		response.Internal(c, "failed to list questions")
		return
	}
	response.OK(c, gin.H{"questions": list})
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

	// Broadcast via Redis only so all clients get it once.
	h.hub.PublishToWebinarOnly(webinarID, "ask_question", map[string]interface{}{
		"id": q.ID, "webinar_id": webinarID, "user_id": userID, "content": q.Content, "approved": false, "answered": false, "votes": 0,
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
		"id": q.ID, "approved": true, "answered": q.Answered, "votes": q.Votes,
	})
	response.OK(c, gin.H{"id": q.ID, "approved": true})
}

// Answer handles PATCH /questions/:id/answer (speaker/admin marks question as answered).
func (h *Handler) Answer(c *gin.Context) {
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
	if err := h.repo.MarkAnswered(c.Request.Context(), questionID); err != nil {
		response.Internal(c, "failed to mark question answered")
		return
	}

	h.hub.PublishToWebinarOnly(q.WebinarID, "question_answered", map[string]interface{}{
		"id": q.ID, "answered": true,
	})
	response.OK(c, gin.H{"id": q.ID, "answered": true})
}

// Upvote handles POST /questions/:id/upvote (audience upvotes a question; one per user).
func (h *Handler) Upvote(c *gin.Context) {
	questionID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid question id")
		return
	}
	userID := c.MustGet(middleware.ContextUserID).(uuid.UUID)

	q, err := h.repo.GetByID(c.Request.Context(), questionID)
	if err != nil {
		response.NotFound(c, "question not found")
		return
	}
	votes, err := h.repo.Upvote(c.Request.Context(), questionID, userID)
	if err != nil {
		response.Internal(c, "failed to upvote question")
		return
	}

	h.hub.PublishToWebinarOnly(q.WebinarID, "question_votes", map[string]interface{}{
		"id": q.ID, "votes": votes,
	})
	response.OK(c, gin.H{"id": q.ID, "votes": votes})
}
