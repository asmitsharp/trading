-- Drop triggers
DROP TRIGGER IF EXISTS update_tokens_updated_at ON tokens;

-- Drop function
DROP FUNCTION IF EXISTS update_updated_at_column();

-- Drop indexes
DROP INDEX IF EXISTS idx_tokens_categories;
DROP INDEX IF EXISTS idx_tokens_last_update;
DROP INDEX IF EXISTS idx_tokens_metadata;
DROP INDEX IF EXISTS idx_tokens_rank;
DROP INDEX IF EXISTS idx_tokens_symbol_active;

-- Drop table
DROP TABLE IF EXISTS tokens;