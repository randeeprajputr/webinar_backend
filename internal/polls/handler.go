package polls

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/aura-webinar/backend/internal/middleware"
	"github.com/aura-webinar/backend/internal/models"
	"github.com/aura-webinar/backend/internal/realtime"
	"github.com/aura-webinar/backend/internal/webinars"
	"github.com/aura-webinar/backend/pkg/response"
)

// CreateRequest is the body for POST /webinars/:id/polls.
type CreateRequest struct {
	Question string `json:"question" binding:"required"`
	OptionA  string `json:"option_a" binding:"required"`
	OptionB  string `json:"option_b" binding:"required"`
	OptionC  string `json:"option_c" binding:"required"`
	OptionD  string `json:"option_d" binding:"required"`
}

// LaunchRequest / CloseRequest - no body.

// AnswerRequest is the body for POST /polls/:id/answer.
type AnswerRequest struct {
	Option string `json:"option" binding:"required,oneof=A B C D"`
}

// Handler handles poll HTTP endpoints.
type Handler struct {
	repo       *Repository
	webinarRepo *webinars.Repository
	hub        *realtime.Hub
}

// NewHandler creates a polls handler.
func NewHandler(repo *Repository, webinarRepo *webinars.Repository, hub *realtime.Hub) *Handler {
	return &Handler{repo: repo, webinarRepo: webinarRepo, hub: hub}
}

// Create handles POST /webinars/:id/polls (speaker/admin).
func (h *Handler) Create(c *gin.Context) {
	webinarID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid webinar id")
		return
	}
	userID := c.MustGet(middleware.ContextUserID).(uuid.UUID)

	ok, err := h.webinarRepo.IsAdminOrSpeaker(c.Request.Context(), webinarID, userID)
	if err != nil || !ok {
		response.Forbidden(c, "only admin or speaker can create polls")
		return
	}

	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	p := &models.Poll{
		WebinarID: webinarID,
		Question:  req.Question,
		OptionA:   req.OptionA,
		OptionB:   req.OptionB,
		OptionC:   req.OptionC,
		OptionD:   req.OptionD,
	}
	if err := h.repo.Create(c.Request.Context(), p); err != nil {
		response.Internal(c, "failed to create poll")
		return
	}
	response.Created(c, p)
}

// Launch handles POST /polls/:id/launch (speaker/admin).
func (h *Handler) Launch(c *gin.Context) {
	pollID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid poll id")
		return
	}
	userID := c.MustGet(middleware.ContextUserID).(uuid.UUID)

	p, err := h.repo.GetByID(c.Request.Context(), pollID)
	if err != nil {
		response.NotFound(c, "poll not found")
		return
	}
	ok, err := h.webinarRepo.IsAdminOrSpeaker(c.Request.Context(), p.WebinarID, userID)
	if err != nil || !ok {
		response.Forbidden(c, "only admin or speaker can launch poll")
		return
	}
	if err := h.repo.Launch(c.Request.Context(), pollID); err != nil {
		response.Internal(c, "failed to launch poll")
		return
	}

	h.hub.BroadcastToWebinarAndPublish(p.WebinarID, "launch_poll", map[string]interface{}{
		"id": p.ID, "question": p.Question, "option_a": p.OptionA, "option_b": p.OptionB, "option_c": p.OptionC, "option_d": p.OptionD,
	})
	response.OK(c, gin.H{"id": pollID, "launched": true})
}

// Close handles POST /polls/:id/close (speaker/admin).
func (h *Handler) Close(c *gin.Context) {
	pollID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid poll id")
		return
	}
	userID := c.MustGet(middleware.ContextUserID).(uuid.UUID)

	p, err := h.repo.GetByID(c.Request.Context(), pollID)
	if err != nil {
		response.NotFound(c, "poll not found")
		return
	}
	ok, err := h.webinarRepo.IsAdminOrSpeaker(c.Request.Context(), p.WebinarID, userID)
	if err != nil || !ok {
		response.Forbidden(c, "only admin or speaker can close poll")
		return
	}
	if err := h.repo.Close(c.Request.Context(), pollID); err != nil {
		response.Internal(c, "failed to close poll")
		return
	}

	h.hub.BroadcastToWebinarAndPublish(p.WebinarID, "close_poll", map[string]interface{}{"id": p.ID})
	response.OK(c, gin.H{"id": pollID, "closed": true})
}

// Answer handles POST /polls/:id/answer (audience).
func (h *Handler) Answer(c *gin.Context) {
	pollID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid poll id")
		return
	}
	userID := c.MustGet(middleware.ContextUserID).(uuid.UUID)

	p, err := h.repo.GetByID(c.Request.Context(), pollID)
	if err != nil {
		response.NotFound(c, "poll not found")
		return
	}
	if !p.Launched || p.Closed {
		response.BadRequest(c, "poll is not open for answers")
		return
	}

	var req AnswerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: option must be A, B, C, or D")
		return
	}
	if err := h.repo.Answer(c.Request.Context(), pollID, userID, req.Option); err != nil {
		response.Internal(c, "failed to record answer")
		return
	}

	h.hub.BroadcastToWebinarAndPublish(p.WebinarID, "answer_poll", map[string]interface{}{
		"poll_id": pollID, "user_id": userID, "option": req.Option,
	})
	response.OK(c, gin.H{"poll_id": pollID, "option": req.Option})
}
