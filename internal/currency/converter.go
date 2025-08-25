package currency

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// Converter handles currency conversion rates (CoinMarketCap style)
type Converter struct {
	logger *zap.Logger
	client *http.Client
	
	// Rate cache
	mu    sync.RWMutex
	rates map[string]decimal.Decimal // currency -> USD rate
	lastUpdate time.Time
	cacheTTL   time.Duration
}

// NewConverter creates a new currency converter
func NewConverter(logger *zap.Logger) *Converter {
	return &Converter{
		logger: logger,
		client: &http.Client{Timeout: 30 * time.Second},
		rates:  make(map[string]decimal.Decimal),
		cacheTTL: 6 * time.Hour, // Update rates every 6 hours
	}
}

// ForexAPIResponse represents response from exchangerate-api.com
type ForexAPIResponse struct {
	Result             string             `json:"result"`
	BaseCode           string             `json:"base_code"`
	ConversionRates    map[string]float64 `json:"conversion_rates"`
	TimeLastUpdateUTC  string             `json:"time_last_update_utc"`
}

// CryptoAPIResponse represents crypto cross-rates
type CryptoAPIResponse struct {
	Bitcoin struct {
		USD float64 `json:"usd"`
		EUR float64 `json:"eur"`
		GBP float64 `json:"gbp"`
		JPY float64 `json:"jpy"`
	} `json:"bitcoin"`
	Ethereum struct {
		USD float64 `json:"usd"`
		BTC float64 `json:"btc"`
	} `json:"ethereum"`
}

// GetUSDRate returns the USD exchange rate for a given currency
func (c *Converter) GetUSDRate(ctx context.Context, currency string) (decimal.Decimal, error) {
	currency = normalizeSymbol(currency)
	
	// USD is always 1.0
	if currency == "USD" {
		return decimal.NewFromInt(1), nil
	}
	
	c.mu.RLock()
	if rate, exists := c.rates[currency]; exists && time.Since(c.lastUpdate) < c.cacheTTL {
		c.mu.RUnlock()
		return rate, nil
	}
	c.mu.RUnlock()
	
	// Update rates if cache is stale
	if err := c.updateRates(ctx); err != nil {
		return decimal.Zero, fmt.Errorf("failed to update rates: %w", err)
	}
	
	c.mu.RLock()
	rate, exists := c.rates[currency]
	c.mu.RUnlock()
	
	if !exists {
		return decimal.Zero, fmt.Errorf("rate not found for currency: %s", currency)
	}
	
	return rate, nil
}

// ConvertToUSD converts an amount from any currency to USD
func (c *Converter) ConvertToUSD(ctx context.Context, amount decimal.Decimal, fromCurrency string) (decimal.Decimal, error) {
	if amount.IsZero() {
		return decimal.Zero, nil
	}
	
	rate, err := c.GetUSDRate(ctx, fromCurrency)
	if err != nil {
		return decimal.Zero, err
	}
	
	return amount.Mul(rate), nil
}

// ConvertPrice converts a price and volume from one currency to USD
func (c *Converter) ConvertPrice(ctx context.Context, price, volume decimal.Decimal, quoteCurrency string) (priceUSD, volumeUSD decimal.Decimal, err error) {
	rate, err := c.GetUSDRate(ctx, quoteCurrency)
	if err != nil {
		return decimal.Zero, decimal.Zero, err
	}
	
	priceUSD = price.Mul(rate)
	volumeUSD = volume.Mul(rate)
	
	return priceUSD, volumeUSD, nil
}

