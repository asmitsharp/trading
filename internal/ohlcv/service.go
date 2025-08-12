package ohlcv

import (
	"context"
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"go.uber.org/zap"
)

// Service manages OHLCV generation and rollups
type Service struct {
	converter    *Converter
	logger       *zap.Logger
	ticker       *time.Ticker
	done         chan struct{}
	wg           sync.WaitGroup
	lastProcess  map[string]time.Time
	mu           sync.RWMutex
}

// NewService creates a new OHLCV service
func NewService(db clickhouse.Conn, logger *zap.Logger) *Service {
	return &Service{
		converter:   NewConverter(db, logger),
		logger:      logger,
		done:        make(chan struct{}),
		lastProcess: make(map[string]time.Time),
	}
}

// Start begins the OHLCV generation and rollup process
func (s *Service) Start(ctx context.Context) error {
	// Start ticker for 1-minute intervals (aligned to minute boundaries)
	s.ticker = time.NewTicker(1 * time.Minute)
	
	s.wg.Add(1)
	go s.run(ctx)
	
	s.logger.Info("OHLCV service started")
	return nil
}

// Stop gracefully stops the service
func (s *Service) Stop() {
	if s.ticker != nil {
		s.ticker.Stop()
	}
	close(s.done)
	s.wg.Wait()
	s.logger.Info("OHLCV service stopped")
}

// run is the main processing loop
func (s *Service) run(ctx context.Context) {
	defer s.wg.Done()
	
	// Wait until the next minute boundary plus 10 seconds for data to be collected
	now := time.Now()
	nextMinute := now.Truncate(time.Minute).Add(time.Minute)
	waitDuration := nextMinute.Sub(now) + 10*time.Second
	
	s.logger.Info("OHLCV service waiting for next minute boundary",
		zap.Time("current_time", now),
		zap.Time("start_processing_at", nextMinute.Add(10*time.Second)),
		zap.Duration("wait_duration", waitDuration))
	
	select {
	case <-time.After(waitDuration):
		// Process the previous complete minute
		s.processMinute(ctx, time.Now())
	case <-ctx.Done():
		return
	case <-s.done:
		return
	}
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.done:
			return
		case t := <-s.ticker.C:
			s.processMinute(ctx, t)
		}
	}
}

// processMinute handles all OHLCV processing for a given minute
func (s *Service) processMinute(ctx context.Context, currentTime time.Time) {
	// Round to start of minute
	minuteTime := currentTime.Truncate(time.Minute)
	
	// Process previous complete minute
	endTime := minuteTime
	startTime := endTime.Add(-1 * time.Minute)
	
	s.logger.Info("Processing OHLCV for minute",
		zap.Time("minute", startTime),
		zap.String("minute_str", startTime.Format("15:04:05")),
		zap.Time("range_start", startTime),
		zap.Time("range_end", endTime))
	
	// Check if we've already processed this minute (deduplication)
	if s.isAlreadyProcessed("1m", startTime) {
		return
	}
	
	// Generate 1-minute candles from price tickers
	if err := s.converter.GenerateFromPriceTickers(ctx, startTime, endTime); err != nil {
		s.logger.Error("Failed to generate 1m candles from tickers",
			zap.Time("start", startTime),
			zap.Time("end", endTime),
			zap.Error(err))
	} else {
		s.logger.Info("Successfully generated 1m candles",
			zap.Time("minute", startTime),
			zap.String("minute_str", startTime.Format("15:04:05")))
		s.markProcessed("1m", startTime)
	}
	
	// Generate aggregated candles from VWAP
	if err := s.converter.GenerateFromVWAP(ctx, startTime, endTime); err != nil {
		s.logger.Error("Failed to generate aggregated candles from VWAP",
			zap.Time("start", startTime),
			zap.Time("end", endTime),
			zap.Error(err))
	}
	
	// Perform hierarchical rollups
	if err := s.converter.HierarchicalRollup(ctx, minuteTime); err != nil {
		s.logger.Error("Failed to perform hierarchical rollup",
			zap.Time("time", minuteTime),
			zap.Error(err))
	}
	
	// Aggregate exchanges for all timeframes every 5 minutes
	if minuteTime.Minute()%5 == 0 {
		s.aggregateAllTimeframes(ctx, minuteTime)
	}
	
	// Optimize tables every hour
	if minuteTime.Minute() == 0 {
		if err := s.converter.OptimizeTables(ctx); err != nil {
			s.logger.Error("Failed to optimize tables", zap.Error(err))
		}
	}
}

// aggregateAllTimeframes aggregates exchange data for all timeframes
func (s *Service) aggregateAllTimeframes(ctx context.Context, baseTime time.Time) {
	timeframes := []struct {
		tf       Timeframe
		duration time.Duration
	}{
		{Timeframe1m, 5 * time.Minute},
		{Timeframe5m, 15 * time.Minute},
		{Timeframe15m, 1 * time.Hour},
		{Timeframe1h, 4 * time.Hour},
		{Timeframe4h, 24 * time.Hour},
		{Timeframe1d, 7 * 24 * time.Hour},
	}
	
	for _, tf := range timeframes {
		endTime := baseTime
		startTime := endTime.Add(-tf.duration)
		
		if err := s.converter.AggregateExchanges(ctx, tf.tf, startTime, endTime); err != nil {
			s.logger.Error("Failed to aggregate exchanges",
				zap.String("timeframe", string(tf.tf)),
				zap.Time("start", startTime),
				zap.Time("end", endTime),
				zap.Error(err))
		}
	}
}

// isAlreadyProcessed checks if a timeframe has been processed
func (s *Service) isAlreadyProcessed(timeframe string, timestamp time.Time) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	key := s.getProcessKey(timeframe, timestamp)
	lastTime, exists := s.lastProcess[key]
	if !exists {
		return false
	}
	
	// Consider processed if within last 55 seconds (allowing for some delay)
	return time.Since(lastTime) < 55*time.Second
}

// markProcessed marks a timeframe as processed
func (s *Service) markProcessed(timeframe string, timestamp time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	key := s.getProcessKey(timeframe, timestamp)
	s.lastProcess[key] = time.Now()
	
	// Clean up old entries (older than 2 hours)
	cutoff := time.Now().Add(-2 * time.Hour)
	for k, v := range s.lastProcess {
		if v.Before(cutoff) {
			delete(s.lastProcess, k)
		}
	}
}

// getProcessKey generates a unique key for deduplication
func (s *Service) getProcessKey(timeframe string, timestamp time.Time) string {
	return timeframe + "_" + timestamp.Format("2006-01-02T15:04:00")
}

// GetLatestCandles returns the most recent candles for a pair
func (s *Service) GetLatestCandles(ctx context.Context, baseTokenID, quoteTokenID uint32,
	timeframe Timeframe, exchangeID string, limit int) ([]Candle, error) {
	
	return s.converter.GetCandles(ctx, baseTokenID, quoteTokenID, timeframe, exchangeID, limit)
}

// BackfillHistoricalData generates OHLCV data for a historical period
func (s *Service) BackfillHistoricalData(ctx context.Context, startDate, endDate time.Time) error {
	s.logger.Info("Starting historical backfill",
		zap.Time("start", startDate),
		zap.Time("end", endDate))
	
	// Process in 1-hour chunks to avoid memory issues
	current := startDate
	for current.Before(endDate) {
		chunkEnd := current.Add(1 * time.Hour)
		if chunkEnd.After(endDate) {
			chunkEnd = endDate
		}
		
		// Generate 1-minute candles
		if err := s.converter.GenerateFromPriceTickers(ctx, current, chunkEnd); err != nil {
			s.logger.Error("Failed to backfill 1m candles",
				zap.Time("start", current),
				zap.Time("end", chunkEnd),
				zap.Error(err))
			// Continue with next chunk despite error
		}
		
		// Generate aggregated candles from VWAP
		if err := s.converter.GenerateFromVWAP(ctx, current, chunkEnd); err != nil {
			s.logger.Error("Failed to backfill VWAP aggregated candles",
				zap.Time("start", current),
				zap.Time("end", chunkEnd),
				zap.Error(err))
		}
		
		current = chunkEnd
	}
	
	// Perform rollups for the entire period
	s.logger.Info("Performing rollups for backfilled data")
	
	// Roll up to 5m
	if err := s.converter.RollupCandles(ctx, Timeframe1m, Timeframe5m, startDate, endDate); err != nil {
		s.logger.Error("Failed to rollup 1m->5m", zap.Error(err))
	}
	
	// Roll up to 15m
	if err := s.converter.RollupCandles(ctx, Timeframe5m, Timeframe15m, startDate, endDate); err != nil {
		s.logger.Error("Failed to rollup 5m->15m", zap.Error(err))
	}
	
	// Roll up to 1h
	if err := s.converter.RollupCandles(ctx, Timeframe15m, Timeframe1h, startDate, endDate); err != nil {
		s.logger.Error("Failed to rollup 15m->1h", zap.Error(err))
	}
	
	// Roll up to 4h
	if err := s.converter.RollupCandles(ctx, Timeframe1h, Timeframe4h, startDate, endDate); err != nil {
		s.logger.Error("Failed to rollup 1h->4h", zap.Error(err))
	}
	
	// Roll up to 1d
	if err := s.converter.RollupCandles(ctx, Timeframe4h, Timeframe1d, startDate, endDate); err != nil {
		s.logger.Error("Failed to rollup 4h->1d", zap.Error(err))
	}
	
	// Roll up to 1w
	if err := s.converter.RollupCandles(ctx, Timeframe1d, Timeframe1w, startDate, endDate); err != nil {
		s.logger.Error("Failed to rollup 1d->1w", zap.Error(err))
	}
	
	// Aggregate exchanges for all timeframes
	timeframes := []Timeframe{Timeframe1m, Timeframe5m, Timeframe15m, Timeframe1h, Timeframe4h, Timeframe1d, Timeframe1w}
	for _, tf := range timeframes {
		if err := s.converter.AggregateExchanges(ctx, tf, startDate, endDate); err != nil {
			s.logger.Error("Failed to aggregate exchanges",
				zap.String("timeframe", string(tf)),
				zap.Error(err))
		}
	}
	
	// Optimize tables
	if err := s.converter.OptimizeTables(ctx); err != nil {
		s.logger.Error("Failed to optimize tables after backfill", zap.Error(err))
	}
	
	s.logger.Info("Historical backfill completed",
		zap.Time("start", startDate),
		zap.Time("end", endDate))
	
	return nil
}