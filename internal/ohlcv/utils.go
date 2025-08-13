package ohlcv

import "fmt"

// getTableName returns the table name for a timeframe
func (c *Converter) getTableName(tf Timeframe) string {
	return fmt.Sprintf("ohlcv_%s", tf)
}

// getIntervalExpression returns the complete interval expression for GROUP BY and SELECT
func (c *Converter) getIntervalExpression(tf Timeframe) string {
	switch tf {
	case Timeframe5m:
		return "toStartOfInterval(timestamp, INTERVAL 5 MINUTE)"
	case Timeframe15m:
		return "toStartOfInterval(timestamp, INTERVAL 15 MINUTE)"
	case Timeframe1h:
		return "toStartOfInterval(timestamp, INTERVAL 1 HOUR)"
	case Timeframe4h:
		return "toStartOfInterval(timestamp, INTERVAL 4 HOUR)"
	case Timeframe1d:
		return "toStartOfDay(timestamp)"
	case Timeframe1w:
		return "toStartOfWeek(timestamp)"
	default:
		return "toStartOfMinute(timestamp)"
	}
}