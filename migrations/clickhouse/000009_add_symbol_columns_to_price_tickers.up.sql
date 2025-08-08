-- Add symbol columns to price_tickers table for better debugging and tracking
ALTER TABLE price_tickers
    ADD COLUMN IF NOT EXISTS symbol String AFTER exchange_id;

ALTER TABLE price_tickers
    ADD COLUMN IF NOT EXISTS base_symbol String AFTER symbol;

ALTER TABLE price_tickers
    ADD COLUMN IF NOT EXISTS quote_symbol String AFTER base_symbol;