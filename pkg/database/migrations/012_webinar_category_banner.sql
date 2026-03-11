-- Webinar category and banner image
ALTER TABLE webinars ADD COLUMN IF NOT EXISTS category VARCHAR(100);
ALTER TABLE webinars ADD COLUMN IF NOT EXISTS banner_image_url VARCHAR(512);
