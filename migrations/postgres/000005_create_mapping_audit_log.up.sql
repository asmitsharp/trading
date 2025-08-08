-- Create mapping audit log table to track all mapping changes
CREATE TABLE mapping_audit_log (
    id SERIAL PRIMARY KEY,
    token_id INTEGER REFERENCES tokens(id),
    exchange_id VARCHAR(50) NOT NULL,
    exchange_symbol VARCHAR(50) NOT NULL,
    mapping_method VARCHAR(20) NOT NULL, -- 'slug', 'symbol', 'manual', 'fuzzy'
    confidence_score DECIMAL(3,2) DEFAULT 0.50,
    action VARCHAR(50) NOT NULL, -- 'created', 'updated', 'verified', 'flagged'
    performed_by VARCHAR(100),
    notes TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Create index for audit queries
CREATE INDEX idx_mapping_audit_token ON mapping_audit_log(token_id, exchange_id, created_at DESC);
CREATE INDEX idx_mapping_audit_exchange ON mapping_audit_log(exchange_id, created_at DESC);