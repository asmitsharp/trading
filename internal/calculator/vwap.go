package calculator

import (
	"fmt"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// VWAPCalculator calculates Volume Weighted Average Price across exchanges
type VWAPCalculator struct {
	logger *zap.Logger
	mu     sync.RWMutex
}

// NewVWAPCalculator creates a new VWAP calculator
func NewVWAPCalculator(logger *zap.Logger) *VWAPCalculator {
	return &VWAPCalculator{
		logger: logger,
	}
}

// PriceData represents price and volume data from an exchange
type PriceData struct {
	ExchangeID string
	Symbol     string
	Price      decimal.Decimal
	Volume     decimal.Decimal
	Weight     decimal.Decimal // Exchange weight for calculation
	Timestamp  time.Time
}

// VWAPResult represents the calculated VWAP price
type VWAPResult struct {
	BaseTokenID          string
	QuoteTokenID         string
	VWAPPrice            decimal.Decimal
	TotalVolume          decimal.Decimal
	ExchangeCount        int
	ContributingExchanges []string
	PriceSources         []PriceSource
	Timestamp            time.Time
}

// PriceSource represents individual exchange contribution
type PriceSource struct {
	Exchange string          `json:"exchange"`
	Price    decimal.Decimal `json:"price"`
	Volume   decimal.Decimal `json:"volume"`
	Weight   decimal.Decimal `json:"weight"`
}

// Calculate computes VWAP from multiple exchange prices
func (v *VWAPCalculator) Calculate(prices []PriceData) (*VWAPResult, error) {
	if len(prices) == 0 {
		return nil, fmt.Errorf("no price data provided")
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	// Filter out invalid prices
	validPrices := v.filterValidPrices(prices)
	if len(validPrices) == 0 {
		return nil, fmt.Errorf("no valid prices after filtering")
	}

	// Detect and remove outliers
	cleanPrices := v.removeOutliers(validPrices)
	if len(cleanPrices) == 0 {
		return nil, fmt.Errorf("no prices left after outlier removal")
	}

	// Calculate VWAP
	result := v.calculateVWAP(cleanPrices)
	
	v.logger.Debug("VWAP calculated",
		zap.String("base_token", result.BaseTokenID),
		zap.String("quote_token", result.QuoteTokenID),
		zap.String("vwap_price", result.VWAPPrice.String()),
		zap.Int("exchanges", result.ExchangeCount))

	return result, nil
}

// filterValidPrices removes invalid price entries
func (v *VWAPCalculator) filterValidPrices(prices []PriceData) []PriceData {
	valid := make([]PriceData, 0, len(prices))
	
	for _, p := range prices {
		// Check for valid price and volume
		if p.Price.IsPositive() && p.Volume.IsPositive() {
			// Additional sanity checks
			if p.Price.LessThan(decimal.NewFromInt(1000000)) && // Max $1M per token
			   p.Volume.LessThan(decimal.NewFromInt(1000000000)) { // Max $1B volume
				valid = append(valid, p)
			} else {
				v.logger.Warn("Filtered out suspicious price",
					zap.String("exchange", p.ExchangeID),
					zap.String("price", p.Price.String()),
					zap.String("volume", p.Volume.String()))
			}
		}
	}
	
	return valid
}

// removeOutliers removes prices that deviate too much from median
func (v *VWAPCalculator) removeOutliers(prices []PriceData) []PriceData {
	if len(prices) < 3 {
		// Not enough data points to detect outliers
		return prices
	}

	// Calculate median price
	median := v.calculateMedianPrice(prices)
	
	// Define outlier threshold (e.g., 10% deviation from median)
	threshold := decimal.NewFromFloat(0.10)
	maxDeviation := median.Mul(threshold)
	
	cleaned := make([]PriceData, 0, len(prices))
	
	for _, p := range prices {
		deviation := p.Price.Sub(median).Abs()
		if deviation.LessThanOrEqual(maxDeviation) {
			cleaned = append(cleaned, p)
		} else {
			v.logger.Warn("Removed outlier price",
				zap.String("exchange", p.ExchangeID),
				zap.String("price", p.Price.String()),
				zap.String("median", median.String()),
				zap.String("deviation", deviation.String()))
		}
	}
	
	// If we removed too many prices, return original
	if len(cleaned) < len(prices)/2 {
		v.logger.Warn("Too many outliers detected, using all prices")
		return prices
	}
	
	return cleaned
}

// calculateMedianPrice finds the median price
func (v *VWAPCalculator) calculateMedianPrice(prices []PriceData) decimal.Decimal {
	// Simple median calculation
	sum := decimal.Zero
	for _, p := range prices {
		sum = sum.Add(p.Price)
	}
	return sum.Div(decimal.NewFromInt(int64(len(prices))))
}

// calculateVWAP performs the actual VWAP calculation
func (v *VWAPCalculator) calculateVWAP(prices []PriceData) *VWAPResult {
	var (
		weightedSum   = decimal.Zero
		totalVolume   = decimal.Zero
		totalWeight   = decimal.Zero
		exchanges     = make([]string, 0, len(prices))
		priceSources  = make([]PriceSource, 0, len(prices))
	)

	// Group by exchange to handle multiple pairs from same exchange
	exchangeMap := make(map[string]PriceData)
	for _, p := range prices {
		if existing, ok := exchangeMap[p.ExchangeID]; ok {
			// Use the one with higher volume
			if p.Volume.GreaterThan(existing.Volume) {
				exchangeMap[p.ExchangeID] = p
			}
		} else {
			exchangeMap[p.ExchangeID] = p
		}
	}

	// Calculate weighted sum
	for _, p := range exchangeMap {
		// Calculate contribution: price * volume * exchange_weight
		volumeWeight := p.Volume.Mul(p.Weight)
		contribution := p.Price.Mul(volumeWeight)
		
		weightedSum = weightedSum.Add(contribution)
		totalVolume = totalVolume.Add(p.Volume)
		totalWeight = totalWeight.Add(volumeWeight)
		
		exchanges = append(exchanges, p.ExchangeID)
		priceSources = append(priceSources, PriceSource{
			Exchange: p.ExchangeID,
			Price:    p.Price,
			Volume:   p.Volume,
			Weight:   p.Weight,
		})
	}

	// Calculate VWAP
	vwapPrice := decimal.Zero
	if totalWeight.IsPositive() {
		vwapPrice = weightedSum.Div(totalWeight)
	} else if totalVolume.IsPositive() {
		// Fallback to simple volume weighting if no weights
		vwapPrice = weightedSum.Div(totalVolume)
	}

	// Round to 8 decimal places
	vwapPrice = vwapPrice.Round(8)

	return &VWAPResult{
		BaseTokenID:           prices[0].Symbol, // Will be updated by caller
		QuoteTokenID:          "",               // Will be updated by caller
		VWAPPrice:             vwapPrice,
		TotalVolume:           totalVolume,
		ExchangeCount:         len(exchangeMap),
		ContributingExchanges: exchanges,
		PriceSources:          priceSources,
		Timestamp:             time.Now(),
	}
}

// CalculateBatch processes multiple token pairs in parallel
func (v *VWAPCalculator) CalculateBatch(pricesByPair map[string][]PriceData) map[string]*VWAPResult {
	results := make(map[string]*VWAPResult)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for pair, prices := range pricesByPair {
		wg.Add(1)
		go func(p string, priceData []PriceData) {
			defer wg.Done()
			
			result, err := v.Calculate(priceData)
			if err != nil {
				v.logger.Error("Failed to calculate VWAP",
					zap.String("pair", p),
					zap.Error(err))
				return
			}

			mu.Lock()
			results[p] = result
			mu.Unlock()
		}(pair, prices)
	}

	wg.Wait()
	return results
}