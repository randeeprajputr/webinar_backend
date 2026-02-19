package registrations

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/aura-webinar/backend/internal/models"
	"github.com/aura-webinar/backend/internal/webinars"
	"github.com/aura-webinar/backend/pkg/response"
)

// RegisterRequest is the body for POST /webinars/:id/register.
type RegisterRequest struct {
	Email          string            `json:"email" binding:"required,email"`
	FullName       string            `json:"full_name" binding:"required"`
	FormResponses  map[string]string `json:"form_responses,omitempty"` // dynamic fields from audience_form_config
}

// Handler handles registration HTTP endpoints.
type Handler struct {
	repo       *Repository
	webinarRepo *webinars.Repository
	logger     *zap.Logger
}

// NewHandler creates a registrations handler.
func NewHandler(repo *Repository, webinarRepo *webinars.Repository, logger *zap.Logger) *Handler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Handler{repo: repo, webinarRepo: webinarRepo, logger: logger}
}

// Register handles POST /webinars/:id/register. Creates registration and unique join token.
func (h *Handler) Register(c *gin.Context) {
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

	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	var extraData json.RawMessage
	if len(req.FormResponses) > 0 {
		var err error
		extraData, err = json.Marshal(req.FormResponses)
		if err != nil {
			response.BadRequest(c, "invalid form_responses")
			return
		}
	}
	reg := &models.Registration{
		WebinarID: webinarID,
		Email:     req.Email,
		FullName:  req.FullName,
		ExtraData: extraData,
	}
	if err := h.repo.CreateRegistration(c.Request.Context(), reg); err != nil {
		h.logger.Error("create registration failed", zap.Error(err), zap.String("webinar_id", webinarID.String()))
		response.Internal(c, "failed to register")
		return
	}

	tokenStr, err := generateToken()
	if err != nil {
		h.logger.Error("generate token failed", zap.Error(err))
		response.Internal(c, "failed to generate join link")
		return
	}
	expiresAt := time.Now().Add(30 * 24 * time.Hour) // 30 days
	tok := &models.RegistrationToken{
		RegistrationID: reg.ID,
		Token:          tokenStr,
		ExpiresAt:      expiresAt,
	}
	if err := h.repo.CreateToken(c.Request.Context(), tok); err != nil {
		h.logger.Error("create token failed", zap.Error(err), zap.String("registration_id", reg.ID.String()))
		response.Internal(c, "failed to create join link")
		return
	}

	joinURL := "/audience?webinar_id=" + webinarID.String() + "&token=" + tokenStr
	response.OK(c, gin.H{
		"registration_id": reg.ID,
		"join_token":      tokenStr,
		"join_url":        joinURL,
		"expires_at":      expiresAt,
	})
}

// ValidateToken handles GET /registrations/:token/validate. Returns registration + webinar info if token valid.
func (h *Handler) ValidateToken(c *gin.Context) {
	tokenStr := c.Param("token")
	if tokenStr == "" {
		response.BadRequest(c, "token required")
		return
	}

	tok, err := h.repo.GetTokenByToken(c.Request.Context(), tokenStr)
	if err != nil || tok == nil {
		response.NotFound(c, "invalid or expired token")
		return
	}
	if tok.UsedAt != nil {
		response.BadRequest(c, "token already used")
		return
	}
	if time.Now().After(tok.ExpiresAt) {
		response.BadRequest(c, "token expired")
		return
	}

	reg, err := h.repo.GetRegistrationByID(c.Request.Context(), tok.RegistrationID)
	if err != nil || reg == nil {
		response.NotFound(c, "registration not found")
		return
	}

	w, err := h.webinarRepo.GetByID(c.Request.Context(), reg.WebinarID)
	if err != nil || w == nil {
		response.NotFound(c, "webinar not found")
		return
	}

	response.OK(c, gin.H{
		"valid":             true,
		"registration":      reg,
		"webinar_id":        w.ID,
		"webinar_title":     w.Title,
		"webinar_starts_at": w.StartsAt,
	})
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b)[:43], nil
}
