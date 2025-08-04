-- Trading pairs for VWAP calculation
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