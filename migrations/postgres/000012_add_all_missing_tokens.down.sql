-- Rollback: Remove all tokens added in the up migration
-- This would remove 1319 tokens added from exchange analysis

-- Since we don't want to accidentally delete other tokens, we'll only delete tokens
-- that were specifically added by this migration based on their metadata
DELETE FROM tokens 
WHERE metadata ? 'exchange_count' 
  AND metadata ? 'occurrence_count'
  AND created_at >= (SELECT MAX(created_at) FROM tokens WHERE metadata ? 'exchange_count' LIMIT 1);

-- Note: This is a conservative rollback that only removes tokens with the specific metadata pattern
-- used in this migration. If you need a more aggressive rollback, you can uncomment the following:
-- DELETE FROM tokens WHERE symbol IN (
--   'PEPE', 'UNI', 'TRUMP', 'SAHARA', 'MOODENG', 'SPK', 'CRO', 'WBTC', ... [list all 1319 symbols]
-- );