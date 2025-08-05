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
SETTINGS index_granularity = 8192