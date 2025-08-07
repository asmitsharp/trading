package storage

import (
	"context"
	"fmt"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/ashmitsharp/trading/internal/calculator"
	"go.uber.org/zap"
)

// VWAPStorage handles storage of VWAP calculation results
type VWAPStorage struct {
	conn   driver.Conn
	logger *zap.Logger
}

// NewVWAPStorage creates a new VWAP storage service
func NewVWAPStorage(conn driver.Conn, logger *zap.Logger) *VWAPStorage {
	return &VWAPStorage{
		conn:   conn,
		logger: logger,
	}
}

// StoreVWAPResults stores VWAP calculation results in ClickHouse
func (s *VWAPStorage) StoreVWAPResults(ctx context.Context, results map[string]*calculator.VWAPResult) error {
	if len(results) == 0 {
		return nil
	}

	batch, err := s.conn.PrepareBatch(ctx, `
		INSERT INTO vwap_prices (
			timestamp, base_token_id, quote_token_id,
			vwap_price, total_volume, exchange_count, contributing_exchanges
		)`)
	if err != nil {
		return fmt.Errorf("preparing VWAP batch: %w", err)
	}

	count := 0
	skipped := 0
	for pair, result := range results {
		// Skip if token IDs are not set (0 means unmapped)
		// For now, we'll store with 0 IDs and map them later
		// This allows us to see the data even without token mapping
		
		exchangeList := make([]string, len(result.ContributingExchanges))
		copy(exchangeList, result.ContributingExchanges)

		if err := batch.Append(
			result.Timestamp,
			uint32(result.BaseTokenID),
			uint32(result.QuoteTokenID),
			result.VWAPPrice,
			result.TotalVolume,
			uint8(result.ExchangeCount),
			exchangeList,
		); err != nil {
			s.logger.Debug("Failed to append VWAP result",
				zap.String("pair", pair),
				zap.Error(err))
			skipped++
			continue
		}
		count++
	}

	if count > 0 {
		if err := batch.Send(); err != nil {
			return fmt.Errorf("sending VWAP batch: %w", err)
		}
		
		s.logger.Info("Stored VWAP prices",
			zap.Int("stored", count),
			zap.Int("skipped", skipped),
			zap.Int("total", len(results)))
	}

	return nil
}

// GetLatestVWAP retrieves the latest VWAP for a token pair
func (s *VWAPStorage) GetLatestVWAP(ctx context.Context, baseTokenID, quoteTokenID int) (*calculator.VWAPResult, error) {
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

	err := s.conn.QueryRow(ctx, query, baseTokenID, quoteTokenID).Scan(
		&result.Timestamp,
		&result.VWAPPrice,
		&result.TotalVolume,
		&result.ExchangeCount,
		&result.ContributingExchanges,
	)

	if err != nil {
		return nil, fmt.Errorf("querying latest VWAP: %w", err)
	}

	return &result, nil
}

// GetVWAPHistory retrieves VWAP history for a token pair
func (s *VWAPStorage) GetVWAPHistory(ctx context.Context, baseTokenID, quoteTokenID int, limit int) ([]*calculator.VWAPResult, error) {
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
		LIMIT ?
	`

	rows, err := s.conn.Query(ctx, query, baseTokenID, quoteTokenID, limit)
	if err != nil {
		return nil, fmt.Errorf("querying VWAP history: %w", err)
	}
	defer rows.Close()

	var results []*calculator.VWAPResult
	for rows.Next() {
		result := &calculator.VWAPResult{
			BaseTokenID:  baseTokenID,
			QuoteTokenID: quoteTokenID,
		}
		
		if err := rows.Scan(
			&result.Timestamp,
			&result.VWAPPrice,
			&result.TotalVolume,
			&result.ExchangeCount,
			&result.ContributingExchanges,
		); err != nil {
			s.logger.Error("Failed to scan VWAP result", zap.Error(err))
			continue
		}
		
		results = append(results, result)
	}

	return results, nil
}