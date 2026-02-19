package auth

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aura-webinar/backend/internal/models"
)

// Repository handles user persistence.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates an auth repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// GetByID returns a user by ID.
func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	const q = `SELECT id, email, password_hash, full_name, role,
		COALESCE(department,''), COALESCE(company_name,''), COALESCE(contact_no,''), COALESCE(designation,''), COALESCE(institution,''),
		created_at, updated_at FROM users WHERE id = $1`
	var u models.User
	err := r.pool.QueryRow(ctx, q, id).Scan(&u.ID, &u.Email, &u.Password, &u.FullName, &u.Role,
		&u.Department, &u.CompanyName, &u.ContactNo, &u.Designation, &u.Institution, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// GetByEmail returns a user by email.
func (r *Repository) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	const q = `SELECT id, email, password_hash, full_name, role,
		COALESCE(department,''), COALESCE(company_name,''), COALESCE(contact_no,''), COALESCE(designation,''), COALESCE(institution,''),
		created_at, updated_at FROM users WHERE email = $1`
	var u models.User
	err := r.pool.QueryRow(ctx, q, email).Scan(&u.ID, &u.Email, &u.Password, &u.FullName, &u.Role,
		&u.Department, &u.CompanyName, &u.ContactNo, &u.Designation, &u.Institution, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// List returns all users (id, email, full_name, role, profile fields) for admin e.g. speaker assignment.
func (r *Repository) List(ctx context.Context) ([]models.UserPublic, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, email, full_name, role,
		COALESCE(department,''), COALESCE(company_name,''), COALESCE(contact_no,''), COALESCE(designation,''), COALESCE(institution,''),
		created_at FROM users ORDER BY full_name, email`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []models.UserPublic
	for rows.Next() {
		var u models.UserPublic
		var role string
		if err := rows.Scan(&u.ID, &u.Email, &u.FullName, &role,
			&u.Department, &u.CompanyName, &u.ContactNo, &u.Designation, &u.Institution, &u.CreatedAt); err != nil {
			return nil, err
		}
		u.Role = models.Role(role)
		list = append(list, u)
	}
	return list, rows.Err()
}

// CreateUserParams holds optional profile fields for registration.
type CreateUserParams struct {
	Department   string
	CompanyName  string
	ContactNo    string
	Designation  string
	Institution  string
}

// Create inserts a new user.
func (r *Repository) Create(ctx context.Context, email, passwordHash, fullName string, role models.Role, profile *CreateUserParams) (*models.User, error) {
	const q = `INSERT INTO users (email, password_hash, full_name, role, department, company_name, contact_no, designation, institution)
		VALUES ($1, $2, $3, $4, NULLIF($5,''), NULLIF($6,''), NULLIF($7,''), NULLIF($8,''), NULLIF($9,''))
		RETURNING id, email, password_hash, full_name, role,
		COALESCE(department,''), COALESCE(company_name,''), COALESCE(contact_no,''), COALESCE(designation,''), COALESCE(institution,''),
		created_at, updated_at`
	dep, company, contact, designation, institution := "", "", "", "", ""
	if profile != nil {
		dep, company, contact, designation, institution = profile.Department, profile.CompanyName, profile.ContactNo, profile.Designation, profile.Institution
	}
	var u models.User
	err := r.pool.QueryRow(ctx, q, email, passwordHash, fullName, string(role), dep, company, contact, designation, institution).
		Scan(&u.ID, &u.Email, &u.Password, &u.FullName, &u.Role,
			&u.Department, &u.CompanyName, &u.ContactNo, &u.Designation, &u.Institution, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}
