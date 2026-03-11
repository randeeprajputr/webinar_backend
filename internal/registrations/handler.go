package registrations

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/aura-webinar/backend/internal/auth"
	"github.com/aura-webinar/backend/internal/models"
	"github.com/aura-webinar/backend/internal/waitlist"
	"github.com/aura-webinar/backend/internal/webinars"
	"github.com/aura-webinar/backend/pkg/email"
	"github.com/aura-webinar/backend/pkg/queue"
	"github.com/aura-webinar/backend/pkg/response"
	"github.com/aura-webinar/backend/pkg/storage"
	"github.com/aura-webinar/backend/pkg/utils"
)

// RegisterRequest is the body for POST /webinars/:id/register.
type RegisterRequest struct {
	Email          string            `json:"email" binding:"required,email"`
	FullName       string            `json:"full_name" binding:"required"`
	FormResponses  map[string]string `json:"form_responses,omitempty"` // dynamic fields from audience_form_config
}

// Handler handles registration HTTP endpoints.
type Handler struct {
	repo         *Repository
	webinarRepo  *webinars.Repository
	waitlistRepo *waitlist.Repository
	authRepo     *auth.Repository
	jwtService   *auth.JWTService
	jobQueue     *queue.Queue
	s3Client     *storage.S3
	frontendURL  string
	logger       *zap.Logger
}

// SetS3 sets the S3 client for registration file uploads.
func (h *Handler) SetS3(s3 *storage.S3) {
	h.s3Client = s3
}

// NewHandler creates a registrations handler.
func NewHandler(repo *Repository, webinarRepo *webinars.Repository, logger *zap.Logger) *Handler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Handler{repo: repo, webinarRepo: webinarRepo, logger: logger}
}

// SetWaitlist sets the waitlist repository for capacity enforcement.
func (h *Handler) SetWaitlist(wr *waitlist.Repository) {
	h.waitlistRepo = wr
}

// SetAuth sets auth repo and JWT service for token exchange.
func (h *Handler) SetAuth(authRepo *auth.Repository, jwtService *auth.JWTService) {
	h.authRepo = authRepo
	h.jwtService = jwtService
}

