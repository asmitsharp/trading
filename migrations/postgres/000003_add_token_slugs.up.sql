-- Add slug to tokens table for universal identification
ALTER TABLE tokens 
ADD COLUMN slug VARCHAR(100);

-- Create index for slug lookups
CREATE INDEX idx_tokens_slug ON tokens(slug);