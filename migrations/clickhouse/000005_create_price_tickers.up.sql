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
SETTINGS index_granularity = 8192