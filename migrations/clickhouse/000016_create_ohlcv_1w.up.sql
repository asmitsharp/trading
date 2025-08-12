-- 1-week candles (retain indefinitely)
CREATE TABLE IF NOT EXISTS ohlcv_1w (
    timestamp DateTime64(3),
    base_token_id UInt32,
    quote_token_id UInt32,
    exchange_id String,
    open Decimal(38, 18),
    high Decimal(38, 18),
    low Decimal(38, 18),
    close Decimal(38, 18),
    volume Decimal(38, 18),
    quote_volume Decimal(38, 18),
    trade_count UInt32,
    vwap_price Decimal(38, 18),
    created_at DateTime64(3) DEFAULT now64(),
    version UInt64 DEFAULT toUnixTimestamp64Milli(now64())
) ENGINE = ReplacingMergeTree(version)
PARTITION BY toYear(timestamp)
ORDER BY (base_token_id, quote_token_id, exchange_id, timestamp)
SETTINGS index_granularity = 8192;