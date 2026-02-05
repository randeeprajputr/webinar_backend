package analytics

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aura-webinar/backend/internal/questions"
	"github.com/aura-webinar/backend/internal/registrations"
	"github.com/aura-webinar/backend/internal/streams"
	"github.com/aura-webinar/backend/internal/webinars"
	"github.com/aura-webinar/backend/pkg/response"
)

// Handler handles GET /webinars/:id/analytics.
type Handler struct {
	pool             *pgxpool.Pool
	registrationRepo *registrations.Repository
	questionRepo     *questions.Repository
	streamRepo       *streams.Repository
	webinarRepo      *webinars.Repository
}

// NewHandler creates an analytics handler.
func NewHandler(
	pool *pgxpool.Pool,
	registrationRepo *registrations.Repository,
	questionRepo *questions.Repository,
	streamRepo *streams.Repository,
	webinarRepo *webinars.Repository,
) *Handler {
	return &Handler{
		pool:             pool,
		registrationRepo: registrationRepo,
		questionRepo:     questionRepo,
		streamRepo:       streamRepo,
		webinarRepo:      webinarRepo,
	}
}

// SummaryResponse is the JSON shape for analytics (matches frontend AnalyticsSummary).
type SummaryResponse struct {
	TotalRegistrations      int     `json:"total_registrations"`
	TotalAttended           int     `json:"total_attended"`
	TotalNoShow             int     `json:"total_no_show"`
	PeakLiveViewers         int     `json:"peak_live_viewers"`
	AvgWatchSeconds         int64   `json:"avg_watch_seconds"`
	PollParticipationPercent float64 `json:"poll_participation_percent"`
	QuestionsCount          int     `json:"questions_count"`
	RevenueCents            *int    `json:"revenue_cents,omitempty"`
	ConversionRate          *float64 `json:"conversion_rate,omitempty"`
}

// GetByWebinar handles GET /webinars/:id/analytics. Admin or webinar org access required (enforced by route middleware).
func (h *Handler) GetByWebinar(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		response.BadRequest(c, "invalid webinar id")
		return
	}

	ctx := c.Request.Context()

	// Ensure webinar exists
	_, err = h.webinarRepo.GetByID(ctx, id)
	if err != nil {
		response.NotFound(c, "webinar not found")
		return
	}

	total, attended, err := h.registrationRepo.CountByWebinar(ctx, id)
	if err != nil {
		response.Internal(c, "failed to load registration counts")
		return
	}
	noShow := total - attended
	if noShow < 0 {
		noShow = 0
	}

	agg, err := h.streamRepo.GetAggregatesByWebinar(ctx, id)
	if err != nil {
		response.Internal(c, "failed to load stream aggregates")
		return
	}

	questionsCount, err := h.questionRepo.CountByWebinar(ctx, id)
	if err != nil {
		response.Internal(c, "failed to load questions count")
		return
	}

	var avgWatchSeconds int64
	if agg.TotalViewers > 0 {
		avgWatchSeconds = agg.TotalWatchTime / int64(agg.TotalViewers)
	}

	// Poll participation: distinct users who answered any poll for this webinar
	var pollParticipants int
	const pollQ = `SELECT COUNT(DISTINCT pa.user_id) FROM poll_answers pa
		INNER JOIN polls p ON p.id = pa.poll_id WHERE p.webinar_id = $1`
	_ = h.pool.QueryRow(ctx, pollQ, id).Scan(&pollParticipants)
	pollPercent := 0.0
	if attended > 0 {
		pollPercent = float64(pollParticipants) / float64(attended) * 100
	}

	// Revenue: sum of completed payments for this webinar
	var revenueCents int
	const revQ = `SELECT COALESCE(SUM(amount_cents), 0) FROM payments WHERE webinar_id = $1 AND status = 'completed'`
	_ = h.pool.QueryRow(ctx, revQ, id).Scan(&revenueCents)

	out := SummaryResponse{
		TotalRegistrations:       total,
		TotalAttended:            attended,
		TotalNoShow:              noShow,
		PeakLiveViewers:          agg.PeakViewers,
		AvgWatchSeconds:          avgWatchSeconds,
		PollParticipationPercent: pollPercent,
		QuestionsCount:           questionsCount,
	}
	if total > 0 {
		conv := float64(attended) / float64(total)
		out.ConversionRate = &conv
	}
	if revenueCents > 0 {
		out.RevenueCents = &revenueCents
	}

	response.OK(c, out)
}
