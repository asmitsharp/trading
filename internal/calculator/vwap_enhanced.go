package calculator

import (
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// EnhancedVWAPCalculator implements CoinGecko's methodology for VWAP calculation
// with MAD-based outlier detection for 26+ exchanges
type EnhancedVWAPCalculator struct {
	logger *zap.Logger
	mu     sync.RWMutex
	
	// Configuration
	config VWAPConfig
	
	// Statistics tracking
	stats *VWAPStatistics
}

// VWAPConfig holds configuration for the VWAP calculator
type VWAPConfig struct {
	// Maximum number of tickers to consider (CoinGecko uses 600)
	MaxTickers int
	
	// Time window for considering data as stale
	StaleDataThreshold time.Duration
	
	// MAD multiplier for outlier detection (typically 4)
	MADMultiplier decimal.Decimal
	
	// Consistency constant for MAD (1.4826 for normal distribution)
	MADConsistencyConstant decimal.Decimal
	
	// Minimum exchanges needed after filtering
	MinExchanges int
	
	// Volume threshold for detecting manipulation (50 BTC equivalent)
	VolumeManipulationThreshold decimal.Decimal
	
	// Enable detailed statistics collection
	EnableDetailedStats bool
}

// VWAPStatistics tracks detailed statistics for monitoring
type VWAPStatistics struct {
	mu sync.RWMutex
	
	// Per-calculation stats
	TotalTickersReceived   int
	TickersAfterQuality    int
	TickersAfterMAD        int
	ContributingExchanges  map[string]int
	FilteredExchanges      map[string]int
	
	// Aggregated stats
	CalculationsPerformed  int64
	AverageMAD            decimal.Decimal
	AverageConfidence     decimal.Decimal
	LowConfidenceCount    int64
	InsufficientDataCount int64
}

// FilterResult represents the result of filtering operations
type FilterResult struct {
	Reason    string
	Count     int
	Exchanges []string
}

// EnhancedVWAPResult extends the basic VWAP result with additional metrics
type EnhancedVWAPResult struct {
	VWAPResult
	
	// Additional metrics
	MedianPrice          decimal.Decimal
	StandardDeviation    decimal.Decimal
	MAD                  decimal.Decimal
	ConfidenceScore      float64
	FilterResults        []FilterResult
	PriceDistribution    map[string]int // Price range distribution
	QualityIndicator     string         // "high", "medium", "low", "insufficient"
	CalculationMethod    string         // "mad", "simple_median", "insufficient_data"
}

// NewEnhancedVWAPCalculator creates a new enhanced VWAP calculator
func NewEnhancedVWAPCalculator(logger *zap.Logger, config *VWAPConfig) *EnhancedVWAPCalculator {
	// Set defaults if not provided
	if config == nil {
		config = &VWAPConfig{
			MaxTickers:                 600,
			StaleDataThreshold:         8 * time.Hour,
			MADMultiplier:              decimal.NewFromInt(4),
			MADConsistencyConstant:     decimal.NewFromFloat(1.4826),
			MinExchanges:               3,
			VolumeManipulationThreshold: decimal.NewFromInt(50), // 50 BTC equivalent
			EnableDetailedStats:        true,
		}
	}
	
	return &EnhancedVWAPCalculator{
		logger: logger,
		config: *config,
		stats: &VWAPStatistics{
			ContributingExchanges: make(map[string]int),
			FilteredExchanges:     make(map[string]int),
		},
	}
}

// Calculate performs enhanced VWAP calculation using CoinGecko methodology
func (v *EnhancedVWAPCalculator) Calculate(prices []PriceData) (*EnhancedVWAPResult, error) {
	if len(prices) == 0 {
		return nil, fmt.Errorf("no price data provided")
	}
	
	v.mu.Lock()
	defer v.mu.Unlock()
	
	result := &EnhancedVWAPResult{
		VWAPResult: VWAPResult{
			BaseTokenID:  prices[0].BaseTokenID,
			QuoteTokenID: prices[0].QuoteTokenID,
			Timestamp:    time.Now(),
		},
		FilterResults: make([]FilterResult, 0),
	}
	
	// Track initial count
	v.updateStats("received", len(prices), nil)
	
	// Stage 1: Data Quality Filters
	qualityFiltered := v.applyQualityFilters(prices, result)
	if len(qualityFiltered) == 0 {
		result.QualityIndicator = "insufficient"
		result.CalculationMethod = "insufficient_data"
		return result, fmt.Errorf("no valid prices after quality filtering")
	}
	v.updateStats("quality", len(qualityFiltered), nil)
	
	// Stage 2: Volume-based selection (top N by volume)
	volumeSelected := v.selectTopByVolume(qualityFiltered, result)
	
	// Stage 3: Volume Manipulation Detection
	manipulationFiltered := v.detectVolumeManipulation(volumeSelected, result)
	
	// Stage 4: MAD-based Outlier Detection
	madFiltered := v.applyMADFilter(manipulationFiltered, result)
	if len(madFiltered) < v.config.MinExchanges {
		result.QualityIndicator = "insufficient"
		result.CalculationMethod = "insufficient_data"
		v.updateStats("insufficient", 0, nil)
		return result, fmt.Errorf("insufficient exchanges after filtering: %d < %d", 
			len(madFiltered), v.config.MinExchanges)
	}
	v.updateStats("mad", len(madFiltered), nil)
	
	// Calculate final VWAP
	v.calculateFinalVWAP(madFiltered, result)
	
	// Calculate confidence and quality indicators
	v.calculateConfidenceMetrics(madFiltered, len(prices), result)
	
	// Update global statistics
	v.updateGlobalStats(result)
	
	return result, nil
}

// applyQualityFilters removes stale and invalid data
func (v *EnhancedVWAPCalculator) applyQualityFilters(prices []PriceData, result *EnhancedVWAPResult) []PriceData {
	filtered := make([]PriceData, 0, len(prices))
	staleExchanges := make([]string, 0)
	invalidExchanges := make([]string, 0)
	
	now := time.Now()
	for _, p := range prices {
		// Check for stale data
		if now.Sub(p.Timestamp) > v.config.StaleDataThreshold {
			staleExchanges = append(staleExchanges, p.ExchangeID)
			continue
		}
		
		// Check for valid price and volume
		if !p.Price.IsPositive() || !p.Volume.IsPositive() {
			invalidExchanges = append(invalidExchanges, p.ExchangeID)
			continue
		}
		
		// Check for unrealistic prices (basic sanity check)
		maxPrice := decimal.NewFromInt(100000000) // $100M per token
		if p.Price.GreaterThan(maxPrice) {
			invalidExchanges = append(invalidExchanges, p.ExchangeID)
			continue
		}
		
		filtered = append(filtered, p)
	}
	
	// Record filter results
	if len(staleExchanges) > 0 {
		result.FilterResults = append(result.FilterResults, FilterResult{
			Reason:    "stale_data",
			Count:     len(staleExchanges),
			Exchanges: staleExchanges,
		})
	}
	
	if len(invalidExchanges) > 0 {
		result.FilterResults = append(result.FilterResults, FilterResult{
			Reason:    "invalid_data",
			Count:     len(invalidExchanges),
			Exchanges: invalidExchanges,
		})
	}
	
	return filtered
}

// selectTopByVolume selects top N tickers by 24h volume
func (v *EnhancedVWAPCalculator) selectTopByVolume(prices []PriceData, result *EnhancedVWAPResult) []PriceData {
	if len(prices) <= v.config.MaxTickers {
		return prices
	}
	
	// Sort by volume descending
	sort.Slice(prices, func(i, j int) bool {
		return prices[i].Volume.GreaterThan(prices[j].Volume)
	})
	
	selected := prices[:v.config.MaxTickers]
	
	// Record filter result
	result.FilterResults = append(result.FilterResults, FilterResult{
		Reason: "volume_selection",
		Count:  len(prices) - v.config.MaxTickers,
	})
	
	return selected
}

// detectVolumeManipulation identifies potential volume manipulation
func (v *EnhancedVWAPCalculator) detectVolumeManipulation(prices []PriceData, result *EnhancedVWAPResult) []PriceData {
	if len(prices) < 5 {
		return prices // Not enough data to detect manipulation
	}
	
	// Calculate total volume and average
	totalVolume := decimal.Zero
	for _, p := range prices {
		totalVolume = totalVolume.Add(p.Volume)
	}
	avgVolume := totalVolume.Div(decimal.NewFromInt(int64(len(prices))))
	
	// Check if total volume exceeds manipulation threshold
	if totalVolume.LessThan(v.config.VolumeManipulationThreshold) {
		return prices // Volume too low to worry about manipulation
	}
	
	// Filter exchanges with suspicious volume (>20x average - much more lenient)
	filtered := make([]PriceData, 0, len(prices))
	suspiciousExchanges := make([]string, 0)
	twentyX := avgVolume.Mul(decimal.NewFromInt(20)) // Changed from 5x to 20x
	
	for _, p := range prices {
		if p.Volume.GreaterThan(twentyX) {
			suspiciousExchanges = append(suspiciousExchanges, p.ExchangeID)
			v.logger.Debug("Suspicious volume detected", // Changed from Warn to Debug
				zap.String("exchange", p.ExchangeID),
				zap.String("volume", p.Volume.String()),
				zap.String("avg_volume", avgVolume.String()))
		} else {
			filtered = append(filtered, p)
		}
	}
	
	if len(suspiciousExchanges) > 0 {
		result.FilterResults = append(result.FilterResults, FilterResult{
			Reason:    "volume_manipulation",
			Count:     len(suspiciousExchanges),
			Exchanges: suspiciousExchanges,
		})
	}
	
	// Ensure we still have enough exchanges
	if len(filtered) < v.config.MinExchanges {
		return prices // Return original if filtering would leave too few
	}
	
	return filtered
}

// applyMADFilter implements Median Absolute Deviation outlier detection
func (v *EnhancedVWAPCalculator) applyMADFilter(prices []PriceData, result *EnhancedVWAPResult) []PriceData {
	if len(prices) < 5 {
		// Not enough data points for MAD, use simple filtering
		return v.applySimpleFilter(prices, result)
	}
	
	// Step 1: Calculate median price (unweighted)
	median := v.calculateMedian(prices)
	
	// Step 2: Calculate absolute deviations from median
	deviations := make([]decimal.Decimal, len(prices))
	for i, p := range prices {
		deviations[i] = p.Price.Sub(median).Abs()
	}
	
	// Step 3: Calculate Median Absolute Deviation (MAD)
	mad := v.calculateMedianFromDecimals(deviations)
	
	// Step 4: Apply consistency constant (1.4826 for normal distribution)
	mmad := mad.Mul(v.config.MADConsistencyConstant)
	
	// Step 5: Set bounds: MEDIAN ± (MMAD × multiplier)
	threshold := mmad.Mul(v.config.MADMultiplier)
	lowerBound := median.Sub(threshold)
	upperBound := median.Add(threshold)
	
	// Step 6: Filter prices outside bounds
	filtered := make([]PriceData, 0, len(prices))
	outlierExchanges := make([]string, 0)
	
	for _, p := range prices {
		if p.Price.GreaterThanOrEqual(lowerBound) && p.Price.LessThanOrEqual(upperBound) {
			filtered = append(filtered, p)
		} else {
			outlierExchanges = append(outlierExchanges, p.ExchangeID)
			v.logger.Debug("MAD outlier detected",
				zap.String("exchange", p.ExchangeID),
				zap.String("price", p.Price.String()),
				zap.String("median", median.String()),
				zap.String("mad", mad.String()),
				zap.String("bounds", fmt.Sprintf("[%s, %s]", lowerBound.String(), upperBound.String())))
		}
	}
	
	// Record statistics
	result.MedianPrice = median
	result.MAD = mad
	
	if len(outlierExchanges) > 0 {
		result.FilterResults = append(result.FilterResults, FilterResult{
			Reason:    "mad_outlier",
			Count:     len(outlierExchanges),
			Exchanges: outlierExchanges,
		})
	}
	
	// Check if too many outliers (>50% flagged)
	if len(outlierExchanges) > len(prices)/2 {
		v.logger.Warn("Excessive outliers detected, possible market event",
			zap.Int("outliers", len(outlierExchanges)),
			zap.Int("total", len(prices)))
	}
	
	return filtered
}

// applySimpleFilter is used when there aren't enough data points for MAD
func (v *EnhancedVWAPCalculator) applySimpleFilter(prices []PriceData, result *EnhancedVWAPResult) []PriceData {
	if len(prices) <= 2 {
		return prices // Not enough to filter
	}
	
	// Calculate simple average
	sum := decimal.Zero
	for _, p := range prices {
		sum = sum.Add(p.Price)
	}
	avg := sum.Div(decimal.NewFromInt(int64(len(prices))))
	
	// Filter prices that deviate more than 30% from average
	threshold := avg.Mul(decimal.NewFromFloat(0.30))
	filtered := make([]PriceData, 0, len(prices))
	
	for _, p := range prices {
		deviation := p.Price.Sub(avg).Abs()
		if deviation.LessThanOrEqual(threshold) {
			filtered = append(filtered, p)
		}
	}
	
	result.CalculationMethod = "simple_median"
	return filtered
}

// calculateMedian calculates the median price from price data
func (v *EnhancedVWAPCalculator) calculateMedian(prices []PriceData) decimal.Decimal {
	if len(prices) == 0 {
		return decimal.Zero
	}
	
	// Extract and sort prices
	values := make([]decimal.Decimal, len(prices))
	for i, p := range prices {
		values[i] = p.Price
	}
	
	return v.calculateMedianFromDecimals(values)
}

// calculateMedianFromDecimals calculates median from decimal values
func (v *EnhancedVWAPCalculator) calculateMedianFromDecimals(values []decimal.Decimal) decimal.Decimal {
	if len(values) == 0 {
		return decimal.Zero
	}
	
	// Sort values
	sort.Slice(values, func(i, j int) bool {
		return values[i].LessThan(values[j])
	})
	
	n := len(values)
	if n%2 == 0 {
		// Even number of values, take average of middle two
		mid1 := values[n/2-1]
		mid2 := values[n/2]
		return mid1.Add(mid2).Div(decimal.NewFromInt(2))
	}
	
	// Odd number of values, take middle one
	return values[n/2]
}

// calculateFinalVWAP calculates the final VWAP from filtered prices
func (v *EnhancedVWAPCalculator) calculateFinalVWAP(prices []PriceData, result *EnhancedVWAPResult) {
	if len(prices) == 0 {
		return
	}
	
	// NO MANUAL WEIGHTS - let volume determine influence
	totalWeightedPrice := decimal.Zero
	totalVolume := decimal.Zero
	exchanges := make([]string, 0, len(prices))
	priceSources := make([]PriceSource, 0, len(prices))
	
	// Group by exchange (handle multiple pairs from same exchange)
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
	
	// Calculate volume-weighted average (no manual weights)
	for _, p := range exchangeMap {
		// Each exchange's influence comes ONLY from its volume
		contribution := p.Price.Mul(p.Volume)
		totalWeightedPrice = totalWeightedPrice.Add(contribution)
		totalVolume = totalVolume.Add(p.Volume)
		
		exchanges = append(exchanges, p.ExchangeID)
		priceSources = append(priceSources, PriceSource{
			Exchange: p.ExchangeID,
			Price:    p.Price,
			Volume:   p.Volume,
			Weight:   decimal.NewFromInt(1), // Equal weight, volume determines influence
		})
	}
	
	// Calculate final VWAP
	vwapPrice := decimal.Zero
	if totalVolume.IsPositive() {
		vwapPrice = totalWeightedPrice.Div(totalVolume)
	}
	
	// Round to 8 decimal places
	vwapPrice = vwapPrice.Round(8)
	
	// Update result
	result.VWAPPrice = vwapPrice
	result.TotalVolume = totalVolume
	result.ExchangeCount = len(exchangeMap)
	result.ContributingExchanges = exchanges
	result.PriceSources = priceSources
	result.CalculationMethod = "mad"
}

// calculateConfidenceMetrics calculates confidence score and quality indicators
func (v *EnhancedVWAPCalculator) calculateConfidenceMetrics(filtered []PriceData, originalCount int, result *EnhancedVWAPResult) {
	// Calculate standard deviation
	if len(filtered) > 1 {
		mean := result.VWAPPrice
		sumSquares := decimal.Zero
		
		for _, p := range filtered {
			diff := p.Price.Sub(mean)
			sumSquares = sumSquares.Add(diff.Mul(diff))
		}
		
		variance := sumSquares.Div(decimal.NewFromInt(int64(len(filtered))))
		stdDevFloat, _ := variance.Float64()
		stdDevFloat = math.Sqrt(stdDevFloat)
		result.StandardDeviation = decimal.NewFromFloat(stdDevFloat)
	}
	
	// Calculate confidence score (0-1)
	// Use a more realistic approach: most pairs trade on 3-10 exchanges
	// Score based on actual participation rather than total possible
	maxExpectedExchanges := 10.0 // Most pairs realistically trade on up to 10 exchanges
	exchangeRatio := math.Min(float64(result.ExchangeCount)/maxExpectedExchanges, 1.0)
	
	volumeFactor := 1.0
	if result.TotalVolume.LessThan(decimal.NewFromInt(10000)) { // $10k minimum volume
		volumeFactor = 0.5
	}
	
	result.ConfidenceScore = exchangeRatio * volumeFactor
	
	// Determine quality indicator
	if result.ExchangeCount >= 5 && result.ConfidenceScore > 0.4 {
		result.QualityIndicator = "high"
	} else if result.ExchangeCount >= 3 && result.ConfidenceScore > 0.25 {
		result.QualityIndicator = "medium"
	} else if result.ExchangeCount >= v.config.MinExchanges {
		result.QualityIndicator = "low"
	} else {
		result.QualityIndicator = "insufficient"
	}
	
	// Calculate price distribution
	result.PriceDistribution = v.calculatePriceDistribution(filtered, result.VWAPPrice)
}

// calculatePriceDistribution shows how prices are distributed around VWAP
func (v *EnhancedVWAPCalculator) calculatePriceDistribution(prices []PriceData, vwap decimal.Decimal) map[string]int {
	distribution := map[string]int{
		"within_1%":  0,
		"within_2%":  0,
		"within_5%":  0,
		"within_10%": 0,
		"beyond_10%": 0,
	}
	
	// Avoid division by zero
	if vwap.IsZero() {
		return distribution
	}
	
	for _, p := range prices {
		deviation := p.Price.Sub(vwap).Abs().Div(vwap).Mul(decimal.NewFromInt(100))
		
		if deviation.LessThanOrEqual(decimal.NewFromInt(1)) {
			distribution["within_1%"]++
		} else if deviation.LessThanOrEqual(decimal.NewFromInt(2)) {
			distribution["within_2%"]++
		} else if deviation.LessThanOrEqual(decimal.NewFromInt(5)) {
			distribution["within_5%"]++
		} else if deviation.LessThanOrEqual(decimal.NewFromInt(10)) {
			distribution["within_10%"]++
		} else {
			distribution["beyond_10%"]++
		}
	}
	
	return distribution
}

// updateStats updates internal statistics
func (v *EnhancedVWAPCalculator) updateStats(stage string, count int, exchanges []string) {
	if !v.config.EnableDetailedStats {
		return
	}
	
	v.stats.mu.Lock()
	defer v.stats.mu.Unlock()
	
	switch stage {
	case "received":
		v.stats.TotalTickersReceived += count
	case "quality":
		v.stats.TickersAfterQuality += count
	case "mad":
		v.stats.TickersAfterMAD += count
	case "insufficient":
		v.stats.InsufficientDataCount++
	}
	
	// Track contributing exchanges
	for _, ex := range exchanges {
		v.stats.ContributingExchanges[ex]++
	}
}

// updateGlobalStats updates global statistics after calculation
func (v *EnhancedVWAPCalculator) updateGlobalStats(result *EnhancedVWAPResult) {
	if !v.config.EnableDetailedStats {
		return
	}
	
	v.stats.mu.Lock()
	defer v.stats.mu.Unlock()
	
	v.stats.CalculationsPerformed++
	
	// Update average MAD
	if v.stats.CalculationsPerformed == 1 {
		v.stats.AverageMAD = result.MAD
	} else {
		// Running average
		prevWeight := decimal.NewFromInt(v.stats.CalculationsPerformed - 1)
		v.stats.AverageMAD = v.stats.AverageMAD.Mul(prevWeight).Add(result.MAD).
			Div(decimal.NewFromInt(v.stats.CalculationsPerformed))
	}
	
	// Update average confidence
	confidence := decimal.NewFromFloat(result.ConfidenceScore)
	if v.stats.CalculationsPerformed == 1 {
		v.stats.AverageConfidence = confidence
	} else {
		prevWeight := decimal.NewFromInt(v.stats.CalculationsPerformed - 1)
		v.stats.AverageConfidence = v.stats.AverageConfidence.Mul(prevWeight).Add(confidence).
			Div(decimal.NewFromInt(v.stats.CalculationsPerformed))
	}
	
	// Track low confidence calculations
	if result.ConfidenceScore < 0.3 {
		v.stats.LowConfidenceCount++
	}
}

// GetStatistics returns current statistics
func (v *EnhancedVWAPCalculator) GetStatistics() VWAPStatistics {
	v.stats.mu.RLock()
	defer v.stats.mu.RUnlock()
	
	// Return a copy
	return VWAPStatistics{
		TotalTickersReceived:   v.stats.TotalTickersReceived,
		TickersAfterQuality:    v.stats.TickersAfterQuality,
		TickersAfterMAD:        v.stats.TickersAfterMAD,
		ContributingExchanges:  v.stats.ContributingExchanges,
		FilteredExchanges:      v.stats.FilteredExchanges,
		CalculationsPerformed:  v.stats.CalculationsPerformed,
		AverageMAD:            v.stats.AverageMAD,
		AverageConfidence:     v.stats.AverageConfidence,
		LowConfidenceCount:    v.stats.LowConfidenceCount,
		InsufficientDataCount: v.stats.InsufficientDataCount,
	}
}

// CalculateBatch processes multiple token pairs in parallel
func (v *EnhancedVWAPCalculator) CalculateBatch(pricesByPair map[string][]PriceData) map[string]*EnhancedVWAPResult {
	results := make(map[string]*EnhancedVWAPResult)
	var wg sync.WaitGroup
	var mu sync.Mutex
	
	// Use semaphore to limit concurrent calculations
	sem := make(chan struct{}, 10) // Process max 10 pairs concurrently
	
	for pair, prices := range pricesByPair {
		wg.Add(1)
		go func(p string, priceData []PriceData) {
			defer wg.Done()
			
			sem <- struct{}{}        // Acquire
			defer func() { <-sem }() // Release
			
			result, err := v.Calculate(priceData)
			if err != nil {
				v.logger.Error("Failed to calculate enhanced VWAP",
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