-- Create view for latest VWAP prices
CREATE VIEW IF NOT EXISTS latest_vwap_prices AS
SELECT 
    base_token_id,
    quote_token_id,
    argMax(vwap_price, timestamp) as latest_price,
    argMax(total_volume, timestamp) as latest_volume,
    argMax(exchange_count, timestamp) as exchange_count,
    max(timestamp) as last_update
FROM vwap_prices
GROUP BY base_token_id, quote_token_id