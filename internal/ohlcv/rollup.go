package ohlcv

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// RollupCandles rolls up lower timeframe candles to higher timeframes
func (c *Converter) RollupCandles(ctx context.Context, from, to Timeframe, startTime, endTime time.Time) error {
	fromTable := c.getTableName(from)
	toTable := c.getTableName(to)
	intervalExpr := c.getIntervalExpression(to)

	query := fmt.Sprintf(`
		INSERT INTO %s (
			timestamp, base_token_id, quote_token_id, exchange_id,
			open, high, low, close, volume, quote_volume,
			trade_count, vwap_price, version
		)
		SELECT
			%s AS timestamp,
			base_token_id,
			quote_token_id,
			exchange_id,
			argMin(open, timestamp) AS open,  -- First open chronologically
			max(high) AS high,
			min(low) AS low,
			argMax(close, timestamp) AS close,  -- Last close chronologically
			anyLast(volume) AS volume,  -- Use last volume (24h volume approach)
			anyLast(quote_volume) AS quote_volume,  -- Use last quote volume
			sum(trade_count) AS trade_count,
			if(sum(toFloat64(volume)) > 0,
				toDecimal128(sum(toFloat64(vwap_price) * toFloat64(volume)) / sum(toFloat64(volume)), 18),
				toDecimal128(avg(toFloat64(vwap_price)), 18)) AS vwap_price,
			toUnixTimestamp64Milli(now64()) AS version
		FROM %s
		WHERE timestamp >= ? AND timestamp < ?
		GROUP BY 
			%s,
			base_token_id,
			quote_token_id,
			exchange_id
	`, toTable, intervalExpr, fromTable, intervalExpr)

	err := c.clickhouseDB.Exec(ctx, query, startTime, endTime)
	if err != nil {
		return fmt.Errorf("rolling up %s to %s: %w", from, to, err)
	}

	c.logger.Info("Rolled up candles",
		zap.String("from", string(from)),
		zap.String("to", string(to)),
		zap.Time("start", startTime),
		zap.Time("end", endTime))

	return nil
}

// HierarchicalRollup performs cascading rollup from 1m to all higher timeframes
func (c *Converter) HierarchicalRollup(ctx context.Context, baseTime time.Time) error {
	// Ensure UTC timezone and round to start of minute
	startTime := baseTime.UTC().Truncate(time.Minute)

	// 1m -> 5m (every 5 minutes)
	if startTime.Minute()%5 == 0 {
		if err := c.RollupCandles(ctx, Timeframe1m, Timeframe5m,
			startTime.Add(-5*time.Minute), startTime); err != nil {
			return fmt.Errorf("1m->5m rollup: %w", err)
		}
	}

	// 5m -> 15m (every 15 minutes)
	if startTime.Minute()%15 == 0 {
		if err := c.RollupCandles(ctx, Timeframe5m, Timeframe15m,
			startTime.Add(-15*time.Minute), startTime); err != nil {
			return fmt.Errorf("5m->15m rollup: %w", err)
		}
	}

	// 15m -> 1h (every hour)
	if startTime.Minute() == 0 {
		if err := c.RollupCandles(ctx, Timeframe15m, Timeframe1h,
			startTime.Add(-1*time.Hour), startTime); err != nil {
			return fmt.Errorf("15m->1h rollup: %w", err)
		}
	}

	// 1h -> 4h (every 4 hours)
	if startTime.Hour()%4 == 0 && startTime.Minute() == 0 {
		if err := c.RollupCandles(ctx, Timeframe1h, Timeframe4h,
			startTime.Add(-4*time.Hour), startTime); err != nil {
			return fmt.Errorf("1h->4h rollup: %w", err)
		}
	}

	// 4h -> 1d (at midnight UTC)
	if startTime.Hour() == 0 && startTime.Minute() == 0 {
		if err := c.RollupCandles(ctx, Timeframe4h, Timeframe1d,
			startTime.Add(-24*time.Hour), startTime); err != nil {
			return fmt.Errorf("4h->1d rollup: %w", err)
		}
	}

	// 1d -> 1w (on Mondays at midnight UTC)
	if startTime.Weekday() == time.Monday && startTime.Hour() == 0 && startTime.Minute() == 0 {
		if err := c.RollupCandles(ctx, Timeframe1d, Timeframe1w,
			startTime.Add(-7*24*time.Hour), startTime); err != nil {
			return fmt.Errorf("1d->1w rollup: %w", err)
		}
	}

	return nil
}

// AggregateExchanges combines OHLCV data from multiple exchanges
func (c *Converter) AggregateExchanges(ctx context.Context, timeframe Timeframe, startTime, endTime time.Time) error {
	table := c.getTableName(timeframe)

	query := fmt.Sprintf(`
		INSERT INTO ohlcv_aggregated (
			timestamp, timeframe, base_token_id, quote_token_id,
			open, high, low, close, volume, quote_volume,
			trade_count, vwap_price, exchange_count, version
		)
		SELECT
			timestamp,
			'%s' AS timeframe,
			base_token_id,
			quote_token_id,
			min(open) AS open,  -- Lowest open price across exchanges
			max(high) AS high,
			min(low) AS low,
			max(close) AS close,  -- Highest close price across exchanges
			max(volume) AS volume,  -- Use max volume across exchanges (24h volume approach)
			max(quote_volume) AS quote_volume,
			sum(trade_count) AS trade_count,
			if(sum(toFloat64(volume)) > 0,
				toDecimal128(sum(toFloat64(vwap_price) * toFloat64(volume)) / sum(toFloat64(volume)), 18),
				toDecimal128(avg(toFloat64(vwap_price)), 18)) AS vwap_price,
			count(DISTINCT exchange_id) AS exchange_count,
			toUnixTimestamp64Milli(now64()) AS version
		FROM %s
		WHERE timestamp >= ? AND timestamp < ?
		GROUP BY 
			timestamp,
			base_token_id,
			quote_token_id
	`, timeframe, table)

	err := c.clickhouseDB.Exec(ctx, query, startTime, endTime)
	if err != nil {
		return fmt.Errorf("aggregating exchanges for %s: %w", timeframe, err)
	}

	return nil
}

// OptimizeTables runs OPTIMIZE to merge parts and apply deduplication
func (c *Converter) OptimizeTables(ctx context.Context) error {
	tables := []string{
		"ohlcv_1m", "ohlcv_5m", "ohlcv_15m", "ohlcv_1h",
		"ohlcv_4h", "ohlcv_1d", "ohlcv_1w", "ohlcv_aggregated",
	}

	for _, table := range tables {
		query := fmt.Sprintf("OPTIMIZE TABLE %s FINAL", table)
		if err := c.clickhouseDB.Exec(ctx, query); err != nil {
			c.logger.Warn("Failed to optimize table",
				zap.String("table", table),
				zap.Error(err))
		}
	}

	return nil
}