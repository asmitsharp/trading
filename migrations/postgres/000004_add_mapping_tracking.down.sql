-- Remove indexes
DROP INDEX IF EXISTS idx_token_symbols_verification;
DROP INDEX IF EXISTS idx_trading_pairs_verification;

-- Remove mapping tracking columns from trading_pairs
ALTER TABLE trading_pairs
DROP COLUMN IF EXISTS mapping_method,
DROP COLUMN IF EXISTS confidence_score,
DROP COLUMN IF EXISTS needs_verification,
DROP COLUMN IF EXISTS verified_by,
DROP COLUMN IF EXISTS verified_at;

-- Remove mapping tracking columns from token_exchange_symbols
ALTER TABLE token_exchange_symbols
DROP COLUMN IF EXISTS mapping_method,
DROP COLUMN IF EXISTS confidence_score,
DROP COLUMN IF EXISTS needs_verification,
DROP COLUMN IF EXISTS verified_by,
DROP COLUMN IF EXISTS verified_at,
DROP COLUMN IF EXISTS last_price_check;