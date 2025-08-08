-- Add mapping method tracking to token_exchange_symbols
ALTER TABLE token_exchange_symbols
ADD COLUMN mapping_method VARCHAR(20) DEFAULT 'manual',
ADD COLUMN confidence_score DECIMAL(3,2) DEFAULT 1.00,
ADD COLUMN needs_verification BOOLEAN DEFAULT FALSE,
ADD COLUMN verified_by VARCHAR(100),
ADD COLUMN verified_at TIMESTAMP,
ADD COLUMN last_price_check TIMESTAMP;

-- Add mapping method tracking to trading_pairs
ALTER TABLE trading_pairs
ADD COLUMN mapping_method VARCHAR(20) DEFAULT 'manual',
ADD COLUMN confidence_score DECIMAL(3,2) DEFAULT 1.00,
ADD COLUMN needs_verification BOOLEAN DEFAULT FALSE,
ADD COLUMN verified_by VARCHAR(100),
ADD COLUMN verified_at TIMESTAMP;

-- Create indexes for verification queries
CREATE INDEX idx_token_symbols_verification ON token_exchange_symbols(needs_verification, mapping_method);
CREATE INDEX idx_trading_pairs_verification ON trading_pairs(needs_verification, mapping_method);