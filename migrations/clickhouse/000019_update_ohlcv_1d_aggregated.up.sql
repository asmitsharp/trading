-- Update ohlcv_1d table to store aggregated data across exchanges (CoinMarketCap style)

-- First, backup existing data if any
CREATE TABLE IF NOT EXISTS ohlcv_1d_backup AS SELECT * FROM ohlcv_1d;

-- Drop the existing table
DROP TABLE IF EXISTS ohlcv_1d;

-- Create new aggregated ohlcv_1d table (without exchange_id)
CREATE TABLE IF NOT EXISTS ohlcv_1d (
    timestamp DateTime64(3),
    base_token_id UInt32,
    quote_token_id UInt32,
    
    -- Core OHLCV data (all normalized to USD equivalent)
    open Decimal(38, 18),
    high Decimal(38, 18),
    low Decimal(38, 18),
    close Decimal(38, 18),
    volume Decimal(38, 18),           -- Total volume across all exchanges (USD equivalent)
    quote_volume Decimal(38, 18),     -- Total quote volume (USD)
    
    -- Aggregation metadata
    vwap_price Decimal(38, 18),       -- Volume-weighted average price
    exchange_count UInt8,             -- Number of contributing exchanges
    contributing_exchanges Array(String), -- List of exchanges that contributed
    data_quality_score Decimal(5, 4), -- 0.0-1.0 quality score based on exchange reliability
    
    -- Market data
    market_cap Decimal(38, 18) DEFAULT 0,
    trade_count UInt32 DEFAULT 0,     -- Total number of data points aggregated
    
    -- System fields
    created_at DateTime64(3) DEFAULT now64(),
    version UInt64 DEFAULT toUnixTimestamp64Milli(now64())
) ENGINE = ReplacingMergeTree(version)
PARTITION BY toYYYYMM(timestamp)
ORDER BY (base_token_id, quote_token_id, timestamp)
SETTINGS index_granularity = 8192;

-- Create index for faster lookups
CREATE INDEX IF NOT EXISTS idx_ohlcv_1d_timestamp ON ohlcv_1d (timestamp);
CREATE INDEX IF NOT EXISTS idx_ohlcv_1d_tokens ON ohlcv_1d (base_token_id, quote_token_id);

-- Drop the backup table if migration is successful
-- DROP TABLE IF EXISTS ohlcv_1d_backup;