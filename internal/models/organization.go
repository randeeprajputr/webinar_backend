package models

import (
	"time"

	"github.com/google/uuid"
)

// Organization represents a tenant (SaaS foundation).
type Organization struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// OrganizationUserRole is the role of a user in an organization.
const (
	OrgRoleOwner       = "owner"
	OrgRoleEventManager = "event_manager"
	OrgRoleModerator   = "moderator"
)

// OrganizationUser links a user to an organization with a role.
type OrganizationUser struct {
	ID             uuid.UUID `json:"id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	UserID         uuid.UUID `json:"user_id"`
	Role           string    `json:"role"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}
