package organizations

import (
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/aura-webinar/backend/internal/middleware"
	"github.com/aura-webinar/backend/internal/models"
	"github.com/aura-webinar/backend/pkg/response"
)

// Slug must be lowercase alphanumeric and hyphens only, 2–64 chars.
var slugRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,63}$`)

// Handler handles organization HTTP endpoints.
type Handler struct {
	repo *Repository
}

// NewHandler creates an organizations handler.
func NewHandler(repo *Repository) *Handler {
	return &Handler{repo: repo}
}

// CreateOrganizationRequest is the body for POST /organizations.
type CreateOrganizationRequest struct {
	Name string `json:"name" binding:"required"`
	Slug string `json:"slug" binding:"required"`
}

// JoinOrganizationRequest is the body for POST /organizations/join.
type JoinOrganizationRequest struct {
	Slug string `json:"slug" binding:"required"`
}

// CreateOrganization handles POST /organizations. Creates org and adds current user as owner.
func (h *Handler) CreateOrganization(c *gin.Context) {
	userID := c.MustGet(middleware.ContextUserID).(uuid.UUID)
	var body CreateOrganizationRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		response.BadRequest(c, "name and slug required")
		return
	}
	body.Slug = strings.ToLower(strings.TrimSpace(body.Slug))
	if !slugRegex.MatchString(body.Slug) {
		response.BadRequest(c, "slug must be 2–64 chars, lowercase letters, numbers, hyphens only")
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if len(body.Name) < 1 || len(body.Name) > 255 {
		response.BadRequest(c, "name must be 1–255 characters")
		return
	}
	org := &models.Organization{Name: body.Name, Slug: body.Slug}
	if err := h.repo.Create(c.Request.Context(), org); err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique") {
			response.Conflict(c, "An organization with this slug already exists")
			return
		}
		response.Internal(c, "failed to create organization")
		return
	}
	if err := h.repo.AddUser(c.Request.Context(), org.ID, userID, models.OrgRoleOwner); err != nil {
		response.Internal(c, "failed to add you as owner")
		return
	}
	response.OK(c, org)
}

// JoinOrganization handles POST /organizations/join. Adds current user to org by slug (as moderator).
func (h *Handler) JoinOrganization(c *gin.Context) {
	userID := c.MustGet(middleware.ContextUserID).(uuid.UUID)
	var body JoinOrganizationRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		response.BadRequest(c, "slug required")
		return
	}
	slug := strings.ToLower(strings.TrimSpace(body.Slug))
	if slug == "" {
		response.BadRequest(c, "slug required")
		return
	}
	org, err := h.repo.GetBySlug(c.Request.Context(), slug)
	if err != nil || org == nil {
		response.NotFound(c, "Organization not found")
		return
	}
	if err := h.repo.AddUser(c.Request.Context(), org.ID, userID, models.OrgRoleModerator); err != nil {
		response.Internal(c, "failed to join organization")
		return
	}
	response.OK(c, org)
}

// ListMyOrganizations handles GET /organizations. Returns orgs the current user is a member of.
func (h *Handler) ListMyOrganizations(c *gin.Context) {
	userID := c.MustGet(middleware.ContextUserID).(uuid.UUID)
	orgs, err := h.repo.ListOrganizationsForUser(c.Request.Context(), userID)
	if err != nil {
		response.Internal(c, "failed to load organizations")
		return
	}
	response.OK(c, orgs)
}

// ListMembers handles GET /organizations/:id/members. Requires JWT and org access (owner/event_manager/moderator).
func (h *Handler) ListMembers(c *gin.Context) {
	idStr := c.Param("id")
	orgID, err := uuid.Parse(idStr)
	if err != nil {
		response.BadRequest(c, "invalid organization id")
		return
	}
	userID := c.MustGet(middleware.ContextUserID).(uuid.UUID)
	ok, err := h.repo.UserHasOrgAccess(c.Request.Context(), orgID, userID)
	if err != nil || !ok {
		response.Forbidden(c, "not authorized for this organization")
		return
	}
	members, err := h.repo.ListMembers(c.Request.Context(), orgID)
	if err != nil {
		response.Internal(c, "failed to load members")
		return
	}
	response.OK(c, members)
}
