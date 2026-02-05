package polls

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aura-webinar/backend/internal/models"
)

// Repository handles poll persistence.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a polls repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Create inserts a new poll.
func (r *Repository) Create(ctx context.Context, p *models.Poll) error {
	const query = `INSERT INTO polls (id, webinar_id, question, option_a, option_b, option_c, option_d, launched, closed)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, FALSE, FALSE)
		RETURNING id, created_at`
	return r.pool.QueryRow(ctx, query, p.WebinarID, p.Question, p.OptionA, p.OptionB, p.OptionC, p.OptionD).
		Scan(&p.ID, &p.CreatedAt)
}

// GetByID returns a poll by ID.
func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*models.Poll, error) {
	const query = `SELECT id, webinar_id, question, option_a, option_b, option_c, option_d, launched, closed, created_at
		FROM polls WHERE id = $1`
	var p models.Poll
	err := r.pool.QueryRow(ctx, query, id).
		Scan(&p.ID, &p.WebinarID, &p.Question, &p.OptionA, &p.OptionB, &p.OptionC, &p.OptionD, &p.Launched, &p.Closed, &p.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// Launch sets poll launched to true.
func (r *Repository) Launch(ctx context.Context, id uuid.UUID) error {
	const query = `UPDATE polls SET launched = TRUE WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, id)
	return err
}

// Close sets poll closed to true.
func (r *Repository) Close(ctx context.Context, id uuid.UUID) error {
	const query = `UPDATE polls SET closed = TRUE WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, id)
	return err
}

// Answer records a user's poll answer (A/B/C/D). One per user per poll.
func (r *Repository) Answer(ctx context.Context, pollID, userID uuid.UUID, option string) error {
	const query = `INSERT INTO poll_answers (poll_id, user_id, option) VALUES ($1, $2, $3)
		ON CONFLICT (poll_id, user_id) DO UPDATE SET option = EXCLUDED.option, answered_at = NOW()`
	_, err := r.pool.Exec(ctx, query, pollID, userID, option)
	return err
}
