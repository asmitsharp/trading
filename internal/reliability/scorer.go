package reliability

import (
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// Scorer calculates exchange reliability scores (CoinMarketCap style)
type Scorer struct {
	logger *zap.Logger
	
	// Base reliability scores for known exchanges
	baseScores map[string]decimal.Decimal
	
	// Volume legitimacy tracking
	historicalVolumes map[string][]VolumeSnapshot
}

// VolumeSnapshot tracks historical volume for wash trading detection
type VolumeSnapshot struct {
	Date   time.Time
	Volume decimal.Decimal
}

// ExchangeMetrics holds various reliability metrics
type ExchangeMetrics struct {
	ExchangeID        string
	BaseScore         decimal.Decimal // Base trust score (0.1-1.0)
	VolumeLegitimacy  decimal.Decimal // Volume pattern score (0.5-1.5)
	APIReliability    decimal.Decimal // API uptime score (0.8-1.2)
	RegulationScore   decimal.Decimal // Regulatory compliance (0.5-1.2)
	FinalScore        decimal.Decimal // Calculated final score
}

// NewScorer creates a new exchange reliability scorer
func NewScorer(logger *zap.Logger) *Scorer {
	return &Scorer{
		logger:            logger,
		baseScores:        initializeBaseScores(),
		historicalVolumes: make(map[string][]VolumeSnapshot),
	}
}

// initializeBaseScores sets up base reliability scores for known exchanges
func initializeBaseScores() map[string]decimal.Decimal {
	// Based on CoinMarketCap's approach: regulation, volume, reputation
	return map[string]decimal.Decimal{
		// Tier 1: Highly regulated, transparent
		"COINBASE":  decimal.NewFromFloat(1.0),  // Publicly traded, US regulated
		"KRAKEN":    decimal.NewFromFloat(0.95), // Strong regulatory compliance
		"GEMINI":    decimal.NewFromFloat(0.95), // Winklevoss twins, US regulated
		"BITSTAMP":  decimal.NewFromFloat(0.90), // EU regulated, old exchange
		
		// Tier 2: Large, generally reliable
		"BINANCE":   decimal.NewFromFloat(0.85), // Largest but regulatory issues
		"OKX":       decimal.NewFromFloat(0.80), // Large, decent reputation
		"HUOBI":     decimal.NewFromFloat(0.75), // Large but some concerns
		"BYBIT":     decimal.NewFromFloat(0.75), // Growing, derivatives focused
		
		// Tier 3: Medium reliability
		"BITGET":    decimal.NewFromFloat(0.70), // Growing, copy trading
		"GATEIO":    decimal.NewFromFloat(0.65), // Many altcoins, some concerns
		"GATE.IO":   decimal.NewFromFloat(0.65), // Alternative spelling
		"MEXC":      decimal.NewFromFloat(0.60), // High volume but wash trading concerns
		"KUCOIN":    decimal.NewFromFloat(0.65), // Altcoin focused
		
		// Tier 4: Lower reliability
		"CRYPTOCOM": decimal.NewFromFloat(0.60), // Marketing heavy, newer
		"CRYPTO.COM": decimal.NewFromFloat(0.60), // Alternative spelling
		"BITFINEX":  decimal.NewFromFloat(0.55), // Past issues but still operating
		"HITBTC":    decimal.NewFromFloat(0.40), // Multiple regulatory warnings
		
		// Default for unknown exchanges
		"UNKNOWN":   decimal.NewFromFloat(0.20),
	}
}

// CalculateScore computes the final reliability score for an exchange
func (s *Scorer) CalculateScore(exchangeID string, volume decimal.Decimal, date time.Time) *ExchangeMetrics {
	exchangeID = strings.ToUpper(exchangeID)
	
	// Get base score
	baseScore := s.getBaseScore(exchangeID)
	
	// Calculate volume legitimacy (anti-wash trading)
	volumeLegitimacy := s.calculateVolumeLegitimacy(exchangeID, volume, date)
	
	// Calculate API reliability (simplified for now)
	apiReliability := s.calculateAPIReliability(exchangeID)
	
	// Calculate regulation score
	regulationScore := s.calculateRegulationScore(exchangeID)
	
	// Final weighted score
	finalScore := baseScore.
		Mul(volumeLegitimacy).
		Mul(apiReliability).
		Mul(regulationScore)
	
	// Clamp between 0.05 and 1.0
	finalScore = decimal.Max(decimal.NewFromFloat(0.05), 
		decimal.Min(decimal.NewFromFloat(1.0), finalScore))
	
	return &ExchangeMetrics{
		ExchangeID:       exchangeID,
		BaseScore:        baseScore,
		VolumeLegitimacy: volumeLegitimacy,
		APIReliability:   apiReliability,
		RegulationScore:  regulationScore,
		FinalScore:       finalScore,
	}
}

// getBaseScore returns the base reliability score for an exchange
func (s *Scorer) getBaseScore(exchangeID string) decimal.Decimal {
	if score, exists := s.baseScores[exchangeID]; exists {
		return score
	}
	
	// Return default score for unknown exchanges
	return s.baseScores["UNKNOWN"]
}

// calculateVolumeLegitimacy detects wash trading patterns
func (s *Scorer) calculateVolumeLegitimacy(exchangeID string, currentVolume decimal.Decimal, date time.Time) decimal.Decimal {
	// Track volume history for wash trading detection
	key := exchangeID
	if s.historicalVolumes[key] == nil {
		s.historicalVolumes[key] = make([]VolumeSnapshot, 0)
	}
	
	// Add current volume to history
	s.historicalVolumes[key] = append(s.historicalVolumes[key], VolumeSnapshot{
		Date:   date,
		Volume: currentVolume,
	})
	
	// Keep only last 30 days
	cutoff := date.AddDate(0, 0, -30)
	recentVolumes := make([]VolumeSnapshot, 0)
	for _, snap := range s.historicalVolumes[key] {
		if snap.Date.After(cutoff) {
			recentVolumes = append(recentVolumes, snap)
		}
	}
	s.historicalVolumes[key] = recentVolumes
	
	if len(recentVolumes) < 3 {
		return decimal.NewFromFloat(1.0) // Neutral score for new data
	}
	
	// Calculate average volume
	totalVolume := decimal.Zero
	for _, snap := range recentVolumes {
		totalVolume = totalVolume.Add(snap.Volume)
	}
	avgVolume := totalVolume.Div(decimal.NewFromInt(int64(len(recentVolumes))))
	
	if avgVolume.IsZero() {
		return decimal.NewFromFloat(0.5) // Low score for zero volume
	}
	
	// Check for volume spikes (potential wash trading)
	ratio := currentVolume.Div(avgVolume)
	
	switch {
	case ratio.GreaterThan(decimal.NewFromFloat(10.0)): // 10x spike
		return decimal.NewFromFloat(0.5) // Highly suspicious
	case ratio.GreaterThan(decimal.NewFromFloat(5.0)): // 5x spike
		return decimal.NewFromFloat(0.7) // Somewhat suspicious
	case ratio.GreaterThan(decimal.NewFromFloat(3.0)): // 3x spike
		return decimal.NewFromFloat(0.9) // Slightly suspicious
	case ratio.LessThan(decimal.NewFromFloat(0.1)): // Too low volume
		return decimal.NewFromFloat(0.8) // Reduced reliability
	default:
		return decimal.NewFromFloat(1.0) // Normal volume pattern
	}
}

// calculateAPIReliability assesses API uptime and quality
func (s *Scorer) calculateAPIReliability(exchangeID string) decimal.Decimal {
	// Simplified scoring based on known API quality
	apiScores := map[string]decimal.Decimal{
		"COINBASE":  decimal.NewFromFloat(1.2),  // Excellent API
		"BINANCE":   decimal.NewFromFloat(1.1),  // Very good API
		"KRAKEN":    decimal.NewFromFloat(1.0),  // Good API
		"OKX":       decimal.NewFromFloat(1.0),  // Good API
		"BYBIT":     decimal.NewFromFloat(0.95), // Decent API
		"BITGET":    decimal.NewFromFloat(0.9),  // OK API
		"GATEIO":    decimal.NewFromFloat(0.85), // Sometimes unreliable
		"MEXC":      decimal.NewFromFloat(0.8),  // Often down
		"HITBTC":    decimal.NewFromFloat(0.7),  // Poor API quality
	}
	
	if score, exists := apiScores[exchangeID]; exists {
		return score
	}
	
	return decimal.NewFromFloat(0.9) // Default for unknown exchanges
}

// calculateRegulationScore assesses regulatory compliance
func (s *Scorer) calculateRegulationScore(exchangeID string) decimal.Decimal {
	// Regulation compliance scoring
	regScores := map[string]decimal.Decimal{
		"COINBASE":  decimal.NewFromFloat(1.2),  // Publicly traded, strict compliance
		"GEMINI":    decimal.NewFromFloat(1.2),  // US regulated
		"KRAKEN":    decimal.NewFromFloat(1.1),  // Multi-jurisdiction compliance
		"BITSTAMP":  decimal.NewFromFloat(1.1),  // EU licensed
		"BINANCE":   decimal.NewFromFloat(0.8),  // Regulatory challenges
		"OKX":       decimal.NewFromFloat(0.9),  // Some compliance
		"BYBIT":     decimal.NewFromFloat(0.85), // Limited regulation
		"BITFINEX":  decimal.NewFromFloat(0.6),  // Past regulatory issues
		"HITBTC":    decimal.NewFromFloat(0.5),  // Multiple warnings
	}
	
	if score, exists := regScores[exchangeID]; exists {
		return score
	}
	
	return decimal.NewFromFloat(0.8) // Default for unknown exchanges
}

// GetQualityScore calculates overall data quality for aggregated data
func (s *Scorer) GetQualityScore(metrics []*ExchangeMetrics, volumes []decimal.Decimal) decimal.Decimal {
	if len(metrics) == 0 {
		return decimal.Zero
	}
	
	totalWeight := decimal.Zero
	weightedScore := decimal.Zero
	
	for i, metric := range metrics {
		weight := volumes[i] // Volume-weighted quality
		totalWeight = totalWeight.Add(weight)
		weightedScore = weightedScore.Add(metric.FinalScore.Mul(weight))
	}
	
	if totalWeight.IsZero() {
		return decimal.Zero
	}
	
	return weightedScore.Div(totalWeight)
}

// GetExchangeWeight returns the effective weight for VWAP calculation
func (s *Scorer) GetExchangeWeight(exchangeID string, volume decimal.Decimal, date time.Time) decimal.Decimal {
	metrics := s.CalculateScore(exchangeID, volume, date)
	
	// Base weight from config (if available) multiplied by reliability score
	baseWeight := s.getConfigWeight(exchangeID)
	return baseWeight.Mul(metrics.FinalScore)
}

// getConfigWeight gets weight from exchange config (from your existing system)
func (s *Scorer) getConfigWeight(exchangeID string) decimal.Decimal {
	// These should match your exchanges.json weights
	configWeights := map[string]decimal.Decimal{
		"BINANCE":   decimal.NewFromFloat(0.08),
		"COINBASE":  decimal.NewFromFloat(0.10),
		"KRAKEN":    decimal.NewFromFloat(0.05),
		"OKX":       decimal.NewFromFloat(0.06),
		"BYBIT":     decimal.NewFromFloat(0.04),
		"BITGET":    decimal.NewFromFloat(0.03),
		"GATEIO":    decimal.NewFromFloat(0.02),
	}
	
	if weight, exists := configWeights[exchangeID]; exists {
		return weight
	}
	
	return decimal.NewFromFloat(0.01) // Default weight
}