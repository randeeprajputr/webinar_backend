package auth

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/aura-webinar/backend/internal/models"
	"github.com/aura-webinar/backend/pkg/queue"
	"github.com/aura-webinar/backend/pkg/response"
	"github.com/aura-webinar/backend/pkg/utils"
)

// RegisterRequest is the body for POST /auth/register.
type RegisterRequest struct {
	Email       string `json:"email" binding:"required,email"`
	Password    string `json:"password" binding:"required,min=6"`
	FullName    string `json:"full_name" binding:"required"`
	Role        string `json:"role"` // optional, defaults to audience
	Department  string `json:"department"`
	CompanyName string `json:"company_name"`
	ContactNo   string `json:"contact_no"`
	Designation string `json:"designation"`
	Institution string `json:"institution"`
}

// LoginRequest is the body for POST /auth/login.
type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// TokenResponse is the auth response with JWT.
type TokenResponse struct {
	Token string            `json:"token"`
	User  models.UserPublic `json:"user"`
}

// Handler handles auth HTTP endpoints.
type Handler struct {
	repo         *Repository
	jwt          *JWTService
	jobQueue     *queue.Queue
	frontendURL  string
	logger       *zap.Logger
}

// NewHandler creates an auth handler.
func NewHandler(repo *Repository, jwt *JWTService, logger *zap.Logger) *Handler {
	return &Handler{repo: repo, jwt: jwt, logger: logger}
}

// SetEmailQueue configures the job queue and frontend URL for verification emails.
func (h *Handler) SetEmailQueue(q *queue.Queue, frontendURL string) {
	h.jobQueue = q
	h.frontendURL = frontendURL
}

// Register handles POST /auth/register.
func (h *Handler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	role := models.RoleAudience
	if req.Role != "" {
		switch req.Role {
		case "admin":
			role = models.RoleAdmin
		case "speaker":
			role = models.RoleSpeaker
		case "audience":
			role = models.RoleAudience
		default:
			response.BadRequest(c, "invalid role")
			return
		}
	}

	_, err := h.repo.GetByEmail(c.Request.Context(), req.Email)
	if err == nil {
		response.BadRequest(c, "email already registered")
		return
	}

	// First user (bootstrap) gets verified immediately - no verification email
	list, _ := h.repo.List(c.Request.Context())
	skipVerification := len(list) == 0

	hash, err := utils.HashPassword(req.Password)
	if err != nil {
		response.Internal(c, "failed to hash password")
		return
	}

	profile := &CreateUserParams{
		Department:   req.Department,
		CompanyName:  req.CompanyName,
		ContactNo:    req.ContactNo,
		Designation:  req.Designation,
		Institution:  req.Institution,
	}
	user, err := h.repo.Create(c.Request.Context(), req.Email, hash, req.FullName, role, profile, skipVerification)
	if err != nil {
		response.Internal(c, "failed to create user")
		return
	}

	if skipVerification {
		// First user: log in immediately
		token, err := h.jwt.Generate(user.ID, user.Email, string(user.Role))
		if err != nil {
			response.Internal(c, "failed to generate token")
			return
		}
		response.Created(c, TokenResponse{Token: token, User: user.ToPublic()})
		return
	}

	// Generate verification token and send email
	verifyToken, err := generateVerificationToken()
	if err != nil {
		response.Internal(c, "failed to generate verification token")
		return
	}
	expiresAt := time.Now().Add(24 * time.Hour)
	if err := h.repo.SetVerificationToken(c.Request.Context(), user.ID, verifyToken, expiresAt); err != nil {
		h.logger.Warn("set verification token failed", zap.Error(err))
	}
	if h.jobQueue != nil && h.frontendURL != "" {
		verifyURL := h.frontendURL + "/auth/verify?token=" + verifyToken
		payload := queue.EmailPayload{
			EmailType:      models.EmailTypeEmailVerification,
			WebinarID:      uuid.Nil,
			RegistrationID: uuid.Nil,
			RecipientEmail: req.Email,
			RecipientName:  req.FullName,
			VerifyURL:      verifyURL,
			Subject:        "Verify your email address",
		}
		if err := h.jobQueue.EnqueueEmail(c.Request.Context(), payload); err != nil {
			h.logger.Warn("enqueue verification email failed", zap.Error(err))
		}
	}

	response.Created(c, gin.H{
		"message": "Registration successful. Please check your email to verify your account.",
		"user":    user.ToPublic(),
	})
}

// Login handles POST /auth/login.
func (h *Handler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	user, err := h.repo.GetByEmail(c.Request.Context(), req.Email)
	if err != nil {
		response.Unauthorized(c, "invalid email or password")
		return
	}

	if !utils.CheckPassword(req.Password, user.Password) {
		response.Unauthorized(c, "invalid email or password")
		return
	}
	if !user.EmailVerified {
		response.Unauthorized(c, "please verify your email before logging in")
		return
	}

	token, err := h.jwt.Generate(user.ID, user.Email, string(user.Role))
	if err != nil {
		response.Internal(c, "failed to generate token")
		return
	}

	c.JSON(http.StatusOK, response.Body{Success: true, Data: TokenResponse{Token: token, User: user.ToPublic()}})
}

// VerifyEmail handles GET /auth/verify-email?token=X. Activates account and redirects to login.
func (h *Handler) VerifyEmail(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		response.BadRequest(c, "token required")
		return
	}
	user, err := h.repo.VerifyByToken(c.Request.Context(), token)
	if err != nil || user == nil {
		response.BadRequest(c, "invalid or expired verification link")
		return
	}
	// Return success; frontend can redirect to login
	response.OK(c, gin.H{
		"message": "Email verified successfully. You can now log in.",
		"user":    user.ToPublic(),
	})
}

// List handles GET /users (admin only). Returns platform users for e.g. speaker assignment.
func (h *Handler) List(c *gin.Context) {
	list, err := h.repo.List(c.Request.Context())
	if err != nil {
		response.Internal(c, "failed to list users")
		return
	}
	c.JSON(http.StatusOK, response.Body{Success: true, Data: list})
}

func generateVerificationToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b)[:43], nil
}
