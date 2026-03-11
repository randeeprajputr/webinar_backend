package feedback

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository handles webinar feedback persistence.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a feedback repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Create inserts feedback (one per registration per webinar).
func (r *Repository) Create(ctx context.Context, e *Entry) error {
	const q = `INSERT INTO webinar_feedback (id, webinar_id, registration_id, rating, speaker_effectiveness, content_usefulness, suggestions)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6)
		ON CONFLICT (webinar_id, registration_id) DO UPDATE SET rating = EXCLUDED.rating, speaker_effectiveness = EXCLUDED.speaker_effectiveness, content_usefulness = EXCLUDED.content_usefulness, suggestions = EXCLUDED.suggestions
		RETURNING id, created_at`
	return r.pool.QueryRow(ctx, q, e.WebinarID, e.RegistrationID, e.Rating, e.SpeakerEffectiveness, e.ContentUsefulness, e.Suggestions).
		Scan(&e.ID, &e.CreatedAt)
}

// ListByWebinar returns all feedback for a webinar.
func (r *Repository) ListByWebinar(ctx context.Context, webinarID uuid.UUID) ([]Entry, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, webinar_id, registration_id, rating, speaker_effectiveness, content_usefulness, suggestions, created_at
		 FROM webinar_feedback WHERE webinar_id = $1 ORDER BY created_at DESC`,
		webinarID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Entry
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.ID, &e.WebinarID, &e.RegistrationID, &e.Rating, &e.SpeakerEffectiveness, &e.ContentUsefulness, &e.Suggestions, &e.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, e)
	}
	return list, rows.Err()
}
