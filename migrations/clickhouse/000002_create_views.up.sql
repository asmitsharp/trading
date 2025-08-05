-- Create materialized view for 1-minute OHLCV with token IDs
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

-- Create view for latest VWAP prices
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

-- Create view for exchange health statistics
CREATE VIEW IF NOT EXISTS exchange_health_stats AS
SELECT 
    exchange_id,
    countIf(success = true) as successful_polls,
    countIf(success = false) as failed_polls,
    avg(response_time_ms) as avg_response_time,
    max(timestamp) as last_poll_time,
    countIf(success = false AND timestamp > now() - INTERVAL 1 HOUR) as recent_failures
FROM exchange_health
WHERE timestamp > now() - INTERVAL 24 HOUR
GROUP BY exchange_id;