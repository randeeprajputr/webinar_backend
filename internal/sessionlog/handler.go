package sessionlog

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/aura-webinar/backend/pkg/response"
)

// Handler handles GET /webinars/:id/attendees.
type Handler struct {
	repo *Repository
}

// NewHandler creates a session log handler.
func NewHandler(repo *Repository) *Handler {
	return &Handler{repo: repo}
}

// GetAttendees handles GET /webinars/:id/attendees (admin/speaker: list of attendees with join time).
func (h *Handler) GetAttendees(c *gin.Context) {
	webinarID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid webinar id")
		return
	}
	list, err := h.repo.ListByWebinar(c.Request.Context(), webinarID)
	if err != nil {
		response.Internal(c, "failed to list attendees")
		return
	}
	response.OK(c, gin.H{"attendees": list})
}
