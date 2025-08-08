-- Remove slug column and index
DROP INDEX IF EXISTS idx_tokens_slug;
ALTER TABLE tokens DROP COLUMN IF EXISTS slug;