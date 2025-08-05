-- 1. VWAP Prices Table with integer token IDs
CREATE TABLE IF NOT EXISTS vwap_prices (
    timestamp DateTime64(3),
    base_token_id UInt32,
    quote_token_id UInt32,
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

-- 2. Candlesticks Hot Storage with integer token IDs
CREATE TABLE IF NOT EXISTS candlesticks_hot (
    timestamp DateTime64(3),
    timeframe LowCardinality(String),
    base_token_id UInt32,
    quote_token_id UInt32,
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

-- 4. Trades table with token IDs
CREATE TABLE IF NOT EXISTS trades (
    timestamp DateTime64(3),
    exchange_id LowCardinality(String),
    base_token_id UInt32,
    quote_token_id UInt32,
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

-- 5. Price ticker table for real-time updates
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