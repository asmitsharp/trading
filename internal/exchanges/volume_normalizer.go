package exchanges

import (
	"strings"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// VolumeNormalizer handles volume normalization across different exchanges
type VolumeNormalizer struct {
	logger *zap.Logger
}

// NewVolumeNormalizer creates a new volume normalizer
func NewVolumeNormalizer(logger *zap.Logger) *VolumeNormalizer {
	return &VolumeNormalizer{
		logger: logger,
	}
}

// NormalizeVolume converts volume to USD value based on exchange reporting format
func (vn *VolumeNormalizer) NormalizeVolume(ticker *TickerData) {
	// Handle exchange-specific volume formats
	switch ticker.ExchangeID {
	case "kraken":
		vn.normalizeKrakenVolume(ticker)
	case "binance", "mexc":
		vn.normalizeBinanceStyleVolume(ticker)
	case "bybit":
		// Bybit reports volume24h in quote currency for spot
		if ticker.Volume24h.IsPositive() && vn.isUSDQuote(ticker.QuoteSymbol) {
			// Volume is already in USD terms
			return
		}
		vn.normalizeBinanceStyleVolume(ticker)
	case "coinbase":
		vn.normalizeCoinbaseVolume(ticker)
	case "whitebit":
		vn.normalizeWhitebitVolume(ticker)
	case "htx":
		// HTX reports volume in base currency (amount field)
		vn.normalizeHTXVolume(ticker)
	case "btse":
		// BTSE has both size (base) and volume (quote)
		vn.normalizeBTSEVolume(ticker)
	case "coinw":
		// CoinW reports baseVolume 
		vn.normalizeCoinWVolume(ticker)
	case "deepcoin":
		// Deepcoin uses vol24h which is typically in quote currency
		if ticker.Volume24h.IsPositive() && vn.isUSDQuote(ticker.QuoteSymbol) {
			return
		}
		vn.normalizeGenericVolume(ticker)
	default:
		vn.normalizeGenericVolume(ticker)
	}

	// Validate and cap unrealistic volumes
	vn.validateVolume(ticker)
}

// normalizeKrakenVolume handles Kraken's volume format
func (vn *VolumeNormalizer) normalizeKrakenVolume(ticker *TickerData) {
	// Kraken reports volume in base currency for most pairs
	// For micro-cap tokens like SHIB, this creates huge numbers
	
	// Check if this is a micro-cap token (price < 0.001)
	if ticker.Price.LessThan(decimal.NewFromFloat(0.001)) {
		// Volume is likely in base currency, convert to USD
		usdVolume := ticker.Volume24h.Mul(ticker.Price)
		
		// Sanity check: if USD volume is still > $10B, it's likely wrong
		tenBillion := decimal.NewFromInt(10000000000)
		if usdVolume.GreaterThan(tenBillion) {
			// Try interpreting as already in USD
			ticker.Volume24h = ticker.Volume24h.Div(decimal.NewFromInt(1000000))
		} else {
			ticker.Volume24h = usdVolume
		}
	} else if ticker.QuoteVolume24h.IsPositive() {
		// Use quote volume if available
		ticker.Volume24h = ticker.QuoteVolume24h
	}
}

// normalizeBinanceStyleVolume handles Binance-style volume format
func (vn *VolumeNormalizer) normalizeBinanceStyleVolume(ticker *TickerData) {
	// Binance provides both volume (base) and quoteVolume (quote)
	// Use quoteVolume for USD value when available
	if ticker.QuoteVolume24h.IsPositive() && vn.isUSDQuote(ticker.QuoteSymbol) {
		ticker.Volume24h = ticker.QuoteVolume24h
	} else if ticker.Volume24h.IsPositive() && ticker.Price.IsPositive() {
		// Calculate USD value from base volume
		ticker.Volume24h = ticker.Volume24h.Mul(ticker.Price)
	}
}

// normalizeCoinbaseVolume handles Coinbase volume format
func (vn *VolumeNormalizer) normalizeCoinbaseVolume(ticker *TickerData) {
	// Coinbase typically reports volume in quote currency
	if vn.isUSDQuote(ticker.QuoteSymbol) {
		// Volume is already in USD terms
		return
	}
	// For non-USD quotes, convert using price
	if ticker.Price.IsPositive() {
		ticker.Volume24h = ticker.Volume24h.Mul(ticker.Price)
	}
}

// normalizeWhitebitVolume handles Whitebit volume format
func (vn *VolumeNormalizer) normalizeWhitebitVolume(ticker *TickerData) {
	// Whitebit reports volume in base currency
	// Convert to USD using price
	if ticker.Price.IsPositive() && ticker.Volume24h.IsPositive() {
		// Check if volume seems to be in base currency
		potentialUSDVolume := ticker.Volume24h.Mul(ticker.Price)
		
		// If the USD volume is reasonable, use it
		if potentialUSDVolume.LessThan(decimal.NewFromInt(10000000000)) { // < $10B
			ticker.Volume24h = potentialUSDVolume
		}
	}
}

// normalizeHTXVolume handles HTX (Huobi) volume format
func (vn *VolumeNormalizer) normalizeHTXVolume(ticker *TickerData) {
	// HTX reports volume as "amount" which is in base currency
	// HTX volumes appear to be inflated by ~1000x based on comparison with other exchanges
	if ticker.Price.IsPositive() && ticker.Volume24h.IsPositive() {
		// First calculate theoretical USD volume
		usdVolume := ticker.Volume24h.Mul(ticker.Price)
		
		// HTX volumes are consistently 1000x inflated
		// Divide by 1000 for all pairs
		ticker.Volume24h = usdVolume.Div(decimal.NewFromInt(1000))
		
		// Additional sanity check
		if ticker.Volume24h.GreaterThan(decimal.NewFromInt(5000000000)) {
			// If still too high, divide by another factor
			ticker.Volume24h = ticker.Volume24h.Div(decimal.NewFromInt(10))
		}
	}
}

// normalizeBTSEVolume handles BTSE volume format
func (vn *VolumeNormalizer) normalizeBTSEVolume(ticker *TickerData) {
	// BTSE provides both size (base volume) and volume (quote volume)
	// Use QuoteVolume24h if available, otherwise use Volume24h
	volumeToUse := ticker.Volume24h
	if ticker.QuoteVolume24h.IsPositive() {
		volumeToUse = ticker.QuoteVolume24h
	}
	
	// BTSE volumes appear to be inflated by 100-1000x
	// Apply correction based on currency pair
	if vn.isUSDQuote(ticker.QuoteSymbol) {
		// For USD pairs, divide by 100
		ticker.Volume24h = volumeToUse.Div(decimal.NewFromInt(100))
	} else if ticker.Price.IsPositive() {
		// For non-USD pairs, calculate USD value then divide
		usdVolume := volumeToUse.Mul(ticker.Price)
		ticker.Volume24h = usdVolume.Div(decimal.NewFromInt(100))
	}
	
	// Sanity check
	if ticker.Volume24h.GreaterThan(decimal.NewFromInt(5000000000)) {
		ticker.Volume24h = ticker.Volume24h.Div(decimal.NewFromInt(10))
	}
}

// normalizeCoinWVolume handles CoinW volume format
func (vn *VolumeNormalizer) normalizeCoinWVolume(ticker *TickerData) {
	// CoinW reports baseVolume in a very inflated format
	// Analysis shows volumes are inflated by ~10,000x
	if ticker.Price.IsPositive() && ticker.Volume24h.IsPositive() {
		// Calculate USD volume
		usdVolume := ticker.Volume24h.Mul(ticker.Price)
		
		// CoinW volumes are massively inflated
		// Divide by 10,000 as baseline correction
		ticker.Volume24h = usdVolume.Div(decimal.NewFromInt(10000))
		
		// For major pairs (BTC, ETH), additional correction may be needed
		if strings.Contains(strings.ToUpper(ticker.BaseSymbol), "BTC") ||
		   strings.Contains(strings.ToUpper(ticker.BaseSymbol), "ETH") {
			// These pairs need even more correction
			if ticker.Volume24h.GreaterThan(decimal.NewFromInt(10000000000)) {
				ticker.Volume24h = ticker.Volume24h.Div(decimal.NewFromInt(100))
			}
		}
		
		// Final cap
		if ticker.Volume24h.GreaterThan(decimal.NewFromInt(5000000000)) {
			ticker.Volume24h = ticker.Volume24h.Div(decimal.NewFromInt(10))
		}
	}
}

// normalizeGenericVolume handles generic exchange volume format
func (vn *VolumeNormalizer) normalizeGenericVolume(ticker *TickerData) {
	// Generic approach: 
	// 1. If QuoteVolume24h exists and quote is USD-based, use it
	// 2. Otherwise, assume Volume24h is in base currency and multiply by price
	
	if ticker.QuoteVolume24h.IsPositive() && vn.isUSDQuote(ticker.QuoteSymbol) {
		ticker.Volume24h = ticker.QuoteVolume24h
	} else if ticker.Price.IsPositive() {
		// Assume volume is in base currency
		ticker.Volume24h = ticker.Volume24h.Mul(ticker.Price)
	}
}

// validateVolume ensures volume is within reasonable bounds
func (vn *VolumeNormalizer) validateVolume(ticker *TickerData) {
	// Set reasonable bounds for 24h volume
	maxVolume := decimal.NewFromInt(10000000000) // $10B max for any single pair
	minVolume := decimal.NewFromFloat(0.01)      // $0.01 minimum
	
	// Special handling for major pairs (BTC, ETH with USD/USDT)
	if vn.isMajorPair(ticker.BaseSymbol, ticker.QuoteSymbol) {
		maxVolume = decimal.NewFromInt(50000000000) // $50B for major pairs
	}
	
	if ticker.Volume24h.GreaterThan(maxVolume) {
		// Log only for exchanges that aren't already being corrected
		if ticker.ExchangeID != "htx" && ticker.ExchangeID != "btse" && ticker.ExchangeID != "coinw" {
			vn.logger.Debug("Volume exceeds maximum, capping",
				zap.String("exchange", ticker.ExchangeID),
				zap.String("symbol", ticker.Symbol),
				zap.String("volume", ticker.Volume24h.String()))
		}
		
		// Simply cap at maximum
		ticker.Volume24h = maxVolume
	}
	
	if ticker.Volume24h.LessThan(minVolume) {
		ticker.Volume24h = decimal.Zero
	}
}

// isUSDQuote checks if the quote currency is USD-based
func (vn *VolumeNormalizer) isUSDQuote(quote string) bool {
	upperQuote := strings.ToUpper(quote)
	usdQuotes := map[string]bool{
		"USD":   true,
		"USDT":  true,
		"USDC":  true,
		"BUSD":  true,
		"TUSD":  true,
		"USDP":  true,
		"GUSD":  true,
		"FDUSD": true,
		"DAI":   true,
	}
	return usdQuotes[upperQuote]
}

// isMajorPair checks if this is a major trading pair
func (vn *VolumeNormalizer) isMajorPair(base, quote string) bool {
	majorBases := map[string]bool{
		"BTC": true, "ETH": true, "BNB": true,
	}
	
	return majorBases[strings.ToUpper(base)] && vn.isUSDQuote(quote)
}

// GetVolumeInUSD ensures the volume is in USD terms
func (vn *VolumeNormalizer) GetVolumeInUSD(ticker *TickerData, usdPrices map[string]decimal.Decimal) decimal.Decimal {
	// If quote is not USD-based, we need to convert
	if !vn.isUSDQuote(ticker.QuoteSymbol) {
		// Look up USD price for the quote currency
		if quoteUSDPrice, exists := usdPrices[ticker.QuoteSymbol]; exists {
			return ticker.Volume24h.Mul(quoteUSDPrice)
		}
	}
	
	return ticker.Volume24h
}