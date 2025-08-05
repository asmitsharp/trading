-- Exchange symbol mappings for token disambiguation
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
CREATE INDEX idx_exchange_symbols_stablecoin ON exchange_symbols(is_stablecoin_pair) WHERE is_stablecoin_pair = true;