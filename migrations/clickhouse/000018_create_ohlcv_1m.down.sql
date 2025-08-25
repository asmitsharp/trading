-- Drop 1-minute OHLCV table
DROP INDEX IF EXISTS idx_ohlcv_1m_tokens;
DROP INDEX IF EXISTS idx_ohlcv_1m_timestamp;
DROP TABLE IF EXISTS ohlcv_1m;