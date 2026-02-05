package ads

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aura-webinar/backend/internal/models"
)

// Repository handles ad persistence.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates an ads repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Create inserts a new ad.
func (r *Repository) Create(ctx context.Context, a *models.Ad) error {
	const query = `INSERT INTO ads (id, webinar_id, title, content, image_url, link_url, active)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, FALSE)
		RETURNING id, created_at`
	return r.pool.QueryRow(ctx, query, a.WebinarID, a.Title, a.Content, a.ImageURL, a.LinkURL).
		Scan(&a.ID, &a.CreatedAt)
}

// GetByID returns an ad by ID.
func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*models.Ad, error) {
	const query = `SELECT id, webinar_id, title, content, image_url, link_url, active, created_at
		FROM ads WHERE id = $1`
	var a models.Ad
	err := r.pool.QueryRow(ctx, query, id).
		Scan(&a.ID, &a.WebinarID, &a.Title, &a.Content, &a.ImageURL, &a.LinkURL, &a.Active, &a.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// Activate sets ad active to true. Optionally deactivate others for rotation.
func (r *Repository) Activate(ctx context.Context, id uuid.UUID) error {
	const query = `UPDATE ads SET active = TRUE WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, id)
	return err
}

// ListActiveByWebinar returns active ads for a webinar (for rotation).
func (r *Repository) ListActiveByWebinar(ctx context.Context, webinarID uuid.UUID) ([]models.Ad, error) {
	const query = `SELECT id, webinar_id, title, content, image_url, link_url, active, created_at
		FROM ads WHERE webinar_id = $1 AND active = TRUE ORDER BY created_at`
	rows, err := r.pool.Query(ctx, query, webinarID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []models.Ad
	for rows.Next() {
		var a models.Ad
		if err := rows.Scan(&a.ID, &a.WebinarID, &a.Title, &a.Content, &a.ImageURL, &a.LinkURL, &a.Active, &a.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, a)
	}
	return list, rows.Err()
}
