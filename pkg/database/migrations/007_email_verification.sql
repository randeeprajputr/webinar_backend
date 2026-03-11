-- Email verification for user registration
ALTER TABLE users ADD COLUMN IF NOT EXISTS email_verified BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE users ADD COLUMN IF NOT EXISTS email_verification_token VARCHAR(64);
ALTER TABLE users ADD COLUMN IF NOT EXISTS email_verification_expires_at TIMESTAMPTZ;
CREATE INDEX IF NOT EXISTS idx_users_verification_token ON users(email_verification_token) WHERE email_verification_token IS NOT NULL;