// SetEmailQueue configures the job queue and frontend URL for confirmation emails.
func (h *Handler) SetEmailQueue(q *queue.Queue, frontendURL string) {
	h.jobQueue = q
	h.frontendURL = frontendURL
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

	// Capacity check: if max_audience set and at capacity, add to waitlist instead
	if w.MaxAudience != nil && *w.MaxAudience > 0 && h.waitlistRepo != nil {
		total, _, err := h.repo.CountByWebinar(c.Request.Context(), webinarID)
		if err != nil {
			h.logger.Error("count registrations failed", zap.Error(err), zap.String("webinar_id", webinarID.String()))
			response.Internal(c, "failed to register")
			return
		}
		if total >= *w.MaxAudience {
			// At capacity: add to waitlist
			entry := &waitlist.Entry{
				WebinarID: webinarID,
				Email:     req.Email,
				FullName:  req.FullName,
				ExtraData: extraData,
			}
			if err := h.waitlistRepo.Create(c.Request.Context(), entry); err != nil {
				h.logger.Error("create waitlist entry failed", zap.Error(err), zap.String("webinar_id", webinarID.String()))
				response.Internal(c, "failed to join waitlist")
				return
			}
			response.OK(c, gin.H{
				"status":   "waitlist",
				"message":  "You've been added to the waitlist. We'll notify you if a spot opens up.",
				"waitlist": true,
			})
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
	if h.jobQueue != nil && h.frontendURL != "" {
		fullJoinURL := email.BuildJoinURL(h.frontendURL, webinarID.String(), tokenStr)
		startsAt := w.StartsAt.Format(time.RFC3339)
		payload := queue.EmailPayload{
			EmailType:       models.EmailTypeRegistrationConfirmation,
			WebinarID:       webinarID,
			RegistrationID:  reg.ID,
			RecipientEmail:  reg.Email,
			RecipientName:   reg.FullName,
			WebinarTitle:    w.Title,
			WebinarStartsAt: startsAt,
			JoinURL:         fullJoinURL,
			Subject:         fmt.Sprintf("You're registered: %s", w.Title),
		}
		if err := h.jobQueue.EnqueueEmail(c.Request.Context(), payload); err != nil {
			h.logger.Warn("enqueue confirmation email failed", zap.Error(err))
		}
	}
	response.OK(c, gin.H{
		"registration_id": reg.ID,
		"join_token":      tokenStr,
		"join_url":        joinURL,
		"expires_at":      expiresAt,
	})
}

// ExchangeToken handles POST /auth/exchange-token. Exchanges registration join_token for JWT so audience can join live webinar.
func (h *Handler) ExchangeToken(c *gin.Context) {
	if h.authRepo == nil || h.jwtService == nil {
		response.Internal(c, "auth service not configured")
		return
	}
	var req struct {
		JoinToken string `json:"join_token" binding:"required"`
		WebinarID string `json:"webinar_id" binding:"required,uuid"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "join_token and webinar_id required")
		return
	}
	webinarID, err := uuid.Parse(req.WebinarID)
	if err != nil {
		response.BadRequest(c, "invalid webinar_id")
		return
	}

	tok, err := h.repo.GetTokenByToken(c.Request.Context(), req.JoinToken)
	if err != nil || tok == nil {
		response.Unauthorized(c, "invalid or expired token")
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
	if reg.WebinarID != webinarID {
		response.BadRequest(c, "token not valid for this webinar")
		return
	}

	// Find or create user for this registration (audience needs user_id for WebSocket, questions, polls)
	user, err := h.authRepo.GetByEmail(c.Request.Context(), reg.Email)
	if err != nil || user == nil {
		randomPass, _ := utils.HashPassword(uuid.New().String() + reg.Email)
		user, err = h.authRepo.Create(c.Request.Context(), reg.Email, randomPass, reg.FullName, models.RoleAudience, nil, true)
		if err != nil {
			h.logger.Error("create guest user failed", zap.Error(err), zap.String("email", reg.Email))
			response.Internal(c, "failed to create session")
			return
		}
	}

	token, err := h.jwtService.Generate(user.ID, user.Email, string(user.Role))
	if err != nil {
		response.Internal(c, "failed to generate token")
		return
	}

	response.OK(c, gin.H{"token": token, "user": user.ToPublic()})
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

// UploadFile handles POST /webinars/:id/register/upload (public). Uploads file for registration form, returns URL.
func (h *Handler) UploadFile(c *gin.Context) {
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
	if h.s3Client == nil {
		response.Internal(c, "file upload not configured")
		return
	}
	file, err := c.FormFile("file")
	if err != nil {
		response.BadRequest(c, "missing file (form field: file)")
		return
	}
	if file.Size > storage.MaxRegistrationFileSize {
		response.BadRequest(c, "file size exceeds 5MB limit")
		return
	}
	if !storage.ValidateRegistrationFileType(file.Header.Get("Content-Type"), file.Filename) {
		response.BadRequest(c, "invalid file type: only PDF, DOC, DOCX, and images (jpg, png, webp, gif) allowed")
		return
	}
	contentType := storage.ContentTypeForRegistrationFilename(file.Filename)
	if file.Header.Get("Content-Type") != "" {
		if _, ok := storage.AllowedRegistrationTypes[file.Header.Get("Content-Type")]; ok {
			contentType = file.Header.Get("Content-Type")
		}
	}
	key := storage.RegistrationKey(webinarID.String(), file.Filename)
	rc, err := file.Open()
	if err != nil {
		h.logger.Error("open uploaded file failed", zap.Error(err))
		response.Internal(c, "failed to read file")
		return
	}
	defer rc.Close()
	_, err = h.s3Client.Upload(c.Request.Context(), h.s3Client.UploadAdPresignedBucket(), key, contentType, rc, file.Size, true)
	if err != nil {
		h.logger.Error("registration file upload failed", zap.Error(err), zap.String("webinar_id", webinarID.String()))
		response.Internal(c, "failed to upload file")
		return
	}
	fileURL := h.s3Client.PublicObjectURL(h.s3Client.UploadAdPresignedBucket(), key)
	response.OK(c, gin.H{"file_url": fileURL, "s3_key": key})
}
