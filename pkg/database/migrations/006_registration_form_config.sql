-- Audience registration form: admin-defined fields (JSON array of { id, label, type, required })
ALTER TABLE webinars ADD COLUMN IF NOT EXISTS audience_form_config JSONB;

-- Store dynamic form responses at registration (key-value from custom fields)
ALTER TABLE registrations ADD COLUMN IF NOT EXISTS extra_data JSONB;
