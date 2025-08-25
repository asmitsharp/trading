-- Update all OHLCV tables to store aggregated data (remove exchange_id, add aggregation metadata)

-- Backup existing tables
CREATE TABLE IF NOT EXISTS ohlcv_1m_backup AS SELECT * FROM ohlcv_1m;
CREATE TABLE IF NOT EXISTS ohlcv_5m_backup AS SELECT * FROM ohlcv_5m;
CREATE TABLE IF NOT EXISTS ohlcv_15m_backup AS SELECT * FROM ohlcv_15m;
CREATE TABLE IF NOT EXISTS ohlcv_1h_backup AS SELECT * FROM ohlcv_1h;
CREATE TABLE IF NOT EXISTS ohlcv_4h_backup AS SELECT * FROM ohlcv_4h;
CREATE TABLE IF NOT EXISTS ohlcv_1d_old AS SELECT * FROM ohlcv_1d;

-- Drop existing tables
DROP TABLE IF EXISTS ohlcv_1m;
DROP TABLE IF EXISTS ohlcv_5m;
DROP TABLE IF EXISTS ohlcv_15m;
DROP TABLE IF EXISTS ohlcv_1h;
DROP TABLE IF EXISTS ohlcv_4h;
DROP TABLE IF EXISTS ohlcv_1d;

-- Create new aggregated OHLCV tables (without exchange_id)

-- 1-minute candles (retain for 30 days)
CREATE TABLE IF NOT EXISTS ohlcv_1m (
    timestamp DateTime64(3),
    base_token_id UInt32,
    quote_token_id UInt32,
    
    -- Core OHLCV data (VWAP aggregated across top 5 exchanges)
    open Decimal(38, 18),
    high Decimal(38, 18),
    low Decimal(38, 18),
    close Decimal(38, 18),
    volume Decimal(38, 18),           -- Total volume across exchanges (USD equivalent)
    quote_volume Decimal(38, 18),     -- Total quote volume (USD)
    
    -- Aggregation metadata
    vwap_price Decimal(38, 18),       -- Volume-weighted average price
    exchange_count UInt8,             -- Number of contributing exchanges
    contributing_exchanges Array(String), -- List of exchanges that contributed
    data_quality_score Decimal(5, 4), -- 0.0-1.0 quality score
    
    -- Market data
    trade_count UInt32 DEFAULT 0,     -- Total trades aggregated
    
    -- System fields
    created_at DateTime64(3) DEFAULT now64(),
    version UInt64 DEFAULT toUnixTimestamp64Milli(now64())
) ENGINE = ReplacingMergeTree(version)
PARTITION BY toYYYYMM(timestamp)
ORDER BY (base_token_id, quote_token_id, timestamp)
TTL timestamp + INTERVAL 30 DAY
SETTINGS index_granularity = 8192;

-- 5-minute candles (retain for 6 months)
CREATE TABLE IF NOT EXISTS ohlcv_5m (
    timestamp DateTime64(3),
    base_token_id UInt32,
    quote_token_id UInt32,
    open Decimal(38, 18),
    high Decimal(38, 18),
    low Decimal(38, 18),
    close Decimal(38, 18),
    volume Decimal(38, 18),
    quote_volume Decimal(38, 18),
    vwap_price Decimal(38, 18),
    exchange_count UInt8,
    contributing_exchanges Array(String),
    data_quality_score Decimal(5, 4),
    trade_count UInt32 DEFAULT 0,
    created_at DateTime64(3) DEFAULT now64(),
    version UInt64 DEFAULT toUnixTimestamp64Milli(now64())
) ENGINE = ReplacingMergeTree(version)
PARTITION BY toYYYYMM(timestamp)
ORDER BY (base_token_id, quote_token_id, timestamp)
TTL timestamp + INTERVAL 6 MONTH
SETTINGS index_granularity = 8192;

-- 15-minute candles (retain for 2 years)
CREATE TABLE IF NOT EXISTS ohlcv_15m (
    timestamp DateTime64(3),
    base_token_id UInt32,
    quote_token_id UInt32,
    open Decimal(38, 18),
    high Decimal(38, 18),
    low Decimal(38, 18),
    close Decimal(38, 18),
    volume Decimal(38, 18),
    quote_volume Decimal(38, 18),
    vwap_price Decimal(38, 18),
    exchange_count UInt8,
    contributing_exchanges Array(String),
    data_quality_score Decimal(5, 4),
    trade_count UInt32 DEFAULT 0,
    created_at DateTime64(3) DEFAULT now64(),
    version UInt64 DEFAULT toUnixTimestamp64Milli(now64())
) ENGINE = ReplacingMergeTree(version)
PARTITION BY toYYYYMM(timestamp)
ORDER BY (base_token_id, quote_token_id, timestamp)
TTL timestamp + INTERVAL 2 YEAR
SETTINGS index_granularity = 8192;

-- 1-hour candles (retain for 3 years)
CREATE TABLE IF NOT EXISTS ohlcv_1h (
    timestamp DateTime64(3),
    base_token_id UInt32,
    quote_token_id UInt32,
    open Decimal(38, 18),
    high Decimal(38, 18),
    low Decimal(38, 18),
    close Decimal(38, 18),
    volume Decimal(38, 18),
    quote_volume Decimal(38, 18),
    vwap_price Decimal(38, 18),
    exchange_count UInt8,
    contributing_exchanges Array(String),
    data_quality_score Decimal(5, 4),
    trade_count UInt32 DEFAULT 0,
    created_at DateTime64(3) DEFAULT now64(),
    version UInt64 DEFAULT toUnixTimestamp64Milli(now64())
) ENGINE = ReplacingMergeTree(version)
PARTITION BY toYYYYMM(timestamp)
ORDER BY (base_token_id, quote_token_id, timestamp)
TTL timestamp + INTERVAL 3 YEAR
SETTINGS index_granularity = 8192;

-- 4-hour candles (retain for 5 years)
CREATE TABLE IF NOT EXISTS ohlcv_4h (
    timestamp DateTime64(3),
    base_token_id UInt32,
    quote_token_id UInt32,
    open Decimal(38, 18),
    high Decimal(38, 18),
    low Decimal(38, 18),
    close Decimal(38, 18),
    volume Decimal(38, 18),
    quote_volume Decimal(38, 18),
    vwap_price Decimal(38, 18),
    exchange_count UInt8,
    contributing_exchanges Array(String),
    data_quality_score Decimal(5, 4),
    trade_count UInt32 DEFAULT 0,
    created_at DateTime64(3) DEFAULT now64(),
    version UInt64 DEFAULT toUnixTimestamp64Milli(now64())
) ENGINE = ReplacingMergeTree(version)
PARTITION BY toYYYYMM(timestamp)
ORDER BY (base_token_id, quote_token_id, timestamp)
TTL timestamp + INTERVAL 5 YEAR
SETTINGS index_granularity = 8192;

-- 1-day candles (retain indefinitely)
CREATE TABLE IF NOT EXISTS ohlcv_1d (
    timestamp DateTime64(3),
    base_token_id UInt32,
    quote_token_id UInt32,
    open Decimal(38, 18),
    high Decimal(38, 18),
    low Decimal(38, 18),
    close Decimal(38, 18),
    volume Decimal(38, 18),
    quote_volume Decimal(38, 18),
    vwap_price Decimal(38, 18),
    exchange_count UInt8,
    contributing_exchanges Array(String),
    data_quality_score Decimal(5, 4),
    trade_count UInt32 DEFAULT 0,
    market_cap Decimal(38, 18) DEFAULT 0,
    created_at DateTime64(3) DEFAULT now64(),
    version UInt64 DEFAULT toUnixTimestamp64Milli(now64())
) ENGINE = ReplacingMergeTree(version)
PARTITION BY toYYYYMM(timestamp)
ORDER BY (base_token_id, quote_token_id, timestamp)
SETTINGS index_granularity = 8192;

-- Create indexes for faster lookups
CREATE INDEX IF NOT EXISTS idx_ohlcv_1m_timestamp ON ohlcv_1m (timestamp);
CREATE INDEX IF NOT EXISTS idx_ohlcv_1m_tokens ON ohlcv_1m (base_token_id, quote_token_id);

CREATE INDEX IF NOT EXISTS idx_ohlcv_5m_timestamp ON ohlcv_5m (timestamp);
CREATE INDEX IF NOT EXISTS idx_ohlcv_5m_tokens ON ohlcv_5m (base_token_id, quote_token_id);

CREATE INDEX IF NOT EXISTS idx_ohlcv_15m_timestamp ON ohlcv_15m (timestamp);
CREATE INDEX IF NOT EXISTS idx_ohlcv_15m_tokens ON ohlcv_15m (base_token_id, quote_token_id);

CREATE INDEX IF NOT EXISTS idx_ohlcv_1h_timestamp ON ohlcv_1h (timestamp);
CREATE INDEX IF NOT EXISTS idx_ohlcv_1h_tokens ON ohlcv_1h (base_token_id, quote_token_id);

CREATE INDEX IF NOT EXISTS idx_ohlcv_4h_timestamp ON ohlcv_4h (timestamp);
CREATE INDEX IF NOT EXISTS idx_ohlcv_4h_tokens ON ohlcv_4h (base_token_id, quote_token_id);

CREATE INDEX IF NOT EXISTS idx_ohlcv_1d_timestamp ON ohlcv_1d (timestamp);
CREATE INDEX IF NOT EXISTS idx_ohlcv_1d_tokens ON ohlcv_1d (base_token_id, quote_token_id);