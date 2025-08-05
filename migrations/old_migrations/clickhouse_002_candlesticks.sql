-- Hot storage for recent candlestick data
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
    quote_token_id;