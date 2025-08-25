package chart_api

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// LiveDataService streams live price and candle data from vwap_prices table
type LiveDataService struct {
	clickhouse    clickhouse.Conn
	wsServer      *WebSocketServer
	logger        *zap.Logger
	
	// State management
	lastPriceUpdate map[string]time.Time // key: "base_token_id:quote_token_id"
	lastCandleUpdate map[string]time.Time // key: "timeframe:base_token_id:quote_token_id"
	mutex           sync.RWMutex
	
	// Control
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewLiveDataService creates a new live data service
func NewLiveDataService(
	clickhouse clickhouse.Conn,
	wsServer *WebSocketServer,
	logger *zap.Logger,
) *LiveDataService {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &LiveDataService{
		clickhouse:       clickhouse,
		wsServer:         wsServer,
		logger:           logger,
		lastPriceUpdate:  make(map[string]time.Time),
		lastCandleUpdate: make(map[string]time.Time),
		ctx:              ctx,
		cancel:           cancel,
	}
}

// Start begins streaming live data
func (lds *LiveDataService) Start() error {
	lds.logger.Info("Starting live data service")
	
	// Start live price streaming (every 5 seconds)
	lds.wg.Add(1)
	go lds.streamLivePrices()
	
	// Start live candle streaming (every minute)
	lds.wg.Add(1)
	go lds.streamLiveCandles()
	
	return nil
}

// Stop stops the live data service
func (lds *LiveDataService) Stop() {
	lds.logger.Info("Stopping live data service")
	lds.cancel()
	lds.wg.Wait()
	lds.logger.Info("Live data service stopped")
}

// streamLivePrices streams live price updates from vwap_prices table
func (lds *LiveDataService) streamLivePrices() {
	defer lds.wg.Done()
	
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-lds.ctx.Done():
			return
		case <-ticker.C:
			if err := lds.fetchAndBroadcastPrices(); err != nil {
				lds.logger.Error("Failed to fetch and broadcast prices", zap.Error(err))
			}
		}
	}
}

// streamLiveCandles streams live candle updates from OHLCV tables
func (lds *LiveDataService) streamLiveCandles() {
	defer lds.wg.Done()
	
	// Align to minute boundary
	now := time.Now()
	nextMinute := now.Truncate(time.Minute).Add(time.Minute)
	waitDuration := nextMinute.Sub(now)
	
	lds.logger.Info("Waiting for candle stream alignment", zap.Duration("wait", waitDuration))
	time.Sleep(waitDuration)
	
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	
	for {
		select {
		case <-lds.ctx.Done():
			return
		case <-ticker.C:
			if err := lds.fetchAndBroadcastCandles(); err != nil {
				lds.logger.Error("Failed to fetch and broadcast candles", zap.Error(err))
			}
		}
	}
}

// fetchAndBroadcastPrices fetches latest prices and broadcasts to WebSocket clients
func (lds *LiveDataService) fetchAndBroadcastPrices() error {
	// Query latest VWAP prices
	query := `
		SELECT 
			base_token_id, quote_token_id, vwap_price, total_volume,
			exchange_count, timestamp
		FROM vwap_prices 
		WHERE timestamp >= now() - INTERVAL 1 MINUTE
		ORDER BY timestamp DESC
		LIMIT 1000
	`

	rows, err := lds.clickhouse.Query(lds.ctx, query)
	if err != nil {
		return fmt.Errorf("failed to query vwap prices: %w", err)
	}
	defer rows.Close()

	priceUpdates := make(map[string]LivePrice) // key: "base_token_id:quote_token_id"

	for rows.Next() {
		var (
			baseTokenID   uint32
			quoteTokenID  uint32
			vwapPrice     decimal.Decimal
			volume        decimal.Decimal
			exchangeCount uint8
			timestamp     time.Time
		)

		if err := rows.Scan(&baseTokenID, &quoteTokenID, &vwapPrice, &volume, &exchangeCount, &timestamp); err != nil {
			lds.logger.Error("Failed to scan price row", zap.Error(err))
			continue
		}

		key := fmt.Sprintf("%d:%d", baseTokenID, quoteTokenID)
		
		// Only update if this is newer than our last update
		lds.mutex.RLock()
		lastUpdate, exists := lds.lastPriceUpdate[key]
		lds.mutex.RUnlock()

		if exists && !timestamp.After(lastUpdate) {
			continue
		}

		// Calculate 24h price change (placeholder - you'd implement actual calculation)
		priceChange24h := decimal.Zero
		priceChangePercent := decimal.Zero
		high24h := vwapPrice
		low24h := vwapPrice

		livePrice := LivePrice{
			BaseTokenID:        baseTokenID,
			QuoteTokenID:       quoteTokenID,
			Symbol:             fmt.Sprintf("%d/%d", baseTokenID, quoteTokenID),
			Price:              vwapPrice,
			VWAPPrice:          vwapPrice,
			Volume24h:          volume,
			PriceChange24h:     priceChange24h,
			PriceChangePercent: priceChangePercent,
			High24h:            high24h,
			Low24h:             low24h,
			ExchangeCount:      exchangeCount,
			Timestamp:          timestamp,
			DataQualityScore:   decimal.NewFromFloat(0.95), // Placeholder
		}

		priceUpdates[key] = livePrice

		// Update last update time
		lds.mutex.Lock()
		lds.lastPriceUpdate[key] = timestamp
		lds.mutex.Unlock()
	}

	// Broadcast all price updates
	for _, price := range priceUpdates {
		lds.wsServer.BroadcastPriceUpdate(price)
	}

	lds.logger.Debug("Broadcasted price updates", zap.Int("count", len(priceUpdates)))
	return nil
}

// fetchAndBroadcastCandles fetches latest candles and broadcasts to WebSocket clients
func (lds *LiveDataService) fetchAndBroadcastCandles() error {
	// Fetch latest candles for all timeframes
	for _, timeframe := range SupportedTimeframes {
		if err := lds.fetchCandlesForTimeframe(timeframe); err != nil {
			lds.logger.Error("Failed to fetch candles for timeframe",
				zap.String("timeframe", timeframe), zap.Error(err))
		}
	}
	return nil
}

// fetchCandlesForTimeframe fetches and broadcasts candles for a specific timeframe
func (lds *LiveDataService) fetchCandlesForTimeframe(timeframe string) error {
	tableName := lds.getTableName(timeframe)
	if tableName == "" {
		return fmt.Errorf("no table for timeframe: %s", timeframe)
	}

	// Get candles from the last few minutes to catch any updates
	query := fmt.Sprintf(`
		SELECT 
			timestamp, base_token_id, quote_token_id,
			open, high, low, close, volume, quote_volume,
			vwap_price, exchange_count, contributing_exchanges,
			data_quality_score, trade_count
		FROM %s 
		WHERE timestamp >= now() - INTERVAL 10 MINUTE
		ORDER BY timestamp DESC
	`, tableName)

	rows, err := lds.clickhouse.Query(lds.ctx, query)
	if err != nil {
		return fmt.Errorf("failed to query %s candles: %w", timeframe, err)
	}
	defer rows.Close()

	for rows.Next() {
		var candle LiveCandle
		var exchanges []string

		err := rows.Scan(
			&candle.Timestamp,
			&candle.BaseTokenID,
			&candle.QuoteTokenID,
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
			lds.logger.Error("Failed to scan candle", zap.Error(err))
			continue
		}

		candle.ContributingExchanges = exchanges
		candle.Timeframe = timeframe
		candle.Symbol = fmt.Sprintf("%d/%d", candle.BaseTokenID, candle.QuoteTokenID)
		
		// Check if this is a new update
		key := fmt.Sprintf("%s:%d:%d", timeframe, candle.BaseTokenID, candle.QuoteTokenID)
		
		lds.mutex.RLock()
		lastUpdate, exists := lds.lastCandleUpdate[key]
		lds.mutex.RUnlock()

		if exists && !candle.Timestamp.After(lastUpdate) {
			continue
		}

		// Determine if candle is complete (older than current timeframe period)
		duration := TimeframeDurations[timeframe]
		candle.IsComplete = time.Since(candle.Timestamp) > duration

		// Broadcast candle update
		lds.wsServer.BroadcastCandleUpdate(candle)

		// Update last update time
		lds.mutex.Lock()
		lds.lastCandleUpdate[key] = candle.Timestamp
		lds.mutex.Unlock()
	}

	return nil
}

// getTableName returns the table name for a timeframe
func (lds *LiveDataService) getTableName(timeframe string) string {
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