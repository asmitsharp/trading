package ohlcv

import (
	"context"
	"fmt"
)

// GetCandles retrieves OHLCV data for a specific pair and timeframe
func (c *Converter) GetCandles(ctx context.Context, baseTokenID, quoteTokenID uint32,
	tf Timeframe, exchangeID string, limit int) ([]Candle, error) {

	table := c.getTableName(tf)
	query := fmt.Sprintf(`
		SELECT 
			timestamp, base_token_id, quote_token_id, exchange_id,
			open, high, low, close, volume, quote_volume,
			trade_count, vwap_price, version
		FROM %s
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