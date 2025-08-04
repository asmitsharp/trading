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
GROUP BY base_token_id, quote_token_id;