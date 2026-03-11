-- Webinar feedback from attendees
CREATE TABLE IF NOT EXISTS webinar_feedback (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    webinar_id UUID NOT NULL REFERENCES webinars(id) ON DELETE CASCADE,
    registration_id UUID NOT NULL REFERENCES registrations(id) ON DELETE CASCADE,
    rating INT NOT NULL CHECK (rating >= 1 AND rating <= 5),
    speaker_effectiveness INT CHECK (speaker_effectiveness IS NULL OR (speaker_effectiveness >= 1 AND speaker_effectiveness <= 5)),
    content_usefulness INT CHECK (content_usefulness IS NULL OR (content_usefulness >= 1 AND content_usefulness <= 5)),
    suggestions TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(webinar_id, registration_id)
);
CREATE INDEX IF NOT EXISTS idx_webinar_feedback_webinar ON webinar_feedback(webinar_id);
