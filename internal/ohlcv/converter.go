package ohlcv

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// Timeframe represents OHLCV timeframe
type Timeframe string

const (
	Timeframe1m  Timeframe = "1m"
	Timeframe5m  Timeframe = "5m"
	Timeframe15m Timeframe = "15m"
	Timeframe1h  Timeframe = "1h"
	Timeframe4h  Timeframe = "4h"
	Timeframe1d  Timeframe = "1d"
	Timeframe1w  Timeframe = "1w"
)

// Candle represents OHLCV data
type Candle struct {
	Timestamp    time.Time
	BaseTokenID  uint32
	QuoteTokenID uint32
	ExchangeID   string
	Open         decimal.Decimal
	High         decimal.Decimal
	Low          decimal.Decimal
	Close        decimal.Decimal
	Volume       decimal.Decimal
	QuoteVolume  decimal.Decimal
	TradeCount   uint32
	VWAPPrice    decimal.Decimal
	Version      uint64
}

// Converter handles OHLCV conversion and aggregation
type Converter struct {
	clickhouseDB clickhouse.Conn
	logger       *zap.Logger
}

// NewConverter creates a new OHLCV converter
func NewConverter(db clickhouse.Conn, logger *zap.Logger) *Converter {
	return &Converter{
		clickhouseDB: db,
		logger:       logger,
	}
}

// GenerateFromPriceTickers generates 1-minute OHLCV from price tickers
func (c *Converter) GenerateFromPriceTickers(ctx context.Context, startTime, endTime time.Time) error {
	query := `
		INSERT INTO ohlcv_1m (
			timestamp, base_token_id, quote_token_id, exchange_id,
			open, high, low, close, volume, quote_volume,
			trade_count, vwap_price, version
		)
		SELECT
			toStartOfMinute(timestamp) AS minute_timestamp,
			base_token_id,
			quote_token_id,
			exchange_id,
			any(price) AS open,  -- First price in the minute
			max(price) AS high,
			min(price) AS low,
			anyLast(price) AS close,  -- Last price in the minute
			sum(volume_24h) / 1440 AS volume,
			sum(volume_24h * price) / 1440 AS quote_volume,
			count() AS trade_count,
			if(sum(volume_24h) > 0, 
				sum(price * volume_24h) / sum(volume_24h), 
				avg(price)) AS vwap_price,
			toUnixTimestamp64Milli(now64()) AS version
		FROM price_tickers
		WHERE timestamp >= ? AND timestamp < ?
			AND base_token_id > 0 AND quote_token_id > 0
		GROUP BY 
			minute_timestamp,
			base_token_id,
			quote_token_id,
			exchange_id
		HAVING count() > 0
	`

	err := c.clickhouseDB.Exec(ctx, query, startTime, endTime)
	if err != nil {
		return fmt.Errorf("generating 1m candles: %w", err)
	}

	c.logger.Info("Generated 1-minute candles",
		zap.Time("start", startTime),
		zap.Time("end", endTime))

	return nil
}

// GenerateFromVWAP generates OHLCV from VWAP prices
func (c *Converter) GenerateFromVWAP(ctx context.Context, startTime, endTime time.Time) error {
	query := `
		INSERT INTO ohlcv_aggregated (
			timestamp, timeframe, base_token_id, quote_token_id,
			open, high, low, close, volume, quote_volume,
			trade_count, vwap_price, exchange_count, version
		)
		SELECT
			toStartOfMinute(timestamp) AS minute_timestamp,
			'1m' AS timeframe,
			base_token_id,
			quote_token_id,
			any(vwap_price) AS open,
			max(vwap_price) AS high,
			min(vwap_price) AS low,
			anyLast(vwap_price) AS close,
			sum(total_volume) AS volume,
			sum(total_volume * vwap_price) AS quote_volume,
			count() AS trade_count,
			avg(vwap_price),  -- Simple average for aggregated VWAP
			toUInt8(avg(exchange_count)) AS exchange_count,
			toUnixTimestamp64Milli(now64()) AS version
		FROM vwap_prices
		WHERE timestamp >= ? AND timestamp < ?
		GROUP BY 
			minute_timestamp,
			base_token_id,
			quote_token_id
	`

	err := c.clickhouseDB.Exec(ctx, query, startTime, endTime)
	if err != nil {
		return fmt.Errorf("generating aggregated candles from VWAP: %w", err)
	}

	return nil
}

