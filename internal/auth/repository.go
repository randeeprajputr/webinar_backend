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
	const q = `SELECT id, email, password_hash, full_name, role, created_at, updated_at
		FROM users WHERE id = $1`
	var u models.User
	err := r.pool.QueryRow(ctx, q, id).Scan(&u.ID, &u.Email, &u.Password, &u.FullName, &u.Role, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// GetByEmail returns a user by email.
func (r *Repository) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	const q = `SELECT id, email, password_hash, full_name, role, created_at, updated_at
		FROM users WHERE email = $1`
	var u models.User
	err := r.pool.QueryRow(ctx, q, email).Scan(&u.ID, &u.Email, &u.Password, &u.FullName, &u.Role, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// List returns all users (id, email, full_name, role) for admin e.g. speaker assignment.
func (r *Repository) List(ctx context.Context) ([]models.UserPublic, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, email, full_name, role, created_at FROM users ORDER BY full_name, email`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []models.UserPublic
	for rows.Next() {
		var u models.UserPublic
		var role string
		if err := rows.Scan(&u.ID, &u.Email, &u.FullName, &role, &u.CreatedAt); err != nil {
			return nil, err
		}
		u.Role = models.Role(role)
		list = append(list, u)
	}
	return list, rows.Err()
}

// Create inserts a new user.
func (r *Repository) Create(ctx context.Context, email, passwordHash, fullName string, role models.Role) (*models.User, error) {
	const q = `INSERT INTO users (email, password_hash, full_name, role)
		VALUES ($1, $2, $3, $4)
		RETURNING id, email, password_hash, full_name, role, created_at, updated_at`
	var u models.User
	err := r.pool.QueryRow(ctx, q, email, passwordHash, fullName, string(role)).
		Scan(&u.ID, &u.Email, &u.Password, &u.FullName, &u.Role, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}
