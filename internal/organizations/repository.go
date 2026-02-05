package organizations

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aura-webinar/backend/internal/models"
)

// Repository handles organization and organization_user persistence.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates an organizations repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Create creates an organization.
func (r *Repository) Create(ctx context.Context, org *models.Organization) error {
	const q = `INSERT INTO organizations (id, name, slug)
		VALUES (gen_random_uuid(), $1, $2)
		RETURNING id, created_at, updated_at`
	return r.pool.QueryRow(ctx, q, org.Name, org.Slug).
		Scan(&org.ID, &org.CreatedAt, &org.UpdatedAt)
}

// GetByID returns an organization by ID.
func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*models.Organization, error) {
	const q = `SELECT id, name, slug, created_at, updated_at FROM organizations WHERE id = $1`
	var org models.Organization
	err := r.pool.QueryRow(ctx, q, id).Scan(&org.ID, &org.Name, &org.Slug, &org.CreatedAt, &org.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &org, nil
}

// GetBySlug returns an organization by slug.
func (r *Repository) GetBySlug(ctx context.Context, slug string) (*models.Organization, error) {
	const q = `SELECT id, name, slug, created_at, updated_at FROM organizations WHERE slug = $1`
	var org models.Organization
	err := r.pool.QueryRow(ctx, q, slug).Scan(&org.ID, &org.Name, &org.Slug, &org.CreatedAt, &org.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &org, nil
}

// AddUser adds a user to an organization with a role.
func (r *Repository) AddUser(ctx context.Context, orgID, userID uuid.UUID, role string) error {
	const q = `INSERT INTO organization_users (id, organization_id, user_id, role)
		VALUES (gen_random_uuid(), $1, $2, $3)
		ON CONFLICT (organization_id, user_id) DO UPDATE SET role = EXCLUDED.role, updated_at = NOW()`
	_, err := r.pool.Exec(ctx, q, orgID, userID, role)
	return err
}

// GetUserRole returns the user's role in the organization, or empty if not a member.
func (r *Repository) GetUserRole(ctx context.Context, orgID, userID uuid.UUID) (string, error) {
	const q = `SELECT role FROM organization_users WHERE organization_id = $1 AND user_id = $2`
	var role string
	err := r.pool.QueryRow(ctx, q, orgID, userID).Scan(&role)
	if err != nil {
		return "", err
	}
	return role, nil
}

// UserHasOrgAccess returns true if user is owner, event_manager, or moderator in the org.
func (r *Repository) UserHasOrgAccess(ctx context.Context, orgID, userID uuid.UUID) (bool, error) {
	role, err := r.GetUserRole(ctx, orgID, userID)
	if err != nil || role == "" {
		return false, nil
	}
	return role == models.OrgRoleOwner || role == models.OrgRoleEventManager || role == models.OrgRoleModerator, nil
}

// ListOrganizationsForUser returns organizations the user is a member of (for GET /organizations).
func (r *Repository) ListOrganizationsForUser(ctx context.Context, userID uuid.UUID) ([]*models.Organization, error) {
	const q = `SELECT o.id, o.name, o.slug, o.created_at, o.updated_at
		FROM organizations o
		INNER JOIN organization_users ou ON ou.organization_id = o.id
		WHERE ou.user_id = $1
		ORDER BY o.name`
	rows, err := r.pool.Query(ctx, q, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []*models.Organization
	for rows.Next() {
		var o models.Organization
		if err := rows.Scan(&o.ID, &o.Name, &o.Slug, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, &o)
	}
	return list, rows.Err()
}

// ListOrganizationsByUser returns organization IDs the user belongs to.
func (r *Repository) ListOrganizationsByUser(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.pool.Query(ctx, `SELECT organization_id FROM organization_users WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// Member represents an organization member with user details (for GET /organizations/:id/members).
type Member struct {
	ID       uuid.UUID `json:"id"`
	UserID   uuid.UUID `json:"user_id"`
	Email    string    `json:"email"`
	FullName string    `json:"full_name"`
	Role     string    `json:"role"`
	AddedAt  time.Time `json:"added_at"`
}

// ListMembers returns members of an organization (join organization_users + users).
func (r *Repository) ListMembers(ctx context.Context, orgID uuid.UUID) ([]Member, error) {
	const q = `SELECT ou.id, ou.user_id, u.email, COALESCE(u.full_name, ''), ou.role, ou.created_at
		FROM organization_users ou
		INNER JOIN users u ON u.id = ou.user_id
		WHERE ou.organization_id = $1
		ORDER BY ou.created_at ASC`
	rows, err := r.pool.Query(ctx, q, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Member
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.ID, &m.UserID, &m.Email, &m.FullName, &m.Role, &m.AddedAt); err != nil {
			return nil, err
		}
		list = append(list, m)
	}
	return list, rows.Err()
}