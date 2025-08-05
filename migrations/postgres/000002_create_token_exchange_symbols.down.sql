-- Drop triggers
DROP TRIGGER IF EXISTS update_trading_pairs_updated_at ON trading_pairs;
DROP TRIGGER IF EXISTS update_token_exchange_symbols_updated_at ON token_exchange_symbols;

-- Drop indexes for trading pairs
DROP INDEX IF EXISTS idx_trading_pairs_active;
DROP INDEX IF EXISTS idx_trading_pairs_symbol;
DROP INDEX IF EXISTS idx_trading_pairs_exchange;
DROP INDEX IF EXISTS idx_trading_pairs_quote_token;
DROP INDEX IF EXISTS idx_trading_pairs_base_token;

-- Drop trading pairs table
DROP TABLE IF EXISTS trading_pairs;

-- Drop indexes for token exchange symbols
DROP INDEX IF EXISTS idx_token_exchange_symbols_active;
DROP INDEX IF EXISTS idx_token_exchange_symbols_normalized;
DROP INDEX IF EXISTS idx_token_exchange_symbols_exchange_symbol;
DROP INDEX IF EXISTS idx_token_exchange_symbols_exchange_id;
DROP INDEX IF EXISTS idx_token_exchange_symbols_token_id;

-- Drop token exchange symbols table
DROP TABLE IF EXISTS token_exchange_symbols;