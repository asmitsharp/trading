-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Main tokens table (denormalized for performance)
CREATE TABLE IF NOT EXISTS tokens (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    symbol VARCHAR(20) NOT NULL,
    name VARCHAR(100) NOT NULL,
    contract_address TEXT,
    chain VARCHAR(50),
    decimals INTEGER DEFAULT 18,
    
    -- Market Data (Updated every 10-15 seconds)
    current_price DECIMAL(20,8) DEFAULT 0,
    market_cap DECIMAL(20,2) DEFAULT 0,
    fully_diluted_valuation DECIMAL(20,2) DEFAULT 0,
    trading_volume_24h DECIMAL(20,2) DEFAULT 0,
    
    -- Supply Information
    circulating_supply DECIMAL(30,0) DEFAULT 0,
    total_supply DECIMAL(30,0) DEFAULT 0,
    max_supply DECIMAL(30,0),
    
    -- Price History & Changes
    price_change_24h DECIMAL(10,4) DEFAULT 0,
    price_change_7d DECIMAL(10,4) DEFAULT 0,
    price_change_30d DECIMAL(10,4) DEFAULT 0,
    all_time_high DECIMAL(20,8) DEFAULT 0,
    all_time_high_date DATE,
    all_time_low DECIMAL(20,8) DEFAULT 0,
    all_time_low_date DATE,
    
    -- Rankings and Categories
    market_cap_rank INTEGER,
    categories TEXT[] DEFAULT '{}',
    
    -- Metadata (social links, explorers, wallets, etc.)
    metadata JSONB DEFAULT '{}',
    
    -- System Fields
    is_active BOOLEAN DEFAULT true,
    last_price_update TIMESTAMP DEFAULT NOW(),
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Optimized indexes for fast lookups
CREATE INDEX idx_tokens_symbol_active ON tokens(symbol) WHERE is_active = true;
CREATE INDEX idx_tokens_rank ON tokens(market_cap_rank) WHERE market_cap_rank IS NOT NULL;
CREATE INDEX idx_tokens_metadata ON tokens USING GIN(metadata);
CREATE INDEX idx_tokens_last_update ON tokens(last_price_update);
CREATE INDEX idx_tokens_categories ON tokens USING GIN(categories);

-- Create update trigger for updated_at
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

CREATE TRIGGER update_tokens_updated_at BEFORE UPDATE ON tokens
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();-- Exchange configuration for 30+ exchanges
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
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();-- Exchange symbol mappings for token disambiguation
CREATE TABLE IF NOT EXISTS exchange_symbols (
    id SERIAL PRIMARY KEY,
    exchange_id INTEGER REFERENCES exchanges(id) ON DELETE CASCADE,
    exchange_symbol VARCHAR(50) NOT NULL, -- e.g., "BTCUSDT" on Binance
    base_token_id UUID REFERENCES tokens(id) ON DELETE CASCADE,
    quote_token_id UUID REFERENCES tokens(id) ON DELETE CASCADE,
    is_active BOOLEAN DEFAULT true,
    is_stablecoin_pair BOOLEAN DEFAULT false, -- Track USD/USDT/USDC pairs
    last_seen TIMESTAMP DEFAULT NOW(),
    created_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(exchange_id, exchange_symbol)
);

-- Indexes for fast lookups
CREATE INDEX idx_exchange_symbols_exchange_id ON exchange_symbols(exchange_id);
CREATE INDEX idx_exchange_symbols_base_token ON exchange_symbols(base_token_id);
CREATE INDEX idx_exchange_symbols_quote_token ON exchange_symbols(quote_token_id);
CREATE INDEX idx_exchange_symbols_active ON exchange_symbols(is_active) WHERE is_active = true;
CREATE INDEX idx_exchange_symbols_stablecoin ON exchange_symbols(is_stablecoin_pair) WHERE is_stablecoin_pair = true;-- Trading pairs for VWAP calculation
CREATE TABLE IF NOT EXISTS trading_pairs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    base_token_id UUID REFERENCES tokens(id) ON DELETE CASCADE,
    quote_token_id UUID REFERENCES tokens(id) ON DELETE CASCADE,
    is_primary_pair BOOLEAN DEFAULT false, -- USD/USDT pairs for main pricing
    min_exchanges_required INTEGER DEFAULT 3, -- Minimum exchanges for VWAP
    total_exchanges INTEGER DEFAULT 0, -- Current number of exchanges supporting this pair
    last_vwap_price DECIMAL(20,8),
    last_vwap_update TIMESTAMP,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(base_token_id, quote_token_id)
);

-- Indexes for performance
CREATE INDEX idx_trading_pairs_base_token ON trading_pairs(base_token_id);
CREATE INDEX idx_trading_pairs_quote_token ON trading_pairs(quote_token_id);
CREATE INDEX idx_trading_pairs_primary ON trading_pairs(is_primary_pair) WHERE is_primary_pair = true;
CREATE INDEX idx_trading_pairs_vwap_update ON trading_pairs(last_vwap_update);

-- Create update trigger
CREATE TRIGGER update_trading_pairs_updated_at BEFORE UPDATE ON trading_pairs
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();