package ohlcv

import (
	"context"
	"fmt"
	"time"

	"github.com/ashmitsharp/trading/internal/volume_calculator"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// GenerateFromPriceTickers generates 1-minute OHLCV from price tickers with proper volume calculation
func (c *Converter) GenerateFromPriceTickers(ctx context.Context, startTime, endTime time.Time) error {
	// First, generate candles with OHLC data but placeholder volume
	if err := c.generateOHLCFromTickers(ctx, startTime, endTime); err != nil {
		return fmt.Errorf("generating OHLC data: %w", err)
	}

	// Then, calculate proper 1-minute volumes using the volume calculator
	if err := c.calculateAndUpdateVolumes(ctx, startTime, endTime); err != nil {
		return fmt.Errorf("calculating proper volumes: %w", err)
	}

	c.logger.Info("Generated 1-minute candles with proper volume calculation",
		zap.Time("start", startTime),
		zap.Time("end", endTime))

	return nil
}

// generateOHLCFromTickers generates OHLC data without proper volume calculation
func (c *Converter) generateOHLCFromTickers(ctx context.Context, startTime, endTime time.Time) error {
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
			toDecimal128(0, 18) AS volume,  -- Placeholder - will be calculated properly
			toDecimal128(0, 18) AS quote_volume,  -- Placeholder
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

	return c.clickhouseDB.Exec(ctx, query, startTime, endTime)
}

// calculateAndUpdateVolumes implements the volume calculation formula:
// 1min_volume = current_24h_volume - previous_24h_volume + volume_from_1440_minutes_ago
func (c *Converter) calculateAndUpdateVolumes(ctx context.Context, startTime, endTime time.Time) error {
	// Get price ticker data for volume calculation
	tickerQuery := `
		SELECT 
			toStartOfMinute(timestamp) AS minute_timestamp,
			base_token_id, quote_token_id, exchange_id,
			avg(toFloat64(volume_24h)) AS current_volume_24h,
			any(price) AS price
		FROM price_tickers
		WHERE timestamp >= ? AND timestamp < ?
			AND base_token_id > 0 AND quote_token_id > 0
		GROUP BY minute_timestamp, base_token_id, quote_token_id, exchange_id
		ORDER BY minute_timestamp
	`

	rows, err := c.clickhouseDB.Query(ctx, tickerQuery, startTime, endTime)
	if err != nil {
		return fmt.Errorf("querying ticker data: %w", err)
	}
	defer rows.Close()

	// Process each ticker record
	for rows.Next() {
		var (
			timestamp      time.Time
			baseTokenID    uint32
			quoteTokenID   uint32
			exchangeID     string
			current24hVol  float64
			price          float64
		)

		if err := rows.Scan(&timestamp, &baseTokenID, &quoteTokenID, &exchangeID, &current24hVol, &price); err != nil {
			c.logger.Error("Failed to scan ticker row", zap.Error(err))
			continue
		}

		// Calculate 1-minute volume using the volume calculator
		calculatedVolume, err := c.calculate1MinuteVolume(ctx, 
			baseTokenID, quoteTokenID, exchangeID,
			decimal.NewFromFloat(current24hVol), timestamp)
		if err != nil {
			c.logger.Debug("Failed to calculate 1-minute volume",
				zap.Uint32("base_token_id", baseTokenID),
				zap.Uint32("quote_token_id", quoteTokenID),
				zap.String("exchange_id", exchangeID),
				zap.Time("timestamp", timestamp),
				zap.Error(err))
			continue
		}

		// Update the OHLCV record with the calculated volume
		quoteVolume := calculatedVolume.Mul(decimal.NewFromFloat(price))
		
		updateQuery := `
			ALTER TABLE ohlcv_1m 
			UPDATE 
				volume = ?,
				quote_volume = ?
			WHERE timestamp = ? 
				AND base_token_id = ? 
				AND quote_token_id = ? 
				AND exchange_id = ?
		`

		if err := c.clickhouseDB.Exec(ctx, updateQuery, 
			calculatedVolume, quoteVolume, timestamp, baseTokenID, quoteTokenID, exchangeID); err != nil {
			c.logger.Error("Failed to update volume",
				zap.Uint32("base_token_id", baseTokenID),
				zap.Uint32("quote_token_id", quoteTokenID),
				zap.String("exchange_id", exchangeID),
				zap.Error(err))
		}
	}

	return rows.Err()
}

// calculate1MinuteVolume implements the formula: 
// 1min_volume = current_24h - previous_24h + buffer[-1440].volume
func (c *Converter) calculate1MinuteVolume(ctx context.Context, 
	baseTokenID, quoteTokenID uint32, exchangeID string,
	current24hVolume decimal.Decimal, timestamp time.Time) (decimal.Decimal, error) {

	// Get previous 24h volume (1 minute ago)
	previous24hVolume, err := c.getPrevious24hVolume(ctx, baseTokenID, quoteTokenID, exchangeID, timestamp)
	if err != nil {
		return decimal.Zero, fmt.Errorf("getting previous 24h volume: %w", err)
	}

	// Create volume snapshots for the calculator
	currentSnapshot := volume_calculator.VolumeSnapshot{
		Timestamp:    timestamp,
		Symbol:       fmt.Sprintf("%d_%d", baseTokenID, quoteTokenID),
		Volume24h:    current24hVolume,
		ExchangeID:   exchangeID,
		BaseTokenID:  baseTokenID,
		QuoteTokenID: quoteTokenID,
	}

	previousSnapshot := volume_calculator.VolumeSnapshot{
		Timestamp:    timestamp.Add(-1 * time.Minute),
		Symbol:       fmt.Sprintf("%d_%d", baseTokenID, quoteTokenID),
		Volume24h:    previous24hVolume,
		ExchangeID:   exchangeID,
		BaseTokenID:  baseTokenID,
		QuoteTokenID: quoteTokenID,
	}

	// Calculate 1-minute volume using the volume calculator
	result := c.volumeCalculator.Calculate1MinuteVolume(currentSnapshot, previousSnapshot, timestamp)

	if !result.IsValid {
		return decimal.Zero, fmt.Errorf("volume calculation failed: %s", result.ErrorMessage)
	}

	// Store the calculated candle in the buffer for future calculations
	candle := volume_calculator.OHLCVCandle{
		Timestamp:    timestamp,
		Symbol:       fmt.Sprintf("%d_%d", baseTokenID, quoteTokenID),
		ExchangeID:   exchangeID,
		BaseTokenID:  baseTokenID,
		QuoteTokenID: quoteTokenID,
		Volume:       result.CalculatedVolume,
	}
	c.volumeCalculator.AddCalculatedCandle(candle)

	c.logger.Debug("Calculated 1-minute volume",
		zap.Uint32("base_token_id", baseTokenID),
		zap.Uint32("quote_token_id", quoteTokenID),
		zap.String("exchange_id", exchangeID),
		zap.Time("timestamp", timestamp),
		zap.String("method", result.Method),
		zap.String("calculated_volume", result.CalculatedVolume.String()))

	return result.CalculatedVolume, nil
}

// getPrevious24hVolume gets the 24h volume from 1 minute ago
func (c *Converter) getPrevious24hVolume(ctx context.Context, 
	baseTokenID, quoteTokenID uint32, exchangeID string, timestamp time.Time) (decimal.Decimal, error) {

	previousTime := timestamp.Add(-1 * time.Minute)
	
	query := `
		SELECT avg(toFloat64(volume_24h)) as volume_24h
		FROM price_tickers
		WHERE base_token_id = ? AND quote_token_id = ? AND exchange_id = ?
			AND timestamp >= ? AND timestamp < ?
	`

	var volume24h float64
	err := c.clickhouseDB.QueryRow(ctx, query, 
		baseTokenID, quoteTokenID, exchangeID,
		previousTime, previousTime.Add(1*time.Minute)).Scan(&volume24h)
	
	if err != nil {
		// If no previous data, return zero (first-time calculation)
		return decimal.Zero, nil
	}

	return decimal.NewFromFloat(volume24h), nil
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