// RollupCandles rolls up lower timeframe candles to higher timeframes
func (c *Converter) RollupCandles(ctx context.Context, from, to Timeframe, startTime, endTime time.Time) error {
	fromTable := c.getTableName(from)
	toTable := c.getTableName(to)
	interval := c.getTimeInterval(to)

	query := fmt.Sprintf(`
		INSERT INTO %s (
			timestamp, base_token_id, quote_token_id, exchange_id,
			open, high, low, close, volume, quote_volume,
			trade_count, vwap_price, version
		)
		SELECT
			toStartOf%s(timestamp) AS timestamp,
			base_token_id,
			quote_token_id,
			exchange_id,
			any(open) AS open,  -- First open in the period
			max(high) AS high,
			min(low) AS low,
			anyLast(close) AS close,  -- Last close in the period
			sum(volume) AS volume,
			sum(quote_volume) AS quote_volume,
			sum(trade_count) AS trade_count,
			if(sum(volume) > 0,
				toDecimal64(sum(toDecimal64(vwap_price, 9) * toDecimal64(volume, 9)) / sum(toDecimal64(volume, 9)), 18),
				toDecimal64(avg(vwap_price), 18)) AS vwap_price,
			toUnixTimestamp64Milli(now64()) AS version
		FROM %s
		WHERE timestamp >= ? AND timestamp < ?
		GROUP BY 
			toStartOf%s(timestamp),
			base_token_id,
			quote_token_id,
			exchange_id
	`, toTable, interval, fromTable, interval)

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
	// Round to start of minute
	startTime := baseTime.Truncate(time.Minute)

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
			sum(volume) AS volume,
			sum(quote_volume) AS quote_volume,
			sum(trade_count) AS trade_count,
			if(sum(volume) > 0,
				toDecimal64(sum(toDecimal64(vwap_price, 9) * toDecimal64(volume, 9)) / sum(toDecimal64(volume, 9)), 18),
				toDecimal64(avg(vwap_price), 18)) AS vwap_price,
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

// getTableName returns the table name for a timeframe
func (c *Converter) getTableName(tf Timeframe) string {
	return fmt.Sprintf("ohlcv_%s", tf)
}

// getTimeInterval returns ClickHouse interval function for timeframe
func (c *Converter) getTimeInterval(tf Timeframe) string {
	switch tf {
	case Timeframe5m:
		return "FiveMinute"
	case Timeframe15m:
		return "FifteenMinutes"
	case Timeframe1h:
		return "Hour"
	case Timeframe4h:
		return "Interval(4 HOUR)"
	case Timeframe1d:
		return "Day"
	case Timeframe1w:
		return "Week"
	default:
		return "Minute"
	}
}

// GetCandles retrieves OHLCV data for a specific pair and timeframe
func (c *Converter) GetCandles(ctx context.Context, baseTokenID, quoteTokenID uint32, 
	tf Timeframe, exchangeID string, limit int) ([]Candle, error) {
	
	table := c.getTableName(tf)
	query := fmt.Sprintf(`
		SELECT 
			timestamp, base_token_id, quote_token_id, exchange_id,
			open, high, low, close, volume, quote_volume,
			trade_count, vwap_price, version
		FROM %s FINAL
		WHERE base_token_id = ? AND quote_token_id = ?
			%s
		ORDER BY timestamp DESC
		LIMIT ?
	`, table, c.getExchangeFilter(exchangeID))

	args := []any{baseTokenID, quoteTokenID}
	if exchangeID != "" {
		args = append(args, exchangeID)
	}
	args = append(args, limit)

	rows, err := c.clickhouseDB.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying candles: %w", err)
	}
	defer rows.Close()

	var candles []Candle
	for rows.Next() {
		var c Candle
		err := rows.Scan(
			&c.Timestamp, &c.BaseTokenID, &c.QuoteTokenID, &c.ExchangeID,
			&c.Open, &c.High, &c.Low, &c.Close, &c.Volume, &c.QuoteVolume,
			&c.TradeCount, &c.VWAPPrice, &c.Version,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning candle: %w", err)
		}
		candles = append(candles, c)
	}

	return candles, nil
}

func (c *Converter) getExchangeFilter(exchangeID string) string {
	if exchangeID != "" {
		return "AND exchange_id = ?"
	}
	return ""
}