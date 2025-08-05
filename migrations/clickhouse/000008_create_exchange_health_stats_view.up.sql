-- Create view for exchange health statistics
CREATE VIEW IF NOT EXISTS exchange_health_stats AS
SELECT 
    exchange_id,
    countIf(success = true) as successful_polls,
    countIf(success = false) as failed_polls,
    avg(response_time_ms) as avg_response_time,
    max(timestamp) as last_poll_time,
    countIf(success = false AND timestamp > now() - INTERVAL 1 HOUR) as recent_failures
FROM exchange_health
WHERE timestamp > now() - INTERVAL 24 HOUR
GROUP BY exchange_id