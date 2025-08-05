-- Update ClickHouse schema to use integer token IDs

-- Drop existing views and tables
DROP VIEW IF EXISTS latest_vwap_prices;
DROP VIEW IF EXISTS exchange_health_stats;
DROP VIEW IF EXISTS candles_1m_mv;
DROP TABLE IF EXISTS vwap_prices;
DROP TABLE IF EXISTS candlesticks_hot;
DROP TABLE IF EXISTS candlesticks_warm;

-- 1. Updated VWAP Prices Table with integer token IDs
CREATE TABLE IF NOT EXISTS vwap_prices (
    timestamp DateTime64(3),
    base_token_id UInt32,      -- Changed from String to UInt32
    quote_token_id UInt32,     -- Changed from String to UInt32
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

-- 2. Updated Candlesticks Hot Storage with integer token IDs
CREATE TABLE IF NOT EXISTS candlesticks_hot (
    timestamp DateTime64(3),
    timeframe LowCardinality(String),
    base_token_id UInt32,      -- Changed from String to UInt32
    quote_token_id UInt32,     -- Changed from String to UInt32
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

-- 3. Create new trades table with token IDs
CREATE TABLE IF NOT EXISTS trades (
    timestamp DateTime64(3),
    exchange_id LowCardinality(String),
    base_token_id UInt32,      -- New field
    quote_token_id UInt32,     -- New field
    symbol LowCardinality(String), -- Keep for reference
    price Decimal64(8),
    quantity Decimal64(8),
    trade_id UInt64,
    is_buyer_maker UInt8,
    created_at DateTime64(3) DEFAULT now64()
) ENGINE = MergeTree()
PARTITION BY (exchange_id, toYYYYMMDD(timestamp))
ORDER BY (base_token_id, quote_token_id, exchange_id, timestamp)
TTL timestamp + INTERVAL 7 DAY DELETE
SETTINGS index_granularity = 8192;

-- 4. Create materialized view for 1-minute OHLCV with token IDs
CREATE MATERIALIZED VIEW IF NOT EXISTS trades_ohlcv_1m
ENGINE = AggregatingMergeTree()
PARTITION BY (exchange_id, toYYYYMM(minute))
ORDER BY (base_token_id, quote_token_id, exchange_id, minute)
AS SELECT
    base_token_id,
    quote_token_id,
    exchange_id,
    toStartOfMinute(timestamp) as minute,
    argMinState(price, timestamp) as open,
    maxState(price) as high,
    minState(price) as low,
    argMaxState(price, timestamp) as close,
    sumState(quantity) as volume,
    countState() as trades_count
FROM trades
GROUP BY base_token_id, quote_token_id, exchange_id, minute;

-- 5. Create view for latest VWAP prices
CREATE VIEW IF NOT EXISTS latest_vwap_prices AS
SELECT 
    base_token_id,
    quote_token_id,
    argMax(vwap_price, timestamp) as latest_price,
    argMax(total_volume, timestamp) as latest_volume,
    argMax(exchange_count, timestamp) as exchange_count,
    max(timestamp) as last_update
FROM vwap_prices
GROUP BY base_token_id, quote_token_id;

-- 6. Create price ticker table for real-time updates
CREATE TABLE IF NOT EXISTS price_tickers (
    timestamp DateTime64(3),
    exchange_id LowCardinality(String),
    base_token_id UInt32,
    quote_token_id UInt32,
    price Decimal64(8),
    volume_24h Decimal64(8),
    quote_volume_24h Decimal64(8),
    high_24h Decimal64(8),
    low_24h Decimal64(8),
    price_change_24h Decimal64(8),
    created_at DateTime64(3) DEFAULT now64()
) ENGINE = ReplacingMergeTree(created_at)
PARTITION BY toYYYYMMDD(timestamp)
ORDER BY (base_token_id, quote_token_id, exchange_id, timestamp)
TTL timestamp + INTERVAL 1 DAY DELETE
SETTINGS index_granularity = 8192;

-- Show created tables
SHOW TABLES;