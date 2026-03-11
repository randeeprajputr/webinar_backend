-- Speaker invitations (admin invites external speakers by email)
CREATE TABLE IF NOT EXISTS speaker_invitations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    webinar_id UUID NOT NULL REFERENCES webinars(id) ON DELETE CASCADE,
    email VARCHAR(255) NOT NULL,
    token VARCHAR(64) UNIQUE NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    accepted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(webinar_id, email)
);
CREATE INDEX IF NOT EXISTS idx_speaker_invitations_webinar ON speaker_invitations(webinar_id);
CREATE INDEX IF NOT EXISTS idx_speaker_invitations_token ON speaker_invitations(token);
CREATE INDEX IF NOT EXISTS idx_speaker_invitations_email ON speaker_invitations(webinar_id, email);
