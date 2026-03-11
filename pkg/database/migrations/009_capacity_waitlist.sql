-- Webinar capacity and waitlist
ALTER TABLE webinars ADD COLUMN IF NOT EXISTS max_audience INT;
-- NULL = unlimited; positive = max registrations

CREATE TABLE IF NOT EXISTS waitlist (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    webinar_id UUID NOT NULL REFERENCES webinars(id) ON DELETE CASCADE,
    email VARCHAR(255) NOT NULL,
    full_name VARCHAR(255) NOT NULL,
    extra_data JSONB,
    status VARCHAR(32) NOT NULL DEFAULT 'waiting' CHECK (status IN ('waiting', 'promoted', 'cancelled')),
    promoted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(webinar_id, email)
);
CREATE INDEX IF NOT EXISTS idx_waitlist_webinar ON waitlist(webinar_id);
CREATE INDEX IF NOT EXISTS idx_waitlist_status ON waitlist(webinar_id, status);
