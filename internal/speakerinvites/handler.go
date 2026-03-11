package speakerinvites

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/aura-webinar/backend/internal/auth"
	"github.com/aura-webinar/backend/internal/middleware"
	"github.com/aura-webinar/backend/internal/models"
	"github.com/aura-webinar/backend/internal/webinars"
	"github.com/aura-webinar/backend/pkg/queue"
	"github.com/aura-webinar/backend/pkg/response"
	"github.com/aura-webinar/backend/pkg/utils"
)

// InviteRequest is the body for POST /webinars/:id/speakers/invite.
type InviteRequest struct {
	Email string `json:"email" binding:"required,email"`
}

// AcceptRequest is the body for POST /auth/speaker-invite/accept.
type AcceptRequest struct {
	Token       string `json:"token" binding:"required"`
	FullName    string `json:"full_name" binding:"required"`
	Password    string `json:"password" binding:"required,min=6"`
	Designation string `json:"designation"`
	Institution string `json:"institution"`
	ContactNo   string `json:"contact_no"`
}

// Handler handles speaker invitation endpoints.
type Handler struct {
	inviteRepo   *Repository
	webinarRepo  *webinars.Repository
	authRepo     *auth.Repository
	jwtService   *auth.JWTService
	jobQueue     *queue.Queue
	frontendURL  string
	logger       *zap.Logger
}

// NewHandler creates a speaker invites handler.
func NewHandler(inviteRepo *Repository, webinarRepo *webinars.Repository, authRepo *auth.Repository, jwtService *auth.JWTService, logger *zap.Logger) *Handler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Handler{inviteRepo: inviteRepo, webinarRepo: webinarRepo, authRepo: authRepo, jwtService: jwtService, logger: logger}
}

// SetEmailQueue configures the job queue and frontend URL for invitation emails.
func (h *Handler) SetEmailQueue(q *queue.Queue, frontendURL string) {
	h.jobQueue = q
	h.frontendURL = frontendURL
}

// Invite handles POST /webinars/:id/speakers/invite. Creates invitation and sends email.
func (h *Handler) Invite(c *gin.Context) {
	webinarID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid webinar id")
		return
	}
	userID := c.MustGet(middleware.ContextUserID).(uuid.UUID)

	ok, err := h.webinarRepo.IsAdminOrSpeaker(c.Request.Context(), webinarID, userID)
	if err != nil || !ok {
		response.Forbidden(c, "only admin or webinar creator can invite speakers")
		return
	}

	w, err := h.webinarRepo.GetByID(c.Request.Context(), webinarID)
	if err != nil || w == nil {
		response.NotFound(c, "webinar not found")
		return
	}

	var req InviteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "email required")
		return
	}

	// Check if user already exists and is speaker - add directly
	existingUser, err := h.authRepo.GetByEmail(c.Request.Context(), req.Email)
	if err == nil && existingUser != nil {
		if err := h.webinarRepo.AddSpeaker(c.Request.Context(), webinarID, existingUser.ID); err == nil {
			response.Created(c, gin.H{"message": "Speaker added", "user_id": existingUser.ID})
			return
		}
	}

	inv, err := h.inviteRepo.Create(c.Request.Context(), webinarID, req.Email)
	if err != nil {
		h.logger.Error("create invitation failed", zap.Error(err))
		response.Internal(c, "failed to create invitation")
		return
	}

	if h.jobQueue != nil && h.frontendURL != "" {
		inviteURL := h.frontendURL + "/auth/speaker-invite?token=" + inv.Token
		payload := queue.EmailPayload{
			EmailType:      models.EmailTypeSpeakerInvitation,
			WebinarID:      webinarID,
			RegistrationID: uuid.Nil,
			RecipientEmail: req.Email,
			WebinarTitle:   w.Title,
			InviteURL:      inviteURL,
			Subject:        "You're invited to speak: " + w.Title,
		}
		if err := h.jobQueue.EnqueueEmail(c.Request.Context(), payload); err != nil {
			h.logger.Warn("enqueue invite email failed", zap.Error(err))
		}
	}

	response.Created(c, gin.H{"message": "Invitation sent", "email": req.Email})
}

// GetInviteByToken handles GET /auth/speaker-invite/validate?token=X. Returns invite info for the signup page.
func (h *Handler) GetInviteByToken(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		response.BadRequest(c, "token required")
		return
	}
	inv, err := h.inviteRepo.GetByToken(c.Request.Context(), token)
	if err != nil || inv == nil {
		response.NotFound(c, "invalid or expired invitation")
		return
	}
	w, _ := h.webinarRepo.GetByID(c.Request.Context(), inv.WebinarID)
	webinarTitle := ""
	if w != nil {
		webinarTitle = w.Title
	}
	response.OK(c, gin.H{
		"valid":          true,
		"email":          inv.Email,
		"webinar_id":     inv.WebinarID,
		"webinar_title":  webinarTitle,
	})
}

// AcceptInvite handles POST /auth/speaker-invite/accept. Creates user (or uses existing), adds to webinar, returns JWT.
func (h *Handler) AcceptInvite(c *gin.Context) {
	var req AcceptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "token, full_name, and password required")
		return
	}

	inv, err := h.inviteRepo.GetByToken(c.Request.Context(), req.Token)
	if err != nil || inv == nil {
		response.BadRequest(c, "invalid or expired invitation")
		return
	}

	// Find or create user
	user, err := h.authRepo.GetByEmail(c.Request.Context(), inv.Email)
	if err != nil || user == nil {
		hash, err := utils.HashPassword(req.Password)
		if err != nil {
			response.Internal(c, "failed to hash password")
			return
		}
		profile := &auth.CreateUserParams{
			Designation: req.Designation,
			Institution: req.Institution,
			ContactNo:   req.ContactNo,
		}
		user, err = h.authRepo.Create(c.Request.Context(), inv.Email, hash, req.FullName, models.RoleSpeaker, profile, true)
		if err != nil {
			h.logger.Error("create speaker failed", zap.Error(err))
			response.Internal(c, "failed to create account")
			return
		}
	} else {
		if !utils.CheckPassword(req.Password, user.Password) {
			response.Unauthorized(c, "invalid password for existing account")
			return
		}
	}

	if err := h.webinarRepo.AddSpeaker(c.Request.Context(), inv.WebinarID, user.ID); err != nil {
		h.logger.Warn("add speaker failed", zap.Error(err))
	}
	_ = h.inviteRepo.MarkAccepted(c.Request.Context(), inv.ID)

	token, err := h.jwtService.Generate(user.ID, user.Email, string(user.Role))
	if err != nil {
		response.Internal(c, "failed to generate token")
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"token": token, "user": user.ToPublic()}})
}
