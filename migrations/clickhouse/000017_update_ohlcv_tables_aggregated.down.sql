-- Rollback OHLCV table updates

-- Drop the new aggregated tables
DROP TABLE IF EXISTS ohlcv_1m;
DROP TABLE IF EXISTS ohlcv_5m;
DROP TABLE IF EXISTS ohlcv_15m;
DROP TABLE IF EXISTS ohlcv_1h;
DROP TABLE IF EXISTS ohlcv_4h;
DROP TABLE IF EXISTS ohlcv_1d;

-- Restore from backups if they exist
CREATE TABLE IF NOT EXISTS ohlcv_1m AS SELECT * FROM ohlcv_1m_backup;
CREATE TABLE IF NOT EXISTS ohlcv_5m AS SELECT * FROM ohlcv_5m_backup;
CREATE TABLE IF NOT EXISTS ohlcv_15m AS SELECT * FROM ohlcv_15m_backup;
CREATE TABLE IF NOT EXISTS ohlcv_1h AS SELECT * FROM ohlcv_1h_backup;
CREATE TABLE IF NOT EXISTS ohlcv_4h AS SELECT * FROM ohlcv_4h_backup;
CREATE TABLE IF NOT EXISTS ohlcv_1d AS SELECT * FROM ohlcv_1d_old;

-- Clean up backup tables
DROP TABLE IF EXISTS ohlcv_1m_backup;
DROP TABLE IF EXISTS ohlcv_5m_backup;
DROP TABLE IF EXISTS ohlcv_15m_backup;
DROP TABLE IF EXISTS ohlcv_1h_backup;
DROP TABLE IF EXISTS ohlcv_4h_backup;
DROP TABLE IF EXISTS ohlcv_1d_old;