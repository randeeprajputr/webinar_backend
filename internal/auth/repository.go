package auth

import (
	"context"
	"time"

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
	const q = `SELECT id, email, password_hash, full_name, role, COALESCE(email_verified, true),
		COALESCE(department,''), COALESCE(company_name,''), COALESCE(contact_no,''), COALESCE(designation,''), COALESCE(institution,''),
		created_at, updated_at FROM users WHERE id = $1`
	var u models.User
	err := r.pool.QueryRow(ctx, q, id).Scan(&u.ID, &u.Email, &u.Password, &u.FullName, &u.Role, &u.EmailVerified,
		&u.Department, &u.CompanyName, &u.ContactNo, &u.Designation, &u.Institution, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// GetByEmail returns a user by email.
func (r *Repository) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	const q = `SELECT id, email, password_hash, full_name, role, COALESCE(email_verified, true),
		COALESCE(department,''), COALESCE(company_name,''), COALESCE(contact_no,''), COALESCE(designation,''), COALESCE(institution,''),
		created_at, updated_at FROM users WHERE email = $1`
	var u models.User
	err := r.pool.QueryRow(ctx, q, email).Scan(&u.ID, &u.Email, &u.Password, &u.FullName, &u.Role, &u.EmailVerified,
		&u.Department, &u.CompanyName, &u.ContactNo, &u.Designation, &u.Institution, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// List returns all users (id, email, full_name, role, profile fields) for admin e.g. speaker assignment.
func (r *Repository) List(ctx context.Context) ([]models.UserPublic, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, email, full_name, role, COALESCE(email_verified, true),
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
		if err := rows.Scan(&u.ID, &u.Email, &u.FullName, &role, &u.EmailVerified,
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

// Create inserts a new user. emailVerified: false for new signups requiring verification.
func (r *Repository) Create(ctx context.Context, email, passwordHash, fullName string, role models.Role, profile *CreateUserParams, emailVerified bool) (*models.User, error) {
	const q = `INSERT INTO users (email, password_hash, full_name, role, email_verified, department, company_name, contact_no, designation, institution)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6,''), NULLIF($7,''), NULLIF($8,''), NULLIF($9,''), NULLIF($10,''))
		RETURNING id, email, password_hash, full_name, role, COALESCE(email_verified, true),
		COALESCE(department,''), COALESCE(company_name,''), COALESCE(contact_no,''), COALESCE(designation,''), COALESCE(institution,''),
		created_at, updated_at`
	dep, company, contact, designation, institution := "", "", "", "", ""
	if profile != nil {
		dep, company, contact, designation, institution = profile.Department, profile.CompanyName, profile.ContactNo, profile.Designation, profile.Institution
	}
	var u models.User
	err := r.pool.QueryRow(ctx, q, email, passwordHash, fullName, string(role), emailVerified, dep, company, contact, designation, institution).
		Scan(&u.ID, &u.Email, &u.Password, &u.FullName, &u.Role, &u.EmailVerified,
			&u.Department, &u.CompanyName, &u.ContactNo, &u.Designation, &u.Institution, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// SetVerificationToken stores token and expiry for email verification.
func (r *Repository) SetVerificationToken(ctx context.Context, userID uuid.UUID, token string, expiresAt time.Time) error {
	const q = `UPDATE users SET email_verification_token = $2, email_verification_expires_at = $3, updated_at = NOW() WHERE id = $1`
	_, err := r.pool.Exec(ctx, q, userID, token, expiresAt)
	return err
}

// VerifyByToken marks user as verified if token matches and not expired. Returns user or nil.
func (r *Repository) VerifyByToken(ctx context.Context, token string) (*models.User, error) {
	const q = `UPDATE users SET email_verified = true, email_verification_token = NULL, email_verification_expires_at = NULL, updated_at = NOW()
		WHERE email_verification_token = $1 AND (email_verification_expires_at IS NULL OR email_verification_expires_at > NOW())
		RETURNING id, email, password_hash, full_name, role, true,
		COALESCE(department,''), COALESCE(company_name,''), COALESCE(contact_no,''), COALESCE(designation,''), COALESCE(institution,''),
		created_at, updated_at`
	var u models.User
	err := r.pool.QueryRow(ctx, q, token).Scan(&u.ID, &u.Email, &u.Password, &u.FullName, &u.Role, &u.EmailVerified,
		&u.Department, &u.CompanyName, &u.ContactNo, &u.Designation, &u.Institution, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}
