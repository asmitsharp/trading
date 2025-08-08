-- Create table to track price outliers for mapping verification
CREATE TABLE price_outliers (
    id SERIAL PRIMARY KEY,
    exchange_id VARCHAR(50) NOT NULL,
    base_token_id INTEGER REFERENCES tokens(id),
    quote_token_id INTEGER REFERENCES tokens(id),
    exchange_price DECIMAL(20, 8),
    average_price DECIMAL(20, 8),
    deviation_percent DECIMAL(10, 4),
    standard_deviations DECIMAL(10, 4),
    mapping_method VARCHAR(20),
    is_resolved BOOLEAN DEFAULT FALSE,
    resolution_notes TEXT,
    detected_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    resolved_at TIMESTAMP,
    resolved_by VARCHAR(100)
);

-- Create indexes for outlier queries
CREATE INDEX idx_outliers_unresolved ON price_outliers(is_resolved, detected_at DESC);
CREATE INDEX idx_outliers_exchange ON price_outliers(exchange_id, is_resolved);
CREATE INDEX idx_outliers_tokens ON price_outliers(base_token_id, quote_token_id, detected_at DESC);