// updateRates fetches latest exchange rates from multiple sources
func (c *Converter) updateRates(ctx context.Context) error {
	c.logger.Info("Updating currency exchange rates")
	
	newRates := make(map[string]decimal.Decimal)
	
	// 1. Get fiat rates from exchangerate-api.com (free tier: 1500 requests/month)
	if err := c.updateForexRates(ctx, newRates); err != nil {
		c.logger.Warn("Failed to update forex rates", zap.Error(err))
	}
	
	// 2. Get crypto cross-rates from coingecko (free)
	if err := c.updateCryptoRates(ctx, newRates); err != nil {
		c.logger.Warn("Failed to update crypto rates", zap.Error(err))
	}
	
	// 3. Hardcoded backup rates for critical currencies
	c.addBackupRates(newRates)
	
	c.mu.Lock()
	c.rates = newRates
	c.lastUpdate = time.Now()
	c.mu.Unlock()
	
	c.logger.Info("Updated currency rates", zap.Int("total_currencies", len(newRates)))
	return nil
}

// updateForexRates gets fiat currency rates
func (c *Converter) updateForexRates(ctx context.Context, rates map[string]decimal.Decimal) error {
	// Using exchangerate-api.com (free tier)
	url := "https://api.exchangerate-api.com/v4/latest/USD"
	
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	var forexResp ForexAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&forexResp); err != nil {
		return err
	}
	
	// Convert to our format (1 CURRENCY = X USD)
	for currency, rate := range forexResp.ConversionRates {
		if rate > 0 {
			rates[currency] = decimal.NewFromFloat(1.0 / rate) // Invert: EUR/USD = 0.85 -> 1 EUR = 1.176 USD
		}
	}
	
	return nil
}

// updateCryptoRates gets crypto cross-rates
func (c *Converter) updateCryptoRates(ctx context.Context, rates map[string]decimal.Decimal) error {
	// Using CoinGecko API (free)
	url := "https://api.coingecko.com/api/v3/simple/price?ids=bitcoin,ethereum&vs_currencies=usd,eur,gbp,jpy,btc"
	
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	var cryptoResp CryptoAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&cryptoResp); err != nil {
		return err
	}
	
	// BTC rates
	if cryptoResp.Bitcoin.USD > 0 {
		rates["BTC"] = decimal.NewFromFloat(cryptoResp.Bitcoin.USD)
	}
	
	// ETH rates  
	if cryptoResp.Ethereum.USD > 0 {
		rates["ETH"] = decimal.NewFromFloat(cryptoResp.Ethereum.USD)
	}
	
	return nil
}

// addBackupRates adds hardcoded backup rates for critical currencies
func (c *Converter) addBackupRates(rates map[string]decimal.Decimal) {
	backupRates := map[string]decimal.Decimal{
		"USD":  decimal.NewFromInt(1),
		"USDT": decimal.NewFromFloat(1.0),
		"USDC": decimal.NewFromFloat(1.0),
		"BUSD": decimal.NewFromFloat(1.0),
		"DAI":  decimal.NewFromFloat(1.0),
		"TUSD": decimal.NewFromFloat(1.0),
		"FDUSD": decimal.NewFromFloat(1.0),
	}
	
	// Only use backup if we don't have a rate already
	for currency, rate := range backupRates {
		if _, exists := rates[currency]; !exists {
			rates[currency] = rate
		}
	}
}

// normalizeSymbol standardizes currency symbols
func normalizeSymbol(symbol string) string {
	// Handle common variations
	switch symbol {
	case "XBT":
		return "BTC"
	case "USD", "USDT", "USDC", "BUSD", "DAI", "TUSD", "FDUSD":
		return "USD" // Treat all USD stablecoins as USD
	default:
		return symbol
	}
}

// GetSupportedCurrencies returns list of supported currencies
func (c *Converter) GetSupportedCurrencies() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	currencies := make([]string, 0, len(c.rates))
	for currency := range c.rates {
		currencies = append(currencies, currency)
	}
	
	return currencies
}

// IsSupported checks if a currency is supported
func (c *Converter) IsSupported(currency string) bool {
	currency = normalizeSymbol(currency)
	
	c.mu.RLock()
	_, exists := c.rates[currency]
	c.mu.RUnlock()
	
	return exists
}