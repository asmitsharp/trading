package chart_api

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// HistoricalAPI provides access to historical OHLCV data
type HistoricalAPI struct {
	clickhouse clickhouse.Conn
	logger     *zap.Logger
}

// NewHistoricalAPI creates a new historical API service
func NewHistoricalAPI(clickhouse clickhouse.Conn, logger *zap.Logger) *HistoricalAPI {
	return &HistoricalAPI{
		clickhouse: clickhouse,
		logger:     logger,
	}
}

// GetHistoricalCandles retrieves historical candle data for charts
func (h *HistoricalAPI) GetHistoricalCandles(ctx context.Context, request HistoricalCandleRequest) (*HistoricalCandleResponse, error) {
	// Validate timeframe
	if !h.isValidTimeframe(request.Timeframe) {
		return nil, fmt.Errorf("unsupported timeframe: %s", request.Timeframe)
	}

	// Set default limits if not specified
	if request.Limit <= 0 {
		request.Limit = DefaultCandleLimits[request.Timeframe]
	}

	// Apply maximum limits
	if request.Limit > MaxCandleLimits[request.Timeframe] {
		request.Limit = MaxCandleLimits[request.Timeframe]
	}

	// Get table name for timeframe
	tableName := h.getTableName(request.Timeframe)
	if tableName == "" {
		return nil, fmt.Errorf("no table found for timeframe: %s", request.Timeframe)
	}

	// Build query based on whether we're using the new aggregated tables or old per-exchange tables
	var query string
	var queryArgs []interface{}

	if h.isAggregatedTable(request.Timeframe) {
		// Use new aggregated tables (without exchange_id)
		query, queryArgs = h.buildAggregatedQuery(tableName, request)
	} else {
		// Use old per-exchange tables
		query, queryArgs = h.buildPerExchangeQuery(tableName, request)
	}

	h.logger.Debug("Querying historical candles",
		zap.String("table", tableName),
		zap.String("timeframe", request.Timeframe),
		zap.Uint32("base_token_id", request.BaseTokenID),
		zap.Uint32("quote_token_id", request.QuoteTokenID),
		zap.Int("limit", request.Limit))

	// Execute query
	rows, err := h.clickhouse.Query(ctx, query, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to query candles: %w", err)
	}
	defer rows.Close()

	var candles []LiveCandle
	
	for rows.Next() {
		candle, err := h.scanCandle(rows, request.Timeframe)
		if err != nil {
			h.logger.Error("Failed to scan candle", zap.Error(err))
			continue
		}
		candles = append(candles, *candle)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	// Get symbol name for response (optional - could resolve from token IDs)
	symbol := fmt.Sprintf("%d/%d", request.BaseTokenID, request.QuoteTokenID)

	// Calculate time range
	var startTime, endTime int64
	if len(candles) > 0 {
		startTime = candles[0].Timestamp.Unix() * 1000
		endTime = candles[len(candles)-1].Timestamp.Unix() * 1000
	}

	response := &HistoricalCandleResponse{
		Success:    true,
		Data:       candles,
		Symbol:     symbol,
		Timeframe:  request.Timeframe,
		Count:      len(candles),
		StartTime:  startTime,
		EndTime:    endTime,
		Timestamp:  time.Now().Unix() * 1000,
		DataSource: "aggregated_exchanges",
	}

	h.logger.Info("Retrieved historical candles",
		zap.String("symbol", symbol),
		zap.String("timeframe", request.Timeframe),
		zap.Int("count", len(candles)))

	return response, nil
}

// buildAggregatedQuery builds query for new aggregated tables
func (h *HistoricalAPI) buildAggregatedQuery(tableName string, request HistoricalCandleRequest) (string, []interface{}) {
	query := fmt.Sprintf(`
		SELECT 
			timestamp, open, high, low, close, volume, quote_volume,
			vwap_price, exchange_count, contributing_exchanges,
			data_quality_score, trade_count
		FROM %s 
		WHERE base_token_id = ? AND quote_token_id = ?`, tableName)

	args := []interface{}{request.BaseTokenID, request.QuoteTokenID}

	// Add time range if specified
	if request.StartTime > 0 {
		query += " AND timestamp >= ?"
		args = append(args, time.Unix(request.StartTime/1000, 0))
	}
	if request.EndTime > 0 {
		query += " AND timestamp <= ?"
		args = append(args, time.Unix(request.EndTime/1000, 0))
	}

	query += " ORDER BY timestamp DESC LIMIT ?"
	args = append(args, request.Limit)

	return query, args
}

// buildPerExchangeQuery builds query for old per-exchange tables (aggregates on the fly)
func (h *HistoricalAPI) buildPerExchangeQuery(tableName string, request HistoricalCandleRequest) (string, []interface{}) {
	query := fmt.Sprintf(`
		SELECT 
			timestamp,
			argMin(open, timestamp) as open,
			max(high) as high,
			min(low) as low,
			argMax(close, timestamp) as close,
			sum(volume) as volume,
			sum(quote_volume) as quote_volume,
			sum(volume * vwap_price) / sum(volume) as vwap_price,
			count(DISTINCT exchange_id) as exchange_count,
			groupArray(DISTINCT exchange_id) as contributing_exchanges,
			avg(1.0) as data_quality_score,
			sum(trade_count) as trade_count
		FROM %s 
		WHERE base_token_id = ? AND quote_token_id = ?`, tableName)

	args := []interface{}{request.BaseTokenID, request.QuoteTokenID}

	// Add time range if specified
	if request.StartTime > 0 {
		query += " AND timestamp >= ?"
		args = append(args, time.Unix(request.StartTime/1000, 0))
	}
	if request.EndTime > 0 {
		query += " AND timestamp <= ?"
		args = append(args, time.Unix(request.EndTime/1000, 0))
	}

	query += " GROUP BY timestamp ORDER BY timestamp DESC LIMIT ?"
	args = append(args, request.Limit)

	return query, args
}

// scanCandle scans a candle from query results
func (h *HistoricalAPI) scanCandle(rows driver.Rows, timeframe string) (*LiveCandle, error) {
	var candle LiveCandle
	var exchanges []string

	err := rows.Scan(
		&candle.Timestamp,
		&candle.Open,
		&candle.High,
		&candle.Low,
		&candle.Close,
		&candle.Volume,
		&candle.QuoteVolume,
		&candle.VWAPPrice,
		&candle.ExchangeCount,
		&exchanges,
		&candle.DataQualityScore,
		&candle.TradeCount,
	)
	if err != nil {
		return nil, err
	}

	candle.ContributingExchanges = exchanges
	candle.Timeframe = timeframe
	candle.IsComplete = true // Historical candles are always complete

	return &candle, nil
}

// isValidTimeframe checks if timeframe is supported
func (h *HistoricalAPI) isValidTimeframe(timeframe string) bool {
	for _, tf := range SupportedTimeframes {
		if tf == timeframe {
			return true
		}
	}
	return false
}

// getTableName returns the ClickHouse table name for a timeframe
func (h *HistoricalAPI) getTableName(timeframe string) string {
	switch timeframe {
	case "1m":
		return "ohlcv_1m"
	case "5m":
		return "ohlcv_5m"
	case "15m":
		return "ohlcv_15m"
	case "1h":
		return "ohlcv_1h"
	case "4h":
		return "ohlcv_4h"
	case "1d":
		return "ohlcv_1d"
	default:
		return ""
	}
}

// isAggregatedTable checks if the table uses the new aggregated schema
func (h *HistoricalAPI) isAggregatedTable(timeframe string) bool {
	// After migration 000017, all tables will be aggregated
	// For now, assume they use the new schema
	return true
}

// GetLatestPrice gets the latest price for a token pair
func (h *HistoricalAPI) GetLatestPrice(ctx context.Context, baseTokenID, quoteTokenID uint32) (decimal.Decimal, decimal.Decimal, uint8, time.Time, error) {
	query := `
		SELECT 
			vwap_price, total_volume, exchange_count, timestamp
		FROM vwap_prices 
		WHERE base_token_id = ? AND quote_token_id = ?
		ORDER BY timestamp DESC 
		LIMIT 1
	`

	var vwapPrice, volume decimal.Decimal
	var exchangeCount uint8
	var timestamp time.Time

	row := h.clickhouse.QueryRow(ctx, query, baseTokenID, quoteTokenID)
	err := row.Scan(&vwapPrice, &volume, &exchangeCount, &timestamp)
	
	return vwapPrice, volume, exchangeCount, timestamp, err
}

// GetSupportedTimeframes returns all supported timeframes
func (h *HistoricalAPI) GetSupportedTimeframes() []string {
	return SupportedTimeframes
}

// GetTimeframeLimits returns the limits for each timeframe
func (h *HistoricalAPI) GetTimeframeLimits() map[string]interface{} {
	return map[string]interface{}{
		"max_limits":     MaxCandleLimits,
		"default_limits": DefaultCandleLimits,
		"supported":      SupportedTimeframes,
	}
}