package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/aura-webinar/backend/internal/models"
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
	repo   *Repository
	jwt    *JWTService
	logger *zap.Logger
}

// NewHandler creates an auth handler.
func NewHandler(repo *Repository, jwt *JWTService, logger *zap.Logger) *Handler {
	return &Handler{repo: repo, jwt: jwt, logger: logger}
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
	user, err := h.repo.Create(c.Request.Context(), req.Email, hash, req.FullName, role, profile)
	if err != nil {
		response.Internal(c, "failed to create user")
		return
	}

	token, err := h.jwt.Generate(user.ID, user.Email, string(user.Role))
	if err != nil {
		response.Internal(c, "failed to generate token")
		return
	}

	response.Created(c, TokenResponse{Token: token, User: user.ToPublic()})
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

	token, err := h.jwt.Generate(user.ID, user.Email, string(user.Role))
	if err != nil {
		response.Internal(c, "failed to generate token")
		return
	}

	c.JSON(http.StatusOK, response.Body{Success: true, Data: TokenResponse{Token: token, User: user.ToPublic()}})
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
