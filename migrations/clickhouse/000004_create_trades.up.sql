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
SETTINGS index_granularity = 8192