package certificates

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/aura-webinar/backend/internal/auth"
	"github.com/aura-webinar/backend/internal/registrations"
	"github.com/aura-webinar/backend/internal/sessionlog"
	"github.com/aura-webinar/backend/internal/webinars"
	"github.com/aura-webinar/backend/pkg/response"
)

// MinWatchSeconds is the minimum watch time to qualify for a certificate (40 minutes).
const MinWatchSeconds = 40 * 60

// Handler handles certificate endpoints.
type Handler struct {
	webinarRepo  *webinars.Repository
	regRepo      *registrations.Repository
	sessionRepo  *sessionlog.Repository
	authRepo     *auth.Repository
}

// NewHandler creates a certificate handler.
func NewHandler(webinarRepo *webinars.Repository, regRepo *registrations.Repository, sessionRepo *sessionlog.Repository, authRepo *auth.Repository) *Handler {
	return &Handler{webinarRepo: webinarRepo, regRepo: regRepo, sessionRepo: sessionRepo, authRepo: authRepo}
}

// ValidateCertificate handles GET /webinars/:id/certificate/validate?token=X.
// Returns certificate data if the attendee qualifies (attended + watched >= 40 min).
func (h *Handler) ValidateCertificate(c *gin.Context) {
	webinarID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid webinar id")
		return
	}
	token := c.Query("token")
	if token == "" {
		response.BadRequest(c, "token required")
		return
	}

	w, err := h.webinarRepo.GetByID(c.Request.Context(), webinarID)
	if err != nil || w == nil {
		response.NotFound(c, "webinar not found")
		return
	}
	tok, err := h.regRepo.GetTokenByToken(c.Request.Context(), token)
	if err != nil || tok == nil {
		response.NotFound(c, "invalid or expired token")
		return
	}
	reg, err := h.regRepo.GetRegistrationByID(c.Request.Context(), tok.RegistrationID)
	if err != nil || reg == nil || reg.WebinarID != webinarID {
		response.NotFound(c, "registration not found")
		return
	}
	if reg.AttendedAt == nil {
		response.Forbidden(c, "you must have attended this webinar to receive a certificate")
		return
	}

	// Find user by email to get watch time
	user, err := h.authRepo.GetByEmail(c.Request.Context(), reg.Email)
	if err != nil || user == nil {
		// No user account - use attended_at as qualification (simplified)
		response.OK(c, gin.H{
			"valid":          true,
			"name":           reg.FullName,
			"webinar_title":  w.Title,
			"webinar_date":   w.StartsAt.Format("January 2, 2006"),
			"qualification":  "attended",
		})
		return
	}

	totalWatch, err := h.sessionRepo.GetTotalWatchSecondsByUser(c.Request.Context(), webinarID, user.ID)
	if err != nil {
		response.Internal(c, "failed to verify attendance")
		return
	}
	if totalWatch < MinWatchSeconds {
		response.Forbidden(c, "certificate requires at least 40 minutes of watch time")
		return
	}

	response.OK(c, gin.H{
		"valid":          true,
		"name":           reg.FullName,
		"webinar_title":  w.Title,
		"webinar_date":   w.StartsAt.Format("January 2, 2006"),
		"qualification":  "attended",
		"watch_minutes":   int(totalWatch / 60),
	})
}

// CertificateHTML returns a simple HTML certificate page for printing.
func (h *Handler) CertificateHTML(c *gin.Context) {
	webinarID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.String(http.StatusBadRequest, "invalid webinar id")
		return
	}
	token := c.Query("token")
	if token == "" {
		c.String(http.StatusBadRequest, "token required")
		return
	}

	w, err := h.webinarRepo.GetByID(c.Request.Context(), webinarID)
	if err != nil || w == nil {
		c.String(http.StatusNotFound, "webinar not found")
		return
	}
	tok, err := h.regRepo.GetTokenByToken(c.Request.Context(), token)
	if err != nil || tok == nil {
		c.String(http.StatusNotFound, "invalid or expired token")
		return
	}
	reg, err := h.regRepo.GetRegistrationByID(c.Request.Context(), tok.RegistrationID)
	if err != nil || reg == nil || reg.WebinarID != webinarID {
		c.String(http.StatusNotFound, "registration not found")
		return
	}
	if reg.AttendedAt == nil {
		c.String(http.StatusForbidden, "you must have attended this webinar")
		return
	}

	user, _ := h.authRepo.GetByEmail(c.Request.Context(), reg.Email)
	if user != nil {
		totalWatch, _ := h.sessionRepo.GetTotalWatchSecondsByUser(c.Request.Context(), webinarID, user.ID)
		if totalWatch < MinWatchSeconds {
			c.String(http.StatusForbidden, "certificate requires at least 40 minutes of watch time")
			return
		}
	}

	dateStr := w.StartsAt.Format("January 2, 2006")
	html := `<!DOCTYPE html><html><head><meta charset="UTF-8"><title>Certificate</title>
<style>
body{font-family:Georgia,serif;max-width:800px;margin:60px auto;padding:40px;text-align:center;background:#fafafa;}
.cert{border:3px solid #0ea5e9;border-radius:12px;padding:60px;background:linear-gradient(135deg,#fff 0%,#f0f9ff 100%);}
h1{color:#0c4a6e;font-size:2rem;margin:0 0 8px;}
h2{color:#0369a1;font-size:1.25rem;font-weight:normal;margin:0 0 40px;}
.name{font-size:2.5rem;font-weight:bold;color:#0c4a6e;margin:20px 0;}
.detail{color:#64748b;font-size:1rem;margin:8px 0;}
@media print{body{background:#fff;margin:0;padding:20px;}.cert{border-color:#0ea5e9;box-shadow:none;}}
</style></head><body>
<div class="cert">
<h1>Certificate of Attendance</h1>
<h2>This certificate is presented to</h2>
<p class="name">` + reg.FullName + `</p>
<p class="detail">for attending the webinar</p>
<p class="detail">` + w.Title + `</p>
<p class="detail">` + dateStr + `</p>
<p class="detail" style="margin-top:40px;font-size:0.9rem;">Aura Webinar Platform</p>
</div>
<p style="margin-top:20px;font-size:0.85rem;color:#94a3b8;">Print this page (Ctrl+P / Cmd+P) to save as PDF</p>
</body></html>`
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, html)
}
