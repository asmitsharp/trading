-- Store VWAP-calculated prices (every 10-15 seconds)
CREATE TABLE IF NOT EXISTS vwap_prices (
    timestamp DateTime64(3),
    base_token_id String,
    quote_token_id String,
    vwap_price Decimal64(8),
    total_volume Decimal64(8),
    exchange_count UInt8,
    contributing_exchanges Array(String),
    -- Store individual exchange prices and volumes for debugging/analysis
    price_sources Array(Tuple(exchange String, price Decimal64(8), volume Decimal64(8))),
    created_at DateTime64(3) DEFAULT now64()
) ENGINE = MergeTree()
PARTITION BY toYYYYMMDD(timestamp)
ORDER BY (base_token_id, quote_token_id, timestamp)
TTL timestamp + INTERVAL 30 DAY DELETE
SETTINGS index_granularity = 8192;

-- Create materialized view for latest VWAP prices
CREATE MATERIALIZED VIEW IF NOT EXISTS latest_vwap_prices
ENGINE = ReplacingMergeTree(timestamp)
ORDER BY (base_token_id, quote_token_id)
AS SELECT
    base_token_id,
    quote_token_id,
    argMax(vwap_price, timestamp) as latest_price,
    argMax(total_volume, timestamp) as latest_volume,
    argMax(exchange_count, timestamp) as latest_exchange_count,
    max(timestamp) as timestamp
FROM vwap_prices
GROUP BY base_token_id, quote_token_id;-- Hot storage for recent candlestick data
CREATE TABLE IF NOT EXISTS candlesticks_hot (
    timestamp DateTime64(3),
    timeframe LowCardinality(String), -- '1m', '5m', '15m', '30m', '1h', '4h', '1d'
    base_token_id String,
    quote_token_id String,
    
    -- OHLCV built from VWAP prices
    open Decimal64(8),
    high Decimal64(8),
    low Decimal64(8),
    close Decimal64(8),
    volume Decimal64(8),
    
    -- Additional metadata
    trades_count UInt32,
    exchange_count UInt8,
    is_complete Bool DEFAULT false,
    
    created_at DateTime64(3) DEFAULT now64(),
    updated_at DateTime64(3) DEFAULT now64()
) ENGINE = ReplacingMergeTree(updated_at)
PARTITION BY (timeframe, toYYYYMM(timestamp))
ORDER BY (base_token_id, quote_token_id, timeframe, timestamp)
TTL timestamp + INTERVAL 90 DAY DELETE
SETTINGS index_granularity = 8192;

-- Warm storage for older data (compressed)
CREATE TABLE IF NOT EXISTS candlesticks_warm (
    timestamp DateTime64(3),
    timeframe LowCardinality(String),
    base_token_id String,
    quote_token_id String,
    ohlcv Array(Decimal64(8)), -- [open, high, low, close, volume]
    metadata String -- JSON string with additional data
) ENGINE = MergeTree()
PARTITION BY (timeframe, toYYYY(timestamp))
ORDER BY (base_token_id, quote_token_id, timeframe, timestamp)
TTL timestamp + INTERVAL 1 YEAR TO DISK 'default',
     timestamp + INTERVAL 3 YEAR DELETE
SETTINGS index_granularity = 16384;

-- Materialized view to auto-generate 1-minute candles from VWAP prices
CREATE MATERIALIZED VIEW IF NOT EXISTS candles_1m_mv
TO candlesticks_hot
AS SELECT
    toStartOfMinute(timestamp) as timestamp,
    '1m' as timeframe,
    base_token_id,
    quote_token_id,
    argMin(vwap_price, timestamp) as open,
    max(vwap_price) as high,
    min(vwap_price) as low,
    argMax(vwap_price, timestamp) as close,
    sum(total_volume) as volume,
    count() as trades_count,
    max(exchange_count) as exchange_count,
    true as is_complete,
    now64() as created_at,
    now64() as updated_at
FROM vwap_prices
GROUP BY
    toStartOfMinute(timestamp),
    base_token_id,
    quote_token_id;-- Track exchange API health and performance
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

-- Materialized view for exchange health statistics
CREATE MATERIALIZED VIEW IF NOT EXISTS exchange_health_stats
ENGINE = SummingMergeTree()
PARTITION BY toYYYYMMDD(timestamp)
ORDER BY (exchange_id, toStartOfHour(timestamp))
AS SELECT
    exchange_id,
    toStartOfHour(timestamp) as hour,
    count() as total_requests,
    countIf(success = true) as successful_requests,
    countIf(success = false) as failed_requests,
    avg(response_time_ms) as avg_response_time,
    max(response_time_ms) as max_response_time,
    min(response_time_ms) as min_response_time
FROM exchange_health
GROUP BY exchange_id, toStartOfHour(timestamp);