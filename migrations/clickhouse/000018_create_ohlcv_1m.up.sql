-- 1-minute candles table (aggregated cross-exchange data)
-- Retains data for 30 days due to high volume
CREATE TABLE IF NOT EXISTS ohlcv_1m (
    timestamp DateTime64(3),
    base_token_id UInt32,
    quote_token_id UInt32,
    
    -- Core OHLCV data (aggregated across exchanges)
    open Decimal(38, 18),
    high Decimal(38, 18),
    low Decimal(38, 18),
    close Decimal(38, 18),
    volume Decimal(38, 18),           -- Total volume across exchanges (USD equivalent)
    quote_volume Decimal(38, 18),     -- Total quote volume (USD)
    
    -- Aggregation metadata
    vwap_price Decimal(38, 18),       -- Volume-weighted average price
    exchange_count UInt8,             -- Number of contributing exchanges
    contributing_exchanges Array(String), -- List of exchanges that contributed
    data_quality_score Decimal(5, 4), -- 0.0-1.0 quality score
    
    -- Market data
    trade_count UInt32 DEFAULT 0,     -- Total trades aggregated
    
    -- System fields
    created_at DateTime64(3) DEFAULT now64(),
    version UInt64 DEFAULT toUnixTimestamp64Milli(now64())
) ENGINE = ReplacingMergeTree(version)
PARTITION BY toYYYYMM(timestamp)
ORDER BY (base_token_id, quote_token_id, timestamp)
TTL timestamp + INTERVAL 30 DAY
SETTINGS index_granularity = 8192;

-- Create indexes for faster lookups
CREATE INDEX IF NOT EXISTS idx_ohlcv_1m_timestamp ON ohlcv_1m (timestamp);
CREATE INDEX IF NOT EXISTS idx_ohlcv_1m_tokens ON ohlcv_1m (base_token_id, quote_token_id);