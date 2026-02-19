-- User profile fields: Admin (department, company_name, contact_no), Speaker (designation, institution)
ALTER TABLE users ADD COLUMN IF NOT EXISTS department VARCHAR(255);
ALTER TABLE users ADD COLUMN IF NOT EXISTS company_name VARCHAR(255);
ALTER TABLE users ADD COLUMN IF NOT EXISTS contact_no VARCHAR(64);
ALTER TABLE users ADD COLUMN IF NOT EXISTS designation VARCHAR(255);
ALTER TABLE users ADD COLUMN IF NOT EXISTS institution VARCHAR(255);
