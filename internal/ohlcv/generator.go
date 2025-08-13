package ohlcv

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
)

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
			argMin(price, timestamp) AS open,  -- First price chronologically
			max(price) AS high,
			min(price) AS low,
			argMax(price, timestamp) AS close,  -- Last price chronologically
			toDecimal128(sum(toFloat64(volume_24h)), 18) AS volume,  -- Sum 24h volumes (CoinMarketCap approach)
			toDecimal128(sum(toFloat64(volume_24h) * toFloat64(price)), 18) AS quote_volume,
			count() AS trade_count,
			if(sum(toFloat64(volume_24h)) > 0, 
				toDecimal128(sum(toFloat64(price) * toFloat64(volume_24h)) / sum(toFloat64(volume_24h)), 18), 
				toDecimal128(avg(toFloat64(price)), 18)) AS vwap_price,
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
			argMin(vwap_price, timestamp) AS open,
			max(vwap_price) AS high,
			min(vwap_price) AS low,
			argMax(vwap_price, timestamp) AS close,
			toDecimal128(sum(toFloat64(total_volume)), 18) AS volume,
			toDecimal128(sum(toFloat64(total_volume) * toFloat64(vwap_price)), 18) AS quote_volume,
			count() AS trade_count,
			toDecimal128(sum(toFloat64(vwap_price) * toFloat64(total_volume)) / sum(toFloat64(total_volume)), 18) AS vwap_price,
			max(exchange_count) AS exchange_count,  -- Use max to get total unique exchanges
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