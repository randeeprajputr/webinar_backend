-- Advanced Advertisement Module
-- advertisements: S3-backed ad creatives (image, gif, mp4)
CREATE TABLE IF NOT EXISTS advertisements (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    webinar_id UUID NOT NULL REFERENCES webinars(id) ON DELETE CASCADE,
    file_url VARCHAR(2048) NOT NULL,
    file_type VARCHAR(32) NOT NULL,
    file_size BIGINT NOT NULL DEFAULT 0,
    duration INT NOT NULL DEFAULT 0,
    s3_key VARCHAR(512),
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_advertisements_webinar ON advertisements(webinar_id);
CREATE INDEX IF NOT EXISTS idx_advertisements_webinar_active ON advertisements(webinar_id, is_active);

-- ad_playlists: rotation config per webinar
CREATE TABLE IF NOT EXISTS ad_playlists (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    webinar_id UUID NOT NULL UNIQUE REFERENCES webinars(id) ON DELETE CASCADE,
    rotation_interval INT NOT NULL DEFAULT 30,
    is_running BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ad_playlists_webinar ON ad_playlists(webinar_id);

-- ad_schedule: start/end time per ad (optional scheduling)
CREATE TABLE IF NOT EXISTS ad_schedule (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ad_id UUID NOT NULL REFERENCES advertisements(id) ON DELETE CASCADE,
    start_time TIMESTAMPTZ,
    end_time TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ad_schedule_ad ON ad_schedule(ad_id);

-- Recordings: provider recording + S3 final URL
CREATE TABLE IF NOT EXISTS recordings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    webinar_id UUID NOT NULL REFERENCES webinars(id) ON DELETE CASCADE,
    provider_recording_id VARCHAR(255),
    original_url VARCHAR(2048),
    s3_url VARCHAR(2048),
    s3_key VARCHAR(512),
    duration INT NOT NULL DEFAULT 0,
    file_size BIGINT NOT NULL DEFAULT 0,
    status VARCHAR(32) NOT NULL DEFAULT 'recording' CHECK (status IN ('recording', 'processing', 'completed', 'failed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_recordings_webinar ON recordings(webinar_id);
CREATE INDEX IF NOT EXISTS idx_recordings_status ON recordings(status);

-- Stream metadata tracking
CREATE TABLE IF NOT EXISTS stream_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    webinar_id UUID NOT NULL REFERENCES webinars(id) ON DELETE CASCADE,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ended_at TIMESTAMPTZ,
    peak_viewers INT NOT NULL DEFAULT 0,
    total_viewers INT NOT NULL DEFAULT 0,
    total_watch_time BIGINT NOT NULL DEFAULT 0,
    poll_participation_count INT NOT NULL DEFAULT 0,
    questions_count INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_stream_sessions_webinar ON stream_sessions(webinar_id);
