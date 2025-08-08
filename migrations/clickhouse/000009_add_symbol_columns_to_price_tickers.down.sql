-- Remove symbol columns from price_tickers table
ALTER TABLE price_tickers
    DROP COLUMN IF EXISTS symbol;

ALTER TABLE price_tickers
    DROP COLUMN IF EXISTS base_symbol;

ALTER TABLE price_tickers
    DROP COLUMN IF EXISTS quote_symbol;