-- Track exchange API health and performance
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