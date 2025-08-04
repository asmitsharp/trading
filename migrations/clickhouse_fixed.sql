-- Fixed ClickHouse migrations for crypto platform

-- Drop existing tables if needed
DROP TABLE IF EXISTS vwap_prices;
DROP TABLE IF EXISTS candlesticks_hot;
DROP TABLE IF EXISTS candlesticks_warm;
DROP TABLE IF EXISTS exchange_health;
DROP VIEW IF EXISTS latest_vwap_prices;
DROP VIEW IF EXISTS exchange_health_stats;
DROP VIEW IF EXISTS candles_1m_mv;

-- 1. VWAP Prices Table
CREATE TABLE IF NOT EXISTS vwap_prices (
    timestamp DateTime64(3),
    base_token_id String,
    quote_token_id String,
    vwap_price Decimal64(8),
    total_volume Decimal64(8),
    exchange_count UInt8,
    contributing_exchanges Array(String),
    created_at DateTime64(3) DEFAULT now64()
) ENGINE = MergeTree()
PARTITION BY toYYYYMMDD(timestamp)
ORDER BY (base_token_id, quote_token_id, timestamp)
TTL timestamp + INTERVAL 30 DAY DELETE
SETTINGS index_granularity = 8192;

-- 2. Candlesticks Hot Storage
CREATE TABLE IF NOT EXISTS candlesticks_hot (
    timestamp DateTime64(3),
    timeframe LowCardinality(String),
    base_token_id String,
    quote_token_id String,
    open Decimal64(8),
    high Decimal64(8),
    low Decimal64(8),
    close Decimal64(8),
    volume Decimal64(8),
    trades_count UInt32 DEFAULT 0,
    exchange_count UInt8 DEFAULT 0,
    is_complete Bool DEFAULT false,
    created_at DateTime64(3) DEFAULT now64(),
    updated_at DateTime64(3) DEFAULT now64()
) ENGINE = ReplacingMergeTree(updated_at)
PARTITION BY (timeframe, toYYYYMM(timestamp))
ORDER BY (base_token_id, quote_token_id, timeframe, timestamp)
TTL timestamp + INTERVAL 90 DAY DELETE
SETTINGS index_granularity = 8192;

-- 3. Exchange Health Monitoring
CREATE TABLE IF NOT EXISTS exchange_health (
    timestamp DateTime64(3),
    exchange_id String,
    response_time_ms UInt32,
    success Bool,
    error_message String,
    symbols_fetched UInt32,
    rate_limit_remaining UInt32,
    http_status_code UInt16,
    created_at DateTime64(3) DEFAULT now64()
) ENGINE = MergeTree()
PARTITION BY toYYYYMMDD(timestamp)
ORDER BY (exchange_id, timestamp)
TTL timestamp + INTERVAL 7 DAY DELETE
SETTINGS index_granularity = 8192;

-- Show created tables
SHOW TABLES;