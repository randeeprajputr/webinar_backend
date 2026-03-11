package waitlist

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository handles waitlist persistence.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a waitlist repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Create inserts a waitlist entry (unique per webinar+email).
func (r *Repository) Create(ctx context.Context, e *Entry) error {
	const q = `INSERT INTO waitlist (id, webinar_id, email, full_name, extra_data, status)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5)
		ON CONFLICT (webinar_id, email) DO UPDATE SET full_name = EXCLUDED.full_name, extra_data = EXCLUDED.extra_data, status = 'waiting', promoted_at = NULL
		RETURNING id, created_at`
	return r.pool.QueryRow(ctx, q, e.WebinarID, e.Email, e.FullName, e.ExtraData, StatusWaiting).
		Scan(&e.ID, &e.CreatedAt)
}

// GetByWebinarAndEmail returns the waitlist entry for webinar+email.
func (r *Repository) GetByWebinarAndEmail(ctx context.Context, webinarID uuid.UUID, email string) (*Entry, error) {
	const q = `SELECT id, webinar_id, email, full_name, extra_data, status, promoted_at, created_at FROM waitlist WHERE webinar_id = $1 AND email = $2`
	var e Entry
	err := r.pool.QueryRow(ctx, q, webinarID, email).Scan(&e.ID, &e.WebinarID, &e.Email, &e.FullName, &e.ExtraData, &e.Status, &e.PromotedAt, &e.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &e, nil
}
