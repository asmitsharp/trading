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
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();