package zego

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/aura-webinar/backend/config"
	"github.com/aura-webinar/backend/internal/middleware"
	"github.com/aura-webinar/backend/internal/webinars"
	"github.com/aura-webinar/backend/pkg/response"
)

const tokenValidSec = 3600 * 24 // 24 hours

// Handler handles ZEGOCLOUD token and related endpoints.
type Handler struct {
	webinarRepo *webinars.Repository
	cfg         config.ZegoConfig
	logger      *zap.Logger
}

// NewHandler creates a ZEGO handler.
func NewHandler(webinarRepo *webinars.Repository, cfg config.ZegoConfig, logger *zap.Logger) *Handler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Handler{webinarRepo: webinarRepo, cfg: cfg, logger: logger}
}

// GetToken handles GET /webinars/:id/zego-token?role=speaker|audience.
// Returns { token, app_id } for ZEGOCLOUD SDK (live streaming). JWT required.
func (h *Handler) GetToken(c *gin.Context) {
	if h.cfg.AppID == 0 || h.cfg.ServerSecret == "" {
		response.ServiceUnavailable(c, "ZEGOCLOUD not configured (ZEGO_APP_ID, ZEGO_SERVER_SECRET)")
		return
	}
	webinarID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid webinar id")
		return
	}
	userID := c.MustGet(middleware.ContextUserID).(uuid.UUID)
	roleParam := c.Query("role")
	if roleParam == "" {
		roleParam = "audience"
	}
	if roleParam != "speaker" && roleParam != "audience" {
		response.BadRequest(c, "role must be speaker or audience")
		return
	}
	// Speaker token: only admin or speaker for this webinar
	if roleParam == "speaker" {
		ok, err := h.webinarRepo.IsAdminOrSpeaker(c.Request.Context(), webinarID, userID)
		if err != nil || !ok {
			response.Forbidden(c, "not authorized to stream as speaker")
			return
		}
	}

	roomID := webinarID.String()
	userIDStr := userID.String()
	token, err := GenerateRoomToken(
		h.cfg.AppID,
		h.cfg.ServerSecret,
		roomID,
		userIDStr,
		roleParam,
		tokenValidSec,
	)
	if err != nil {
		h.logger.Error("zego token generation failed", zap.Error(err), zap.String("webinar_id", webinarID.String()))
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to generate token"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"token":   token,
		"app_id":  h.cfg.AppID,
	})
}
