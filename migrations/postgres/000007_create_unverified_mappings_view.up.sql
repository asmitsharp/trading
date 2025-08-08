-- Create view for unverified symbol-based mappings
CREATE VIEW unverified_mappings AS
SELECT 
    tes.id,
    tes.exchange_id,
    tes.exchange_symbol,
    t.symbol as token_symbol,
    t.name as token_name,
    tes.mapping_method,
    tes.confidence_score,
    tes.created_at,
    tes.last_price_check,
    EXISTS(
        SELECT 1 FROM price_outliers po 
        WHERE po.exchange_id = tes.exchange_id 
        AND po.base_token_id = tes.token_id 
        AND po.is_resolved = false
    ) as has_outliers
FROM token_exchange_symbols tes
JOIN tokens t ON tes.token_id = t.id
WHERE tes.needs_verification = true
    AND tes.mapping_method = 'symbol'
ORDER BY tes.confidence_score ASC, tes.created_at DESC;