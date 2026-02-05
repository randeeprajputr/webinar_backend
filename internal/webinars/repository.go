package webinars

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aura-webinar/backend/internal/models"
)

// Repository handles webinar persistence.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a webinar repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Create inserts a new webinar.
func (r *Repository) Create(ctx context.Context, w *models.Webinar) error {
	const q = `INSERT INTO webinars (id, title, description, starts_at, ends_at, created_by, organization_id, is_paid, ticket_price_cents, ticket_currency)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at, updated_at`
	return r.pool.QueryRow(ctx, q, w.Title, w.Description, w.StartsAt, w.EndsAt, w.CreatedBy, w.OrganizationID, w.IsPaid, w.TicketPriceCents, w.TicketCurrency).
		Scan(&w.ID, &w.CreatedAt, &w.UpdatedAt)
}

// GetByID returns a webinar by ID.
func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*models.Webinar, error) {
	const q = `SELECT id, title, description, starts_at, ends_at, created_by, organization_id, is_paid, ticket_price_cents, ticket_currency, created_at, updated_at
		FROM webinars WHERE id = $1`
	var w models.Webinar
	err := r.pool.QueryRow(ctx, q, id).Scan(&w.ID, &w.Title, &w.Description, &w.StartsAt, &w.EndsAt, &w.CreatedBy, &w.OrganizationID, &w.IsPaid, &w.TicketPriceCents, &w.TicketCurrency, &w.CreatedAt, &w.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &w, nil
}

// AddSpeaker adds a speaker to a webinar.
func (r *Repository) AddSpeaker(ctx context.Context, webinarID, userID uuid.UUID) error {
	const q = `INSERT INTO webinar_speakers (webinar_id, user_id) VALUES ($1, $2)
		ON CONFLICT (webinar_id, user_id) DO NOTHING`
	_, err := r.pool.Exec(ctx, q, webinarID, userID)
	return err
}

// List returns all webinars, optionally filtered by created_by or organization_id.
func (r *Repository) List(ctx context.Context, createdBy *uuid.UUID, organizationID *uuid.UUID) ([]models.Webinar, error) {
	base := `SELECT id, title, description, starts_at, ends_at, created_by, organization_id, is_paid, ticket_price_cents, ticket_currency, created_at, updated_at FROM webinars`
	var args []interface{}
	var cond string
	if createdBy != nil {
		cond = " WHERE created_by = $1"
		args = append(args, *createdBy)
	}
	if organizationID != nil {
		if cond == "" {
			cond = " WHERE organization_id = $1"
		} else {
			cond += " AND organization_id = $2"
		}
		args = append(args, *organizationID)
	}
	rows, err := r.pool.Query(ctx, base+cond+" ORDER BY starts_at DESC", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.Webinar
	for rows.Next() {
		var w models.Webinar
		if err := rows.Scan(&w.ID, &w.Title, &w.Description, &w.StartsAt, &w.EndsAt, &w.CreatedBy, &w.OrganizationID, &w.IsPaid, &w.TicketPriceCents, &w.TicketCurrency, &w.CreatedAt, &w.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, w)
	}
	return list, rows.Err()
}

// Update updates webinar fields (title, description, starts_at, ends_at).
func (r *Repository) Update(ctx context.Context, id uuid.UUID, title, description string, startsAt, endsAt *time.Time) error {
	const q = `UPDATE webinars SET title = $1, description = $2, starts_at = COALESCE($3, starts_at), ends_at = COALESCE($4, ends_at), updated_at = NOW() WHERE id = $5`
	_, err := r.pool.Exec(ctx, q, title, description, startsAt, endsAt, id)
	return err
}

// Delete removes a webinar by ID.
func (r *Repository) Delete(ctx context.Context, id uuid.UUID) error {
	const q = `DELETE FROM webinars WHERE id = $1`
	_, err := r.pool.Exec(ctx, q, id)
	return err
}

// IsAdminOrSpeaker returns true if the user created the webinar or is a speaker.
func (r *Repository) IsAdminOrSpeaker(ctx context.Context, webinarID, userID uuid.UUID) (bool, error) {
	w, err := r.GetByID(ctx, webinarID)
	if err != nil {
		return false, err
	}
	if w.CreatedBy == userID {
		return true, nil
	}
	const q = `SELECT 1 FROM webinar_speakers WHERE webinar_id = $1 AND user_id = $2`
	var exists int
	err = r.pool.QueryRow(ctx, q, webinarID, userID).Scan(&exists)
	return err == nil, nil
}
