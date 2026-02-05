package questions

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aura-webinar/backend/internal/models"
)

// Repository handles question persistence.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a questions repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Create inserts a new question.
func (r *Repository) Create(ctx context.Context, q *models.Question) error {
	const query = `INSERT INTO questions (id, webinar_id, user_id, content, approved)
		VALUES (gen_random_uuid(), $1, $2, $3, FALSE)
		RETURNING id, created_at`
	return r.pool.QueryRow(ctx, query, q.WebinarID, q.UserID, q.Content).
		Scan(&q.ID, &q.CreatedAt)
}

// GetByID returns a question by ID.
func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*models.Question, error) {
	const query = `SELECT id, webinar_id, user_id, content, approved, created_at
		FROM questions WHERE id = $1`
	var q models.Question
	err := r.pool.QueryRow(ctx, query, id).
		Scan(&q.ID, &q.WebinarID, &q.UserID, &q.Content, &q.Approved, &q.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &q, nil
}

// Approve sets question approved to true.
func (r *Repository) Approve(ctx context.Context, id uuid.UUID) error {
	const query = `UPDATE questions SET approved = TRUE WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, id)
	return err
}

// CountByWebinar returns the number of questions for a webinar.
func (r *Repository) CountByWebinar(ctx context.Context, webinarID uuid.UUID) (int, error) {
	const query = `SELECT COUNT(*) FROM questions WHERE webinar_id = $1`
	var n int
	err := r.pool.QueryRow(ctx, query, webinarID).Scan(&n)
	return n, err
}
