-- Create token_exchange_symbols table for mapping exchange-specific symbols to token IDs
CREATE TABLE IF NOT EXISTS token_exchange_symbols (
    id SERIAL PRIMARY KEY,
    token_id INTEGER NOT NULL REFERENCES tokens(id) ON DELETE CASCADE,
    exchange_id VARCHAR(50) NOT NULL,
    exchange_symbol VARCHAR(50) NOT NULL,
    normalized_symbol VARCHAR(50) NOT NULL, -- Standardized symbol (e.g., BTC, ETH)
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    
    -- Ensure unique mapping per exchange
    UNIQUE(exchange_id, exchange_symbol)
);

-- Create indexes for performance
CREATE INDEX idx_token_exchange_symbols_token_id ON token_exchange_symbols(token_id);
CREATE INDEX idx_token_exchange_symbols_exchange_id ON token_exchange_symbols(exchange_id);
CREATE INDEX idx_token_exchange_symbols_exchange_symbol ON token_exchange_symbols(exchange_id, exchange_symbol);
CREATE INDEX idx_token_exchange_symbols_normalized ON token_exchange_symbols(normalized_symbol);
CREATE INDEX idx_token_exchange_symbols_active ON token_exchange_symbols(is_active) WHERE is_active = true;

-- Create trading pairs table to map base/quote token combinations
CREATE TABLE IF NOT EXISTS trading_pairs (
    id SERIAL PRIMARY KEY,
    base_token_id INTEGER NOT NULL REFERENCES tokens(id) ON DELETE CASCADE,
    quote_token_id INTEGER NOT NULL REFERENCES tokens(id) ON DELETE CASCADE,
    exchange_id VARCHAR(50) NOT NULL,
    exchange_pair_symbol VARCHAR(100) NOT NULL, -- Full pair symbol on exchange (e.g., BTCUSDT)
    is_active BOOLEAN DEFAULT true,
    min_volume_threshold DECIMAL(20,2) DEFAULT 1000, -- Minimum volume for inclusion in VWAP
    last_volume_24h DECIMAL(20,2) DEFAULT 0,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    
    -- Ensure unique pair per exchange
    UNIQUE(exchange_id, exchange_pair_symbol)
);

-- Create indexes for trading pairs
CREATE INDEX idx_trading_pairs_base_token ON trading_pairs(base_token_id);
CREATE INDEX idx_trading_pairs_quote_token ON trading_pairs(quote_token_id);
CREATE INDEX idx_trading_pairs_exchange ON trading_pairs(exchange_id);
CREATE INDEX idx_trading_pairs_symbol ON trading_pairs(exchange_id, exchange_pair_symbol);
CREATE INDEX idx_trading_pairs_active ON trading_pairs(is_active) WHERE is_active = true;

-- Add update trigger for updated_at
CREATE TRIGGER update_token_exchange_symbols_updated_at BEFORE UPDATE ON token_exchange_symbols
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_trading_pairs_updated_at BEFORE UPDATE ON trading_pairs
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();