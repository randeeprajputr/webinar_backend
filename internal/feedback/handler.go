package feedback

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/aura-webinar/backend/internal/auth"
	"github.com/aura-webinar/backend/internal/middleware"
	"github.com/aura-webinar/backend/internal/models"
	"github.com/aura-webinar/backend/internal/registrations"
	"github.com/aura-webinar/backend/internal/webinars"
	"github.com/aura-webinar/backend/pkg/response"
)

// Handler handles feedback HTTP endpoints.
type Handler struct {
	repo        *Repository
	webinarRepo *webinars.Repository
	regRepo     *registrations.Repository
	authRepo    *auth.Repository
}

// NewHandler creates a feedback handler.
func NewHandler(repo *Repository, webinarRepo *webinars.Repository, regRepo *registrations.Repository, authRepo *auth.Repository) *Handler {
	return &Handler{repo: repo, webinarRepo: webinarRepo, regRepo: regRepo, authRepo: authRepo}
}

// SubmitRequest is the body for POST /webinars/:id/feedback.
type SubmitRequest struct {
	Rating               int    `json:"rating" binding:"required,min=1,max=5"`
	SpeakerEffectiveness *int   `json:"speaker_effectiveness"`
	ContentUsefulness    *int   `json:"content_usefulness"`
	Suggestions          string `json:"suggestions"`
	JoinToken            string `json:"join_token"` // when not using JWT (e.g. replay link)
}

// Submit handles POST /webinars/:id/feedback. JWT or join_token; user must have attended.
func (h *Handler) Submit(c *gin.Context) {
	webinarID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid webinar id")
		return
	}
	w, err := h.webinarRepo.GetByID(c.Request.Context(), webinarID)
	if err != nil || w == nil {
		response.NotFound(c, "webinar not found")
		return
	}

	var req SubmitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	var reg *models.Registration
	if userIDVal, exists := c.Get(middleware.ContextUserID); exists {
		user, _ := h.authRepo.GetByID(c.Request.Context(), userIDVal.(uuid.UUID))
		if user != nil {
			reg, _ = h.regRepo.GetRegistrationByWebinarAndEmail(c.Request.Context(), webinarID, user.Email)
		}
	}
	if reg == nil && req.JoinToken != "" {
		tok, _ := h.regRepo.GetTokenByToken(c.Request.Context(), req.JoinToken)
		if tok != nil {
			r, _ := h.regRepo.GetRegistrationByID(c.Request.Context(), tok.RegistrationID)
			if r != nil && r.WebinarID == webinarID {
				reg = r
			}
		}
	}
	if reg == nil {
		response.Unauthorized(c, "authentication required (JWT or join_token)")
		return
	}
	if reg.AttendedAt == nil {
		response.Forbidden(c, "you must have attended this webinar to submit feedback")
		return
	}
	if req.SpeakerEffectiveness != nil && (*req.SpeakerEffectiveness < 1 || *req.SpeakerEffectiveness > 5) {
		req.SpeakerEffectiveness = nil
	}
	if req.ContentUsefulness != nil && (*req.ContentUsefulness < 1 || *req.ContentUsefulness > 5) {
		req.ContentUsefulness = nil
	}

	entry := &Entry{
		WebinarID:           webinarID,
		RegistrationID:      reg.ID,
		Rating:              req.Rating,
		SpeakerEffectiveness: req.SpeakerEffectiveness,
		ContentUsefulness:   req.ContentUsefulness,
		Suggestions:         req.Suggestions,
	}
	if err := h.repo.Create(c.Request.Context(), entry); err != nil {
		response.Internal(c, "failed to save feedback")
		return
	}
	response.Created(c, entry)
}

// List handles GET /webinars/:id/feedback. Admin/speaker only.
func (h *Handler) List(c *gin.Context) {
	webinarID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid webinar id")
		return
	}
	list, err := h.repo.ListByWebinar(c.Request.Context(), webinarID)
	if err != nil {
		response.Internal(c, "failed to list feedback")
		return
	}
	response.OK(c, list)
}
