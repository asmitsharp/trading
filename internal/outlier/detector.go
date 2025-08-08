package outlier

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// Detector identifies price outliers that may indicate mapping issues
type Detector struct {
	postgresDB     *sql.DB
	clickhouseConn driver.Conn
	logger         *zap.Logger
	
	// Configurable thresholds
	deviationThreshold float64 // Percentage deviation to flag (default 5%)
	stdDevMultiplier   float64 // Number of standard deviations (default 2.0)
}

// NewDetector creates a new outlier detector
func NewDetector(postgresDB *sql.DB, clickhouseConn driver.Conn, logger *zap.Logger) *Detector {
	return &Detector{
		postgresDB:         postgresDB,
		clickhouseConn:     clickhouseConn,
		logger:             logger,
		deviationThreshold: 0.05, // 5% deviation
		stdDevMultiplier:   2.0,   // 2 standard deviations
	}
}

// PricePoint represents a single price data point
type PricePoint struct {
	ExchangeID   string
	BaseTokenID  int
	QuoteTokenID int
	Price        decimal.Decimal
	Timestamp    time.Time
}

// Outlier represents a detected price outlier
type Outlier struct {
	ExchangeID      string
	BaseTokenID     int
	QuoteTokenID    int
	ExchangePrice   decimal.Decimal
	AveragePrice    decimal.Decimal
	DeviationPercent float64
	StdDeviations   float64
	MappingMethod   string
	Timestamp       time.Time
}

// DetectOutliers scans recent price data for outliers
func (d *Detector) DetectOutliers(ctx context.Context, window time.Duration) ([]Outlier, error) {
	// Get recent price data grouped by token pair
	priceData, err := d.fetchRecentPrices(ctx, window)
	if err != nil {
		return nil, fmt.Errorf("fetching recent prices: %w", err)
	}
	
	// Group prices by token pair
	pricesByPair := d.groupByPair(priceData)
	
	// Detect outliers for each pair
	var outliers []Outlier
	for _, prices := range pricesByPair {
		if len(prices) < 2 {
			continue // Need at least 2 exchanges for comparison
		}
		
		pairOutliers := d.detectPairOutliers(prices)
		outliers = append(outliers, pairOutliers...)
	}
	
	// Store outliers in database
	if err := d.storeOutliers(outliers); err != nil {
		d.logger.Error("Failed to store outliers", zap.Error(err))
	}
	
	return outliers, nil
}

func (d *Detector) fetchRecentPrices(ctx context.Context, window time.Duration) ([]PricePoint, error) {
	query := `
		SELECT 
			exchange_id,
			base_token_id,
			quote_token_id,
			argMax(price, timestamp) as latest_price,
			max(timestamp) as latest_timestamp
		FROM price_tickers
		WHERE timestamp >= now() - INTERVAL ? SECOND
			AND base_token_id > 0
			AND quote_token_id > 0
			AND price > 0
		GROUP BY exchange_id, base_token_id, quote_token_id
		HAVING latest_price > 0
	`
	
	rows, err := d.clickhouseConn.Query(ctx, query, int(window.Seconds()))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var prices []PricePoint
	for rows.Next() {
		var p PricePoint
		var priceFloat float64
		
		if err := rows.Scan(&p.ExchangeID, &p.BaseTokenID, &p.QuoteTokenID, 
			&priceFloat, &p.Timestamp); err != nil {
			d.logger.Error("Failed to scan price row", zap.Error(err))
			continue
		}
		
		p.Price = decimal.NewFromFloat(priceFloat)
		prices = append(prices, p)
	}
	
	return prices, nil
}

func (d *Detector) groupByPair(prices []PricePoint) map[string][]PricePoint {
	grouped := make(map[string][]PricePoint)
	
	for _, price := range prices {
		key := fmt.Sprintf("%d-%d", price.BaseTokenID, price.QuoteTokenID)
		grouped[key] = append(grouped[key], price)
	}
	
	return grouped
}

