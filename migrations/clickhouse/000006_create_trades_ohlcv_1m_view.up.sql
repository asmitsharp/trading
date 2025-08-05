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
GROUP BY base_token_id, quote_token_id, exchange_id, minute