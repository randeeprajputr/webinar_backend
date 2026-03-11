package speakerinvites

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Invitation represents a speaker invitation.
type Invitation struct {
	ID         uuid.UUID  `json:"id"`
	WebinarID  uuid.UUID  `json:"webinar_id"`
	Email      string     `json:"email"`
	Token      string     `json:"-"`
	ExpiresAt  time.Time  `json:"expires_at"`
	AcceptedAt *time.Time `json:"accepted_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// Repository handles speaker invitation persistence.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a speaker invites repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Create creates an invitation (or returns existing if same email for webinar).
func (r *Repository) Create(ctx context.Context, webinarID uuid.UUID, email string) (*Invitation, error) {
	token, err := generateToken()
	if err != nil {
		return nil, err
	}
	expiresAt := time.Now().Add(7 * 24 * time.Hour)
	const q = `INSERT INTO speaker_invitations (webinar_id, email, token, expires_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (webinar_id, email) DO UPDATE SET token = EXCLUDED.token, expires_at = EXCLUDED.expires_at, accepted_at = NULL
		RETURNING id, webinar_id, email, token, expires_at, accepted_at, created_at`
	var inv Invitation
	err = r.pool.QueryRow(ctx, q, webinarID, email, token, expiresAt).
		Scan(&inv.ID, &inv.WebinarID, &inv.Email, &inv.Token, &inv.ExpiresAt, &inv.AcceptedAt, &inv.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

// GetByToken returns an invitation by token if valid.
func (r *Repository) GetByToken(ctx context.Context, token string) (*Invitation, error) {
	const q = `SELECT id, webinar_id, email, token, expires_at, accepted_at, created_at
		FROM speaker_invitations WHERE token = $1 AND accepted_at IS NULL AND expires_at > NOW()`
	var inv Invitation
	err := r.pool.QueryRow(ctx, q, token).Scan(&inv.ID, &inv.WebinarID, &inv.Email, &inv.Token, &inv.ExpiresAt, &inv.AcceptedAt, &inv.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

// MarkAccepted marks an invitation as accepted.
func (r *Repository) MarkAccepted(ctx context.Context, id uuid.UUID) error {
	const q = `UPDATE speaker_invitations SET accepted_at = NOW() WHERE id = $1 AND accepted_at IS NULL`
	_, err := r.pool.Exec(ctx, q, id)
	return err
}

// ListByWebinar returns invitations for a webinar.
func (r *Repository) ListByWebinar(ctx context.Context, webinarID uuid.UUID) ([]Invitation, error) {
	const q = `SELECT id, webinar_id, email, token, expires_at, accepted_at, created_at
		FROM speaker_invitations WHERE webinar_id = $1 ORDER BY created_at DESC`
	rows, err := r.pool.Query(ctx, q, webinarID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Invitation
	for rows.Next() {
		var inv Invitation
		if err := rows.Scan(&inv.ID, &inv.WebinarID, &inv.Email, &inv.Token, &inv.ExpiresAt, &inv.AcceptedAt, &inv.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, inv)
	}
	return list, rows.Err()
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b)[:43], nil
}