func (d *Detector) detectPairOutliers(prices []PricePoint) []Outlier {
	if len(prices) < 2 {
		return nil
	}
	
	// Calculate statistics
	var sum, sumSquares decimal.Decimal
	for _, p := range prices {
		sum = sum.Add(p.Price)
		sumSquares = sumSquares.Add(p.Price.Mul(p.Price))
	}
	
	n := decimal.NewFromInt(int64(len(prices)))
	mean := sum.Div(n)
	
	// Calculate standard deviation
	variance := sumSquares.Div(n).Sub(mean.Mul(mean))
	stdDev, _ := variance.Float64()
	stdDev = math.Sqrt(stdDev)
	
	// Detect outliers
	var outliers []Outlier
	for _, price := range prices {
		deviation, _ := price.Price.Sub(mean).Abs().Float64()
		deviationPercent := deviation / mean.InexactFloat64() * 100
		stdDeviations := deviation / stdDev
		
		// Check if this is an outlier
		if deviationPercent > d.deviationThreshold*100 || stdDeviations > d.stdDevMultiplier {
			// Get mapping method for this exchange/token combination
			mappingMethod := d.getMappingMethod(price.ExchangeID, price.BaseTokenID)
			
			// Only flag if it's a symbol-based mapping
			if mappingMethod == "symbol" {
				outliers = append(outliers, Outlier{
					ExchangeID:       price.ExchangeID,
					BaseTokenID:      price.BaseTokenID,
					QuoteTokenID:     price.QuoteTokenID,
					ExchangePrice:    price.Price,
					AveragePrice:     mean,
					DeviationPercent: deviationPercent,
					StdDeviations:    stdDeviations,
					MappingMethod:    mappingMethod,
					Timestamp:        price.Timestamp,
				})
			}
		}
	}
	
	return outliers
}

func (d *Detector) getMappingMethod(exchangeID string, tokenID int) string {
	var method string
	query := `
		SELECT mapping_method 
		FROM token_exchange_symbols 
		WHERE exchange_id = $1 AND token_id = $2
		LIMIT 1
	`
	
	err := d.postgresDB.QueryRow(query, exchangeID, tokenID).Scan(&method)
	if err != nil {
		return "unknown"
	}
	
	return method
}

func (d *Detector) storeOutliers(outliers []Outlier) error {
	if len(outliers) == 0 {
		return nil
	}
	
	tx, err := d.postgresDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	
	stmt, err := tx.Prepare(`
		INSERT INTO price_outliers (
			exchange_id, base_token_id, quote_token_id,
			exchange_price, average_price, deviation_percent,
			standard_deviations, mapping_method
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	
	for _, outlier := range outliers {
		_, err := stmt.Exec(
			outlier.ExchangeID,
			outlier.BaseTokenID,
			outlier.QuoteTokenID,
			outlier.ExchangePrice.String(),
			outlier.AveragePrice.String(),
			outlier.DeviationPercent,
			outlier.StdDeviations,
			outlier.MappingMethod,
		)
		if err != nil {
			d.logger.Error("Failed to store outlier", zap.Error(err))
		}
	}
	
	return tx.Commit()
}

// GetUnresolvedOutliers retrieves unresolved outliers for review
func (d *Detector) GetUnresolvedOutliers() ([]Outlier, error) {
	query := `
		SELECT 
			po.exchange_id,
			po.base_token_id,
			po.quote_token_id,
			po.exchange_price,
			po.average_price,
			po.deviation_percent,
			po.standard_deviations,
			po.mapping_method,
			po.detected_at
		FROM price_outliers po
		WHERE po.is_resolved = false
		ORDER BY po.deviation_percent DESC
		LIMIT 100
	`
	
	rows, err := d.postgresDB.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var outliers []Outlier
	for rows.Next() {
		var o Outlier
		var exchangePrice, avgPrice string
		
		err := rows.Scan(
			&o.ExchangeID,
			&o.BaseTokenID,
			&o.QuoteTokenID,
			&exchangePrice,
			&avgPrice,
			&o.DeviationPercent,
			&o.StdDeviations,
			&o.MappingMethod,
			&o.Timestamp,
		)
		if err != nil {
			continue
		}
		
		o.ExchangePrice, _ = decimal.NewFromString(exchangePrice)
		o.AveragePrice, _ = decimal.NewFromString(avgPrice)
		outliers = append(outliers, o)
	}
	
	return outliers, nil
}

// ResolveOutlier marks an outlier as resolved
func (d *Detector) ResolveOutlier(outlierID int, resolvedBy, notes string) error {
	query := `
		UPDATE price_outliers 
		SET is_resolved = true,
		    resolved_at = NOW(),
		    resolved_by = $2,
		    resolution_notes = $3
		WHERE id = $1
	`
	
	_, err := d.postgresDB.Exec(query, outlierID, resolvedBy, notes)
	return err
}