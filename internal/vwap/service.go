package vwap

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/ashmitsharp/trading/internal/calculator"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// Service handles VWAP calculations and storage
type Service struct {
	clickhouseConn driver.Conn
	calculator     *calculator.VWAPCalculator
	logger         *zap.Logger
	
	mu sync.RWMutex
}

// NewService creates a new VWAP service
func NewService(clickhouseConn driver.Conn, logger *zap.Logger) *Service {
	return &Service{
		clickhouseConn: clickhouseConn,
		calculator:     calculator.NewVWAPCalculator(logger),
		logger:         logger,
	}
}

// CalculateAndStore calculates VWAP for all token pairs and stores in ClickHouse
func (s *Service) CalculateAndStore(ctx context.Context) error {
	// Fetch recent prices from ClickHouse
	priceData, err := s.fetchRecentPrices(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch recent prices: %w", err)
	}
	
	// Group prices by token pair
	pricesByPair := s.groupPricesByPair(priceData)
	
	// Calculate VWAP for each pair
	vwapResults := s.calculator.CalculateBatch(pricesByPair)
	
	// Store VWAP results
	if err := s.storeVWAPResults(ctx, vwapResults); err != nil {
		return fmt.Errorf("failed to store VWAP results: %w", err)
	}
	
	s.logger.Info("VWAP calculation completed",
		zap.Int("pairs", len(vwapResults)))
	
	return nil
}

func (s *Service) fetchRecentPrices(ctx context.Context) ([]calculator.PriceData, error) {
	query := `
		SELECT 
			exchange_id,
			base_token_id,
			quote_token_id,
			argMax(price, timestamp) as latest_price,
			sum(volume_24h) as total_volume,
			max(timestamp) as latest_timestamp
		FROM price_tickers
		WHERE timestamp >= now() - INTERVAL 1 MINUTE
			AND base_token_id > 0
			AND quote_token_id > 0
			AND price > 0
			AND volume_24h > 0
		GROUP BY exchange_id, base_token_id, quote_token_id
		HAVING total_volume > 1000  -- Minimum volume threshold
	`
	
	rows, err := s.clickhouseConn.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query prices: %w", err)
	}
	defer rows.Close()
	
	var prices []calculator.PriceData
	exchangeWeights := getExchangeWeights()
	
	for rows.Next() {
		var (
			exchangeID   string
			baseTokenID  uint32
			quoteTokenID uint32
			price        decimal.Decimal
			volume       decimal.Decimal
			timestamp    time.Time
		)
		
		if err := rows.Scan(&exchangeID, &baseTokenID, &quoteTokenID, &price, &volume, &timestamp); err != nil {
			s.logger.Error("Failed to scan price row", zap.Error(err))
			continue
		}
		
		weight := exchangeWeights[exchangeID]
		if weight.IsZero() {
			weight = decimal.NewFromFloat(0.01) // Default weight
		}
		
		prices = append(prices, calculator.PriceData{
			ExchangeID:   exchangeID,
			Symbol:       fmt.Sprintf("%d-%d", baseTokenID, quoteTokenID),
			BaseTokenID:  int(baseTokenID),
			QuoteTokenID: int(quoteTokenID),
			Price:        price,
			Volume:       volume,
			Weight:       weight,
			Timestamp:    timestamp,
		})
	}
	
	return prices, nil
}

func (s *Service) groupPricesByPair(prices []calculator.PriceData) map[string][]calculator.PriceData {
	grouped := make(map[string][]calculator.PriceData)
	
	for _, price := range prices {
		key := fmt.Sprintf("%d-%d", price.BaseTokenID, price.QuoteTokenID)
		grouped[key] = append(grouped[key], price)
	}
	
	// Filter out pairs with insufficient exchanges
	filtered := make(map[string][]calculator.PriceData)
	for key, prices := range grouped {
		if len(prices) >= 2 { // Minimum 2 exchanges for VWAP
			filtered[key] = prices
		}
	}
	
	return filtered
}

func (s *Service) storeVWAPResults(ctx context.Context, results map[string]*calculator.VWAPResult) error {
	if len(results) == 0 {
		return nil
	}
	
	batch, err := s.clickhouseConn.PrepareBatch(ctx, `
		INSERT INTO vwap_prices (
			timestamp, base_token_id, quote_token_id,
			vwap_price, total_volume, exchange_count, contributing_exchanges
		)`)
	if err != nil {
		return fmt.Errorf("failed to prepare batch: %w", err)
	}
	
	for _, result := range results {
		if err := batch.Append(
			result.Timestamp,
			uint32(result.BaseTokenID),
			uint32(result.QuoteTokenID),
			result.VWAPPrice,
			result.TotalVolume,
			uint8(result.ExchangeCount),
			result.ContributingExchanges,
		); err != nil {
			s.logger.Error("Failed to append VWAP result",
				zap.Int("base_token_id", result.BaseTokenID),
				zap.Int("quote_token_id", result.QuoteTokenID),
				zap.Error(err))
			continue
		}
	}
	
	if err := batch.Send(); err != nil {
		return fmt.Errorf("failed to send batch: %w", err)
	}
	
	return nil
}

// GetLatestVWAP retrieves the latest VWAP for a token pair
func (s *Service) GetLatestVWAP(ctx context.Context, baseTokenID, quoteTokenID int) (*calculator.VWAPResult, error) {
	query := `
		SELECT 
			timestamp,
			vwap_price,
			total_volume,
			exchange_count,
			contributing_exchanges
		FROM vwap_prices
		WHERE base_token_id = ? AND quote_token_id = ?
		ORDER BY timestamp DESC
		LIMIT 1
	`
	
	var result calculator.VWAPResult
	result.BaseTokenID = baseTokenID
	result.QuoteTokenID = quoteTokenID
	
	err := s.clickhouseConn.QueryRow(ctx, query, baseTokenID, quoteTokenID).Scan(
		&result.Timestamp,
		&result.VWAPPrice,
		&result.TotalVolume,
		&result.ExchangeCount,
		&result.ContributingExchanges,
	)
	
	if err != nil {
		return nil, fmt.Errorf("failed to get latest VWAP: %w", err)
	}
	
	return &result, nil
}

// getExchangeWeights returns predefined exchange weights for VWAP calculation
func getExchangeWeights() map[string]decimal.Decimal {
	return map[string]decimal.Decimal{
		"binance":   decimal.NewFromFloat(0.15),
		"coinbase":  decimal.NewFromFloat(0.12),
		"kraken":    decimal.NewFromFloat(0.10),
		"okx":       decimal.NewFromFloat(0.08),
		"bybit":     decimal.NewFromFloat(0.07),
		"bitget":    decimal.NewFromFloat(0.06),
		"gateio":    decimal.NewFromFloat(0.05),
		"huobi":     decimal.NewFromFloat(0.04),
		"kucoin":    decimal.NewFromFloat(0.05),
		"cryptocom": decimal.NewFromFloat(0.03),
		"mexc":      decimal.NewFromFloat(0.03),
		"bitfinex":  decimal.NewFromFloat(0.03),
		"gemini":    decimal.NewFromFloat(0.02),
		"bitstamp":  decimal.NewFromFloat(0.02),
		// Others default to 0.01
	}
}