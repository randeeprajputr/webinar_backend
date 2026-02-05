package webinars

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/aura-webinar/backend/internal/middleware"
	"github.com/aura-webinar/backend/internal/organizations"
	"github.com/aura-webinar/backend/pkg/response"
)

// ContextOrganizationID is the context key for organization ID when org access is enforced.
const ContextOrganizationID = "organization_id"

// RequireWebinarOrgAccess validates that the user has access to the webinar's organization (if any).
// Call after JWT. If webinar has no organization_id, allows. Otherwise requires org membership.
func RequireWebinarOrgAccess(webinarRepo *Repository, orgRepo *organizations.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		webinarIDStr := c.Param("id")
		if webinarIDStr == "" {
			c.Next()
			return
		}
		webinarID, err := uuid.Parse(webinarIDStr)
		if err != nil {
			response.BadRequest(c, "invalid webinar id")
			c.Abort()
			return
		}
		w, err := webinarRepo.GetByID(c.Request.Context(), webinarID)
		if err != nil || w == nil {
			response.NotFound(c, "webinar not found")
			c.Abort()
			return
		}
		userID := c.MustGet(middleware.ContextUserID).(uuid.UUID)
		if w.OrganizationID == nil {
			c.Next()
			return
		}
		ok, _ := orgRepo.UserHasOrgAccess(c.Request.Context(), *w.OrganizationID, userID)
		if !ok {
			response.Forbidden(c, "not authorized for this organization")
			c.Abort()
			return
		}
		c.Set(ContextOrganizationID, *w.OrganizationID)
		c.Next()
	}
}
