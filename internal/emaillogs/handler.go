package emaillogs

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/aura-webinar/backend/pkg/response"
)

// Handler handles email log HTTP endpoints.
type Handler struct {
	repo *Repository
}

// NewHandler creates an email logs handler.
func NewHandler(repo *Repository) *Handler {
	return &Handler{repo: repo}
}

// ListByWebinar handles GET /webinars/:id/emails. Returns email logs for the webinar.
// Call after RequireWebinarOrgAccess or RequireRole(admin) so access is already validated.
func (h *Handler) ListByWebinar(c *gin.Context) {
	idStr := c.Param("id")
	webinarID, err := uuid.Parse(idStr)
	if err != nil {
		response.BadRequest(c, "invalid webinar id")
		return
	}
	logs, err := h.repo.ListByWebinar(c.Request.Context(), webinarID)
	if err != nil {
		response.Internal(c, "failed to load email logs")
		return
	}
	response.OK(c, logs)
}

// ResendRequest is the body for POST /webinars/:id/emails/resend.
type ResendRequest struct {
	RegistrationID string `json:"registration_id" binding:"required,uuid"`
	EmailType     string `json:"email_type"`
}

// Resend handles POST /webinars/:id/emails/resend. Enqueues or sends reminder (no-op if worker not configured).
func (h *Handler) Resend(c *gin.Context) {
	idStr := c.Param("id")
	_, err := uuid.Parse(idStr)
	if err != nil {
		response.BadRequest(c, "invalid webinar id")
		return
	}
	var body ResendRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		response.BadRequest(c, "registration_id required")
		return
	}
	// TODO: enqueue email to worker (QueueEmails) or send via SMTP when email worker is implemented
	response.OK(c, gin.H{"message": "resend queued"})
}
