-- Exchange configuration for 30+ exchanges
CREATE TABLE IF NOT EXISTS exchanges (
    id SERIAL PRIMARY KEY,
    exchange_id VARCHAR(50) UNIQUE NOT NULL,
    name VARCHAR(100) NOT NULL,
    base_url TEXT NOT NULL,
    ticker_endpoint TEXT NOT NULL,
    symbols_endpoint TEXT NOT NULL,
    rate_limit_per_minute INTEGER DEFAULT 60,
    request_timeout_ms INTEGER DEFAULT 5000,
    retry_attempts INTEGER DEFAULT 3,
    weight DECIMAL(5,4) DEFAULT 0.01, -- Exchange weight for VWAP calculation
    symbol_format VARCHAR(20) DEFAULT 'BTCUSDT', -- Format example: BTCUSDT vs BTC-USDT
    quote_currencies TEXT[] DEFAULT '{}',
    headers JSONB DEFAULT '{}', -- Custom headers if needed
    api_key VARCHAR(255), -- Optional API key for some exchanges
    api_secret VARCHAR(255), -- Optional API secret
    is_active BOOLEAN DEFAULT true,
    last_successful_poll TIMESTAMP,
    consecutive_failures INTEGER DEFAULT 0,
    avg_response_time_ms INTEGER,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Index for active exchanges
CREATE INDEX idx_exchanges_active ON exchanges(is_active) WHERE is_active = true;
CREATE INDEX idx_exchanges_exchange_id ON exchanges(exchange_id);
CREATE INDEX idx_exchanges_last_poll ON exchanges(last_successful_poll);

-- Create update trigger
CREATE TRIGGER update_exchanges_updated_at BEFORE UPDATE ON exchanges
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();