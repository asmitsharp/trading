package volume_calculator

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// Service manages volume calculation for multiple exchanges and symbols
type Service struct {
	calculator   *VolumeCalculator
	clickhouse   clickhouse.Conn
	logger       *zap.Logger
	
	// State management
	volumeSnapshots map[string]VolumeSnapshot // key: "exchange:symbol", latest 24h volume
	mutex          sync.RWMutex
	
	// Configuration
	topExchanges []string
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
}

// NewService creates a new volume calculation service
func NewService(
	config CalculationConfig,
	clickhouse clickhouse.Conn,
	logger *zap.Logger,
) *Service {
	ctx, cancel := context.WithCancel(context.Background())
	
	// Top 5 exchanges for live OHLCV
	topExchanges := []string{
		"binance",
		"coinbase",
		"kraken", 
		"bybit",
		"mexc",
	}
	
	return &Service{
		calculator:      NewVolumeCalculator(config, logger),
		clickhouse:      clickhouse,
		logger:          logger,
		volumeSnapshots: make(map[string]VolumeSnapshot),
		topExchanges:    topExchanges,
		ctx:             ctx,
		cancel:          cancel,
	}
}

// Start begins the volume calculation service
func (s *Service) Start() error {
	s.logger.Info("Starting volume calculation service")
	
	// Load existing OHLCV data into buffers
	if err := s.loadHistoricalCandles(); err != nil {
		s.logger.Warn("Failed to load historical candles", zap.Error(err))
	}
	
	s.logger.Info("Volume calculation service started successfully")
	return nil
}

// Stop stops the volume calculation service
func (s *Service) Stop() {
	s.logger.Info("Stopping volume calculation service")
	s.cancel()
	s.wg.Wait()
	s.logger.Info("Volume calculation service stopped")
}

// Calculate1MinuteVolumeForExchange calculates 1m volume for a specific exchange/symbol
func (s *Service) Calculate1MinuteVolumeForExchange(
	exchangeID, symbol string,
	baseTokenID, quoteTokenID uint32,
	current24hVolume decimal.Decimal,
	timestamp time.Time,
) (VolumeCalculationResult, error) {
	
	key := fmt.Sprintf("%s:%s", exchangeID, symbol)
	
	// Create current snapshot
	currentSnapshot := VolumeSnapshot{
		Timestamp:    timestamp,
		Symbol:       symbol,
		Volume24h:    current24hVolume,
		ExchangeID:   exchangeID,
		BaseTokenID:  baseTokenID,
		QuoteTokenID: quoteTokenID,
	}
	
	// Get previous snapshot (1 minute ago)
	s.mutex.RLock()
	previousSnapshot, exists := s.volumeSnapshots[key]
	s.mutex.RUnlock()
	
	if !exists {
		// First time seeing this symbol/exchange - store and return zero volume
		s.mutex.Lock()
		s.volumeSnapshots[key] = currentSnapshot
		s.mutex.Unlock()
		
		return VolumeCalculationResult{
			Timestamp:        timestamp,
			Symbol:           symbol,
			ExchangeID:       exchangeID,
			BaseTokenID:      baseTokenID,
			QuoteTokenID:     quoteTokenID,
			CalculatedVolume: decimal.Zero,
			Method:           "first_snapshot",
			IsValid:          false,
			ErrorMessage:     "first snapshot - no previous data",
		}, nil
	}
	
	// Calculate 1-minute volume
	result := s.calculator.Calculate1MinuteVolume(currentSnapshot, previousSnapshot, timestamp)
	
	// Update stored snapshot
	s.mutex.Lock()
	s.volumeSnapshots[key] = currentSnapshot
	s.mutex.Unlock()
	
	// If calculation was successful, add to buffer
	if result.IsValid {
		candle := OHLCVCandle{
			Timestamp:    timestamp,
			Symbol:       symbol,
			ExchangeID:   exchangeID,
			BaseTokenID:  baseTokenID,
			QuoteTokenID: quoteTokenID,
			Volume:       result.CalculatedVolume,
			// OHLC values would be populated separately
		}
		s.calculator.AddCalculatedCandle(candle)
	}
	
	return result, nil
}

// ProcessLiveTickerData processes live ticker data from exchanges
func (s *Service) ProcessLiveTickerData(
	exchangeID, symbol string,
	baseTokenID, quoteTokenID uint32,
	volume24h decimal.Decimal,
	timestamp time.Time,
) (decimal.Decimal, error) {
	
	result, err := s.Calculate1MinuteVolumeForExchange(
		exchangeID, symbol, baseTokenID, quoteTokenID, volume24h, timestamp)
	
	if err != nil {
		return decimal.Zero, err
	}
	
	if !result.IsValid {
		return decimal.Zero, fmt.Errorf("volume calculation failed: %s", result.ErrorMessage)
	}
	
	s.logger.Debug("Calculated 1m volume from ticker",
		zap.String("exchange", exchangeID),
		zap.String("symbol", symbol),
		zap.String("method", result.Method),
		zap.String("volume", result.CalculatedVolume.String()))
	
	return result.CalculatedVolume, nil
}

// Note: Volume snapshot collection methods removed since they're not needed
// for the current integration. The volume calculator will be called directly
// from the OHLCV generator.

// loadHistoricalCandles loads existing OHLCV data into buffers
func (s *Service) loadHistoricalCandles() error {
	s.logger.Info("Loading historical candles into volume calculator buffers")
	
	// Load last 1440 candles for each exchange/symbol pair
	cutoffTime := time.Now().Add(-1440 * time.Minute)
	
	query := `
		SELECT 
			timestamp, base_token_id, quote_token_id, exchange_id,
			open, high, low, close, volume, quote_volume, trade_count
		FROM ohlcv_1m 
		WHERE timestamp >= ? 
		ORDER BY base_token_id, quote_token_id, exchange_id, timestamp`
	
	rows, err := s.clickhouse.Query(s.ctx, query, cutoffTime)
	if err != nil {
		return fmt.Errorf("failed to query historical candles: %w", err)
	}
	defer rows.Close()
	
	loadCount := 0
	for rows.Next() {
		var candle OHLCVCandle
		
		err := rows.Scan(
			&candle.Timestamp,
			&candle.BaseTokenID,
			&candle.QuoteTokenID,
			&candle.ExchangeID,
			&candle.Open,
			&candle.High,
			&candle.Low,
			&candle.Close,
			&candle.Volume,
			&candle.QuoteVolume,
			&candle.TradeCount,
		)
		if err != nil {
			s.logger.Error("Failed to scan historical candle", zap.Error(err))
			continue
		}
		
		// Generate symbol from token IDs for the buffer key
		candle.Symbol = fmt.Sprintf("%d_%d", candle.BaseTokenID, candle.QuoteTokenID)
		
		// Add to calculator buffers
		s.calculator.AddCalculatedCandle(candle)
		loadCount++
	}
	
	s.logger.Info("Loaded historical candles", 
		zap.Int("count", loadCount),
		zap.Time("cutoff", cutoffTime))
	
	return rows.Err()
}

// TradingPair represents a trading pair (reusing from live_ohlcv)
type TradingPair struct {
	BaseTokenID  uint32
	QuoteTokenID uint32
	BaseSymbol   string
	QuoteSymbol  string
}

// GetVolumeCalculator returns the volume calculator instance for use by OHLCV generator
func (s *Service) GetVolumeCalculator() *VolumeCalculator {
	return s.calculator
}

// GetCalculatorStats returns statistics about the volume calculator
func (s *Service) GetCalculatorStats() map[string]interface{} {
	s.mutex.RLock()
	snapshotCount := len(s.volumeSnapshots)
	s.mutex.RUnlock()
	
	return map[string]interface{}{
		"volume_snapshots": snapshotCount,
		"buffer_stats":     s.calculator.GetBufferStats(),
	}
}