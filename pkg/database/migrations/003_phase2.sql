-- Phase 2: Organizations, Registrations, Payments, Coupons, Analytics, Email

-- Organizations (SaaS foundation)
CREATE TABLE IF NOT EXISTS organizations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(128) UNIQUE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_organizations_slug ON organizations(slug);

-- Organization users (roles: owner, event_manager, moderator)
CREATE TABLE IF NOT EXISTS organization_users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role VARCHAR(32) NOT NULL DEFAULT 'moderator' CHECK (role IN ('owner', 'event_manager', 'moderator')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(organization_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_organization_users_org ON organization_users(organization_id);
CREATE INDEX IF NOT EXISTS idx_organization_users_user ON organization_users(user_id);

-- Webinars: add organization_id and paid webinar fields (nullable for backward compat)
ALTER TABLE webinars ADD COLUMN IF NOT EXISTS organization_id UUID REFERENCES organizations(id) ON DELETE SET NULL;
ALTER TABLE webinars ADD COLUMN IF NOT EXISTS is_paid BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE webinars ADD COLUMN IF NOT EXISTS ticket_price_cents INT NOT NULL DEFAULT 0;
ALTER TABLE webinars ADD COLUMN IF NOT EXISTS ticket_currency VARCHAR(3) NOT NULL DEFAULT 'USD';

CREATE INDEX IF NOT EXISTS idx_webinars_organization ON webinars(organization_id) WHERE organization_id IS NOT NULL;

-- Registrations (attendee must register before joining)
CREATE TABLE IF NOT EXISTS registrations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    webinar_id UUID NOT NULL REFERENCES webinars(id) ON DELETE CASCADE,
    email VARCHAR(255) NOT NULL,
    full_name VARCHAR(255) NOT NULL,
    attended_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(webinar_id, email)
);

CREATE INDEX IF NOT EXISTS idx_registrations_webinar ON registrations(webinar_id);
CREATE INDEX IF NOT EXISTS idx_registrations_email ON registrations(webinar_id, email);

-- Registration tokens (unique join link per attendee)
CREATE TABLE IF NOT EXISTS registration_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    registration_id UUID NOT NULL REFERENCES registrations(id) ON DELETE CASCADE,
    token VARCHAR(64) UNIQUE NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_registration_tokens_token ON registration_tokens(token);
CREATE INDEX IF NOT EXISTS idx_registration_tokens_registration ON registration_tokens(registration_id);
CREATE INDEX IF NOT EXISTS idx_registration_tokens_expires ON registration_tokens(expires_at) WHERE used_at IS NULL;

-- Coupons (for paid webinars)
CREATE TABLE IF NOT EXISTS coupons (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    webinar_id UUID NOT NULL REFERENCES webinars(id) ON DELETE CASCADE,
    code VARCHAR(64) NOT NULL,
    discount_type VARCHAR(16) NOT NULL CHECK (discount_type IN ('percent', 'fixed')),
    discount_value INT NOT NULL,
    max_uses INT NOT NULL DEFAULT 0,
    used_count INT NOT NULL DEFAULT 0,
    valid_from TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    valid_until TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(webinar_id, code)
);

CREATE INDEX IF NOT EXISTS idx_coupons_webinar ON coupons(webinar_id);
CREATE INDEX IF NOT EXISTS idx_coupons_code ON coupons(webinar_id, code);

-- Payments (Stripe + Razorpay)
CREATE TABLE IF NOT EXISTS payments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    webinar_id UUID NOT NULL REFERENCES webinars(id) ON DELETE CASCADE,
    registration_id UUID REFERENCES registrations(id) ON DELETE SET NULL,
    provider VARCHAR(32) NOT NULL CHECK (provider IN ('stripe', 'razorpay')),
    provider_payment_id VARCHAR(255),
    provider_order_id VARCHAR(255),
    amount_cents INT NOT NULL,
    currency VARCHAR(3) NOT NULL DEFAULT 'USD',
    status VARCHAR(32) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'completed', 'failed', 'refunded', 'partially_refunded')),
    metadata JSONB,
    refunded_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_payments_webinar ON payments(webinar_id);
CREATE INDEX IF NOT EXISTS idx_payments_registration ON payments(registration_id) WHERE registration_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_payments_provider_id ON payments(provider, provider_payment_id);
CREATE INDEX IF NOT EXISTS idx_payments_status ON payments(status);

-- User session logs (for analytics: join/leave, watch duration)
CREATE TABLE IF NOT EXISTS user_session_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    webinar_id UUID NOT NULL REFERENCES webinars(id) ON DELETE CASCADE,
    registration_id UUID REFERENCES registrations(id) ON DELETE SET NULL,
    user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    joined_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    left_at TIMESTAMPTZ,
    watch_seconds BIGINT NOT NULL DEFAULT 0,
    metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_user_session_logs_webinar ON user_session_logs(webinar_id);
CREATE INDEX IF NOT EXISTS idx_user_session_logs_joined ON user_session_logs(webinar_id, joined_at);

-- Engagement metrics (aggregated per webinar/session)
CREATE TABLE IF NOT EXISTS engagement_metrics (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    webinar_id UUID NOT NULL REFERENCES webinars(id) ON DELETE CASCADE,
    stream_session_id UUID REFERENCES stream_sessions(id) ON DELETE SET NULL,
    total_registrations INT NOT NULL DEFAULT 0,
    total_attended INT NOT NULL DEFAULT 0,
    total_no_show INT NOT NULL DEFAULT 0,
    peak_live_viewers INT NOT NULL DEFAULT 0,
    avg_watch_seconds BIGINT NOT NULL DEFAULT 0,
    poll_participation_count INT NOT NULL DEFAULT 0,
    poll_participation_percent NUMERIC(5,2) NOT NULL DEFAULT 0,
    questions_count INT NOT NULL DEFAULT 0,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_engagement_metrics_webinar ON engagement_metrics(webinar_id);

-- Email logs (automation: confirmation, reminders, thank-you, replay)
CREATE TABLE IF NOT EXISTS email_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    webinar_id UUID REFERENCES webinars(id) ON DELETE SET NULL,
    registration_id UUID REFERENCES registrations(id) ON DELETE SET NULL,
    email_type VARCHAR(32) NOT NULL,
    recipient_email VARCHAR(255) NOT NULL,
    subject VARCHAR(512),
    status VARCHAR(32) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'sent', 'failed')),
    sent_at TIMESTAMPTZ,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_email_logs_webinar ON email_logs(webinar_id);
CREATE INDEX IF NOT EXISTS idx_email_logs_registration ON email_logs(registration_id);
CREATE INDEX IF NOT EXISTS idx_email_logs_type ON email_logs(email_type);
CREATE INDEX IF NOT EXISTS idx_email_logs_created ON email_logs(created_at);
