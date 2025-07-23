package scheduler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ashmitsharp/trading/internal/db"
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
)

// Scheduler handles scheduled background tasks
type Scheduler struct {
	cron   *cron.Cron
	db     *sql.DB
	logger *zap.Logger
}

// NewScheduler creates a new scheduler instance
func NewScheduler(db *sql.DB, logger *zap.Logger) *Scheduler {
	c := cron.New(cron.WithSeconds())

	return &Scheduler{
		cron:   c,
		db:     db,
		logger: logger,
	}
}

// Start starts the scheduler and registers all cron jobs
func (s *Scheduler) Start() {
	s.logger.Info("Starting scheduler")

	// Register cron jobs
	s.registerJobs()

	// Start the cron scheduler
	s.cron.Start()
}

// Stop stops the scheduler
func (s *Scheduler) Stop() {
	s.logger.Info("Stopping scheduler")
	s.cron.Stop()
}

// registerJobs registers all scheduled jobs
func (s *Scheduler) registerJobs() {
	// Update token metadata every hour
	s.cron.AddFunc("0 0 * * * *", func() {
		if err := s.updateTokenMetadata(); err != nil {
			s.logger.Error("Failed to update token metadata", zap.Error(err))
		}
	})

	// Health check every 5 minutes
	s.cron.AddFunc("0 */5 * * * *", func() {
		if err := s.healthCheck(); err != nil {
			s.logger.Error("Health check failed", zap.Error(err))
		}
	})

	// Log system stats every 15 minutes
	s.cron.AddFunc("0 */15 * * * *", func() {
		s.logSystemStats()
	})

	// Cleanup old data daily at 2 AM
	s.cron.AddFunc("0 0 2 * * *", func() {
		if err := s.cleanupOldData(); err != nil {
			s.logger.Error("Failed to cleanup old data", zap.Error(err))
		}
	})

	s.logger.Info("Registered cron jobs", zap.Int("jobs_count", len(s.cron.Entries())))
}

// updateTokenMetadata updates token metadata from external sources
func (s *Scheduler) updateTokenMetadata() error {
	s.logger.Info("Starting token metadata update")

	// Get all tokens from database
	tokens, err := db.GetAllTokens(s.db)
	if err != nil {
		return fmt.Errorf("failed to get tokens: %w", err)
	}

	updatedCount := 0
	for _, token := range tokens {
		// Simulate fetching market data from external API
		// In a real implementation, this would call CoinGecko, CoinMarketCap, etc.
		marketData, err := s.fetchTokenMarketData(token.Symbol)
		if err != nil {
			s.logger.Warn("Failed to fetch market data",
				zap.String("symbol", token.Symbol),
				zap.Error(err))
			continue
		}

		// Update token in database
		if err := db.UpdateTokenMarketData(s.db, token.Symbol, marketData.MarketCap, marketData.CirculatingSupply); err != nil {
			s.logger.Error("Failed to update token market data",
				zap.String("symbol", token.Symbol),
				zap.Error(err))
			continue
		}

		updatedCount++

		// Add delay to avoid rate limiting
		time.Sleep(100 * time.Millisecond)
	}

	s.logger.Info("Token metadata update completed",
		zap.Int("total_tokens", len(tokens)),
		zap.Int("updated_count", updatedCount))

	return nil
}

// TokenMarketData represents market data from external API
type TokenMarketData struct {
	MarketCap         float64
	CirculatingSupply float64
	Volume24h         float64
	PriceChange24h    float64
}

// fetchTokenMarketData simulates fetching market data from external API
func (s *Scheduler) fetchTokenMarketData(symbol string) (*TokenMarketData, error) {
	// This is a mock implementation
	// In a real application, you would integrate with APIs like:
	// - CoinGecko API
	// - CoinMarketCap API
	// - Binance API

	// Mock data based on symbol
	mockData := map[string]*TokenMarketData{
		"BTCUSDT": {
			MarketCap:         800000000000, // $800B
			CirculatingSupply: 19500000,     // 19.5M BTC
			Volume24h:         30000000000,  // $30B
			PriceChange24h:    2.5,          // +2.5%
		},
		"ETHUSDT": {
			MarketCap:         300000000000, // $300B
			CirculatingSupply: 120000000,    // 120M ETH
			Volume24h:         15000000000,  // $15B
			PriceChange24h:    1.8,          // +1.8%
		},
		// Add more mock data for other tokens
	}

	if data, exists := mockData[symbol]; exists {
		return data, nil
	}

	// Return default data for unknown symbols
	return &TokenMarketData{
		MarketCap:         1000000000, // $1B default
		CirculatingSupply: 1000000,    // 1M default
		Volume24h:         10000000,   // $10M default
		PriceChange24h:    0.0,        // 0% default
	}, nil
}

// healthCheck performs system health checks
func (s *Scheduler) healthCheck() error {
	s.logger.Debug("Performing health check")

	// Check database connectivity
	if err := s.db.Ping(); err != nil {
		return fmt.Errorf("database health check failed: %w", err)
	}

	// Check if we have recent trade data (within last 5 minutes)
	var lastTradeTime sql.NullInt64
	query := `
		SELECT EXTRACT(EPOCH FROM MAX(updated_at))
		FROM tokens
		LIMIT 1
	`

	if err := s.db.QueryRow(query).Scan(&lastTradeTime); err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to check recent activity: %w", err)
	}

	if lastTradeTime.Valid {
		lastUpdate := time.Unix(lastTradeTime.Int64, 0)
		if time.Since(lastUpdate) > 10*time.Minute {
			s.logger.Warn("No recent database activity detected",
				zap.Time("last_update", lastUpdate))
		}
	}

	s.logger.Debug("Health check completed successfully")
	return nil
}

// logSystemStats logs system statistics
func (s *Scheduler) logSystemStats() {
	s.logger.Info("System stats",
		zap.Time("timestamp", time.Now()),
		zap.String("status", "healthy"))

	// Get token count
	tokens, err := db.GetAllTokens(s.db)
	if err == nil {
		s.logger.Info("Token statistics",
			zap.Int("total_tokens", len(tokens)))
	}

	// Log cron job statistics
	entries := s.cron.Entries()
	s.logger.Info("Scheduler statistics",
		zap.Int("total_jobs", len(entries)))
}

// cleanupOldData removes old data to manage storage
func (s *Scheduler) cleanupOldData() error {
	s.logger.Info("Starting old data cleanup")

	// This would typically:
	// 1. Remove old trade data (older than X months)
	// 2. Cleanup old logs
	// 3. Optimize database tables

	// For now, just log the operation
	s.logger.Info("Old data cleanup completed")

	return nil
}

// GetJobStats returns statistics about scheduled jobs
func (s *Scheduler) GetJobStats() map[string]interface{} {
	entries := s.cron.Entries()

	var jobs []map[string]interface{}
	for _, entry := range entries {
		jobs = append(jobs, map[string]interface{}{
			"next_run": entry.Next.Unix(),
			"prev_run": entry.Prev.Unix(),
		})
	}

	return map[string]interface{}{
		"total_jobs":   len(entries),
		"jobs":         jobs,
		"is_running":   len(entries) > 0,
		"last_updated": time.Now().Unix(),
	}
}

// CoinGeckoResponse represents a simplified CoinGecko API response
type CoinGeckoResponse struct {
	ID                string  `json:"id"`
	Symbol            string  `json:"symbol"`
	Name              string  `json:"name"`
	CurrentPrice      float64 `json:"current_price"`
	MarketCap         float64 `json:"market_cap"`
	CirculatingSupply float64 `json:"circulating_supply"`
	TotalVolume       float64 `json:"total_volume"`
	PriceChange24h    float64 `json:"price_change_24h"`
}

// fetchFromCoinGecko demonstrates how to integrate with real API
func (s *Scheduler) fetchFromCoinGecko(coinID string) (*CoinGeckoResponse, error) {
	url := fmt.Sprintf("https://api.coingecko.com/api/v3/simple/price?ids=%s&vs_currencies=usd&include_market_cap=true&include_24hr_vol=true&include_24hr_change=true", coinID)

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch from CoinGecko: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var result map[string]*CoinGeckoResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if data, exists := result[coinID]; exists {
		return data, nil
	}

	return nil, fmt.Errorf("coin not found: %s", coinID)
}
