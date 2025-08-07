package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/ashmitsharp/trading/internal/exchanges"
	"go.uber.org/zap"
)

// PriceStorage handles storage of price ticker data
type PriceStorage struct {
	conn   driver.Conn
	logger *zap.Logger
}

// NewPriceStorage creates a new price storage service
func NewPriceStorage(conn driver.Conn, logger *zap.Logger) *PriceStorage {
	return &PriceStorage{
		conn:   conn,
		logger: logger,
	}
}

// StorePriceTickers stores price ticker data in ClickHouse
func (s *PriceStorage) StorePriceTickers(ctx context.Context, tickers []exchanges.TickerData) error {
	if len(tickers) == 0 {
		return nil
	}

	batch, err := s.conn.PrepareBatch(ctx, `
		INSERT INTO price_tickers (
			timestamp, exchange_id, symbol, base_symbol, quote_symbol,
			base_token_id, quote_token_id, price, volume_24h, quote_volume_24h,
			price_change_24h, high_24h, low_24h
		)`)
	if err != nil {
		return fmt.Errorf("preparing batch: %w", err)
	}

	count := 0
	for _, ticker := range tickers {
		// Skip if symbols are not properly parsed
		if ticker.BaseSymbol == "" || ticker.QuoteSymbol == "" {
			continue
		}

		if err := batch.Append(
			ticker.Timestamp,
			ticker.ExchangeID,
			ticker.Symbol,
			ticker.BaseSymbol,
			ticker.QuoteSymbol,
			uint32(ticker.BaseTokenID),   // Will be 0 if not mapped yet
			uint32(ticker.QuoteTokenID),  // Will be 0 if not mapped yet
			ticker.Price,
			ticker.Volume24h,
			ticker.QuoteVolume24h,
			ticker.PriceChange24h,
			ticker.High24h,
			ticker.Low24h,
		); err != nil {
			s.logger.Debug("Failed to append ticker",
				zap.String("exchange", ticker.ExchangeID),
				zap.String("symbol", ticker.Symbol),
				zap.Error(err))
			continue
		}
		count++
	}

	if count > 0 {
		if err := batch.Send(); err != nil {
			return fmt.Errorf("sending batch: %w", err)
		}
		s.logger.Debug("Stored price tickers",
			zap.Int("count", count),
			zap.Int("total", len(tickers)))
	}

	return nil
}

// GetLatestPrices retrieves the latest prices for VWAP calculation
func (s *PriceStorage) GetLatestPrices(ctx context.Context, window time.Duration) ([]exchanges.TickerData, error) {
	query := `
		SELECT 
			exchange_id,
			symbol,
			base_symbol,
			quote_symbol,
			base_token_id,
			quote_token_id,
			argMax(price, timestamp) as latest_price,
			max(volume_24h) as volume,
			max(timestamp) as latest_timestamp
		FROM price_tickers
		WHERE timestamp >= now() - INTERVAL ? SECOND
		GROUP BY exchange_id, symbol, base_symbol, quote_symbol, base_token_id, quote_token_id
		HAVING latest_price > 0
	`

	rows, err := s.conn.Query(ctx, query, int(window.Seconds()))
	if err != nil {
		return nil, fmt.Errorf("querying prices: %w", err)
	}
	defer rows.Close()

	var tickers []exchanges.TickerData
	for rows.Next() {
		var ticker exchanges.TickerData
		if err := rows.Scan(
			&ticker.ExchangeID,
			&ticker.Symbol,
			&ticker.BaseSymbol,
			&ticker.QuoteSymbol,
			&ticker.BaseTokenID,
			&ticker.QuoteTokenID,
			&ticker.Price,
			&ticker.Volume24h,
			&ticker.Timestamp,
		); err != nil {
			s.logger.Error("Failed to scan ticker", zap.Error(err))
			continue
		}
		tickers = append(tickers, ticker)
	}

	return tickers, nil
}

// UpdateExchangeHealth stores exchange health metrics
func (s *PriceStorage) UpdateExchangeHealth(ctx context.Context, exchangeID string, isHealthy bool, responseTime time.Duration) error {
	query := `
		INSERT INTO exchange_health (
			timestamp, exchange_id, is_healthy, response_time_ms, error_count
		) VALUES (?, ?, ?, ?, ?)
	`

	errorCount := 0
	if !isHealthy {
		errorCount = 1
	}

	err := s.conn.Exec(ctx, query,
		time.Now(),
		exchangeID,
		isHealthy,
		responseTime.Milliseconds(),
		errorCount,
	)

	if err != nil {
		return fmt.Errorf("updating exchange health: %w", err)
	}

	return nil
}