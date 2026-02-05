package registrations

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aura-webinar/backend/internal/models"
)

// Repository handles registration and token persistence.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a registrations repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// CreateRegistration inserts a registration (unique per webinar+email).
func (r *Repository) CreateRegistration(ctx context.Context, reg *models.Registration) error {
	const q = `INSERT INTO registrations (id, webinar_id, email, full_name)
		VALUES (gen_random_uuid(), $1, $2, $3)
		ON CONFLICT (webinar_id, email) DO UPDATE SET full_name = EXCLUDED.full_name, updated_at = NOW()
		RETURNING id, attended_at, created_at, updated_at`
	return r.pool.QueryRow(ctx, q, reg.WebinarID, reg.Email, reg.FullName).
		Scan(&reg.ID, &reg.AttendedAt, &reg.CreatedAt, &reg.UpdatedAt)
}

// GetRegistrationByID returns a registration by ID.
func (r *Repository) GetRegistrationByID(ctx context.Context, id uuid.UUID) (*models.Registration, error) {
	const q = `SELECT id, webinar_id, email, full_name, attended_at, created_at, updated_at FROM registrations WHERE id = $1`
	var reg models.Registration
	err := r.pool.QueryRow(ctx, q, id).Scan(&reg.ID, &reg.WebinarID, &reg.Email, &reg.FullName, &reg.AttendedAt, &reg.CreatedAt, &reg.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &reg, nil
}

// GetRegistrationByWebinarAndEmail returns the registration for webinar+email.
func (r *Repository) GetRegistrationByWebinarAndEmail(ctx context.Context, webinarID uuid.UUID, email string) (*models.Registration, error) {
	const q = `SELECT id, webinar_id, email, full_name, attended_at, created_at, updated_at FROM registrations WHERE webinar_id = $1 AND email = $2`
	var reg models.Registration
	err := r.pool.QueryRow(ctx, q, webinarID, email).Scan(&reg.ID, &reg.WebinarID, &reg.Email, &reg.FullName, &reg.AttendedAt, &reg.CreatedAt, &reg.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &reg, nil
}

// ListByWebinar returns all registrations for a webinar.
func (r *Repository) ListByWebinar(ctx context.Context, webinarID uuid.UUID) ([]models.Registration, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, webinar_id, email, full_name, attended_at, created_at, updated_at FROM registrations WHERE webinar_id = $1 ORDER BY created_at DESC`, webinarID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []models.Registration
	for rows.Next() {
		var reg models.Registration
		if err := rows.Scan(&reg.ID, &reg.WebinarID, &reg.Email, &reg.FullName, &reg.AttendedAt, &reg.CreatedAt, &reg.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, reg)
	}
	return list, rows.Err()
}

// CountByWebinar returns total registrations and attended count for a webinar.
func (r *Repository) CountByWebinar(ctx context.Context, webinarID uuid.UUID) (total, attended int, err error) {
	const q = `SELECT COUNT(*), COUNT(attended_at) FROM registrations WHERE webinar_id = $1`
	err = r.pool.QueryRow(ctx, q, webinarID).Scan(&total, &attended)
	return total, attended, err
}

// MarkAttended sets attended_at for a registration.
func (r *Repository) MarkAttended(ctx context.Context, registrationID uuid.UUID) error {
	const q = `UPDATE registrations SET attended_at = NOW(), updated_at = NOW() WHERE id = $1 AND attended_at IS NULL`
	_, err := r.pool.Exec(ctx, q, registrationID)
	return err
}

// CreateToken inserts a registration token.
func (r *Repository) CreateToken(ctx context.Context, t *models.RegistrationToken) error {
	const q = `INSERT INTO registration_tokens (id, registration_id, token, expires_at)
		VALUES (gen_random_uuid(), $1, $2, $3)
		RETURNING id, used_at, created_at`
	return r.pool.QueryRow(ctx, q, t.RegistrationID, t.Token, t.ExpiresAt).
		Scan(&t.ID, &t.UsedAt, &t.CreatedAt)
}

// GetTokenByToken returns a token by its string (for validation).
func (r *Repository) GetTokenByToken(ctx context.Context, tokenStr string) (*models.RegistrationToken, error) {
	const q = `SELECT id, registration_id, token, expires_at, used_at, created_at FROM registration_tokens WHERE token = $1`
	var t models.RegistrationToken
	err := r.pool.QueryRow(ctx, q, tokenStr).Scan(&t.ID, &t.RegistrationID, &t.Token, &t.ExpiresAt, &t.UsedAt, &t.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// MarkTokenUsed sets used_at for a token.
func (r *Repository) MarkTokenUsed(ctx context.Context, tokenID uuid.UUID) error {
	const q = `UPDATE registration_tokens SET used_at = NOW() WHERE id = $1 AND used_at IS NULL`
	_, err := r.pool.Exec(ctx, q, tokenID)
	return err
}

// GetByRegistrationID returns a registration by ID (alias for handlers).
func (r *Repository) GetByRegistrationID(ctx context.Context, id uuid.UUID) (*models.Registration, error) {
	return r.GetRegistrationByID(ctx, id)
}
