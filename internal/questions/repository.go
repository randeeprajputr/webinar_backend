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
	const query = `INSERT INTO questions (id, webinar_id, user_id, content, approved, answered, votes)
		VALUES (gen_random_uuid(), $1, $2, $3, FALSE, FALSE, 0)
		RETURNING id, created_at`
	return r.pool.QueryRow(ctx, query, q.WebinarID, q.UserID, q.Content).
		Scan(&q.ID, &q.CreatedAt)
}

// GetByID returns a question by ID.
func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*models.Question, error) {
	const query = `SELECT id, webinar_id, user_id, content, approved, COALESCE(answered, FALSE), COALESCE(votes, 0), created_at
		FROM questions WHERE id = $1`
	var q models.Question
	err := r.pool.QueryRow(ctx, query, id).
		Scan(&q.ID, &q.WebinarID, &q.UserID, &q.Content, &q.Approved, &q.Answered, &q.Votes, &q.CreatedAt)
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

// MarkAnswered sets question answered to true.
func (r *Repository) MarkAnswered(ctx context.Context, id uuid.UUID) error {
	const query = `UPDATE questions SET answered = TRUE WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, id)
	return err
}

// Upvote adds a vote from user for question (one per user). Returns new vote count.
func (r *Repository) Upvote(ctx context.Context, questionID, userID uuid.UUID) (int, error) {
	_, err := r.pool.Exec(ctx, `INSERT INTO question_votes (question_id, user_id) VALUES ($1, $2) ON CONFLICT (question_id, user_id) DO NOTHING`, questionID, userID)
	if err != nil {
		return 0, err
	}
	var votes int
	err = r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM question_votes WHERE question_id = $1`, questionID).Scan(&votes)
	if err != nil {
		return 0, err
	}
	_, err = r.pool.Exec(ctx, `UPDATE questions SET votes = $1 WHERE id = $2`, votes, questionID)
	return votes, err
}

// ListByWebinar returns all questions for a webinar (with votes, answered), ordered by created_at.
func (r *Repository) ListByWebinar(ctx context.Context, webinarID uuid.UUID) ([]*models.Question, error) {
	const query = `SELECT id, webinar_id, user_id, content, approved, COALESCE(answered, FALSE), COALESCE(votes, 0), created_at
		FROM questions WHERE webinar_id = $1 ORDER BY created_at ASC`
	rows, err := r.pool.Query(ctx, query, webinarID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []*models.Question
	for rows.Next() {
		var q models.Question
		if err := rows.Scan(&q.ID, &q.WebinarID, &q.UserID, &q.Content, &q.Approved, &q.Answered, &q.Votes, &q.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, &q)
	}
	return list, rows.Err()
}

// CountByWebinar returns the number of questions for a webinar.
func (r *Repository) CountByWebinar(ctx context.Context, webinarID uuid.UUID) (int, error) {
	const query = `SELECT COUNT(*) FROM questions WHERE webinar_id = $1`
	var n int
	err := r.pool.QueryRow(ctx, query, webinarID).Scan(&n)
	return n, err
}
