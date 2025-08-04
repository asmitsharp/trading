package exchanges

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// GenericRESTClient implements a configurable REST client for any exchange
type GenericRESTClient struct {
	config     ExchangeConfig
	httpClient *http.Client
	logger     *zap.Logger
	health     Health
	parser     ResponseParser
	mu         sync.RWMutex
}

// ResponseParser defines methods for parsing exchange-specific responses
type ResponseParser interface {
	ParseTickers(data []byte, exchangeID string) ([]TickerData, error)
	ParseSymbols(data []byte, exchangeID string) ([]ExchangeSymbol, error)
	ParseSymbolPair(symbol string, format string) (base, quote string)
}

// NewGenericRESTClient creates a new generic REST client for any exchange
func NewGenericRESTClient(config ExchangeConfig, parser ResponseParser, logger *zap.Logger) *GenericRESTClient {
	return &GenericRESTClient{
		config: config,
		httpClient: &http.Client{
			Timeout: time.Duration(config.RequestTimeout) * time.Millisecond,
		},
		logger: logger,
		parser: parser,
		health: Health{
			IsHealthy: true,
		},
	}
}

func (g *GenericRESTClient) GetName() string {
	return g.config.Name
}

func (g *GenericRESTClient) GetID() string {
	return g.config.ID
}

func (g *GenericRESTClient) GetWeight() float64 {
	return g.config.Weight
}

func (g *GenericRESTClient) GetAllTickers(ctx context.Context) ([]TickerData, error) {
	url := g.config.BaseURL + g.config.TickerEndpoint
	
	data, err := g.makeRequest(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("fetching tickers: %w", err)
	}

	// Use parser to handle exchange-specific response format
	return g.parser.ParseTickers(data, g.config.ID)
}

func (g *GenericRESTClient) GetTickers(ctx context.Context, symbols []string) ([]TickerData, error) {
	// For most exchanges, it's more efficient to get all tickers and filter
	allTickers, err := g.GetAllTickers(ctx)
	if err != nil {
		return nil, err
	}

	// Create a set for faster lookup
	symbolSet := make(map[string]bool)
	for _, s := range symbols {
		// Normalize symbol format based on exchange
		normalizedSymbol := g.normalizeSymbol(s)
		symbolSet[normalizedSymbol] = true
	}

	filtered := make([]TickerData, 0, len(symbols))
	for _, ticker := range allTickers {
		if symbolSet[ticker.Symbol] {
			filtered = append(filtered, ticker)
		}
	}

	return filtered, nil
}

func (g *GenericRESTClient) GetSymbols(ctx context.Context) ([]ExchangeSymbol, error) {
	url := g.config.BaseURL + g.config.SymbolsEndpoint

	data, err := g.makeRequest(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("fetching symbols: %w", err)
	}

	return g.parser.ParseSymbols(data, g.config.ID)
}

func (g *GenericRESTClient) GetRateLimit() time.Duration {
	return time.Minute / time.Duration(g.config.RateLimitPerMinute)
}

func (g *GenericRESTClient) IsHealthy() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.health.IsHealthy
}

func (g *GenericRESTClient) UpdateHealth(success bool, responseTime time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if success {
		g.health.IsHealthy = true
		g.health.LastSuccessfulPoll = time.Now()
		g.health.ConsecutiveErrors = 0
		g.health.AverageResponseMs = responseTime.Milliseconds()
	} else {
		g.health.ConsecutiveErrors++
		if g.health.ConsecutiveErrors >= 3 {
			g.health.IsHealthy = false
		}
	}
}

func (g *GenericRESTClient) makeRequest(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	// Add custom headers if needed
	req.Header.Set("User-Agent", "CryptoPlatform/1.0")
	req.Header.Set("Accept", "application/json")
	
	start := time.Now()
	resp, err := g.httpClient.Do(req)
	if err != nil {
		g.UpdateHealth(false, time.Since(start))
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	g.UpdateHealth(resp.StatusCode == http.StatusOK, time.Since(start))

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	return data, nil
}

func (g *GenericRESTClient) normalizeSymbol(symbol string) string {
	// Convert symbol to exchange-specific format
	switch g.config.SymbolFormat {
	case "BTCUSDT": // Binance format
		return strings.ToUpper(strings.ReplaceAll(symbol, "-", ""))
	case "BTC-USDT", "BTC-USD": // Hyphen separated
		return strings.ToUpper(symbol)
	case "BTC_USDT": // Underscore separated
		return strings.ToUpper(strings.ReplaceAll(symbol, "-", "_"))
	case "btcusdt": // Lowercase
		return strings.ToLower(strings.ReplaceAll(symbol, "-", ""))
	case "tBTCUSD": // Bitfinex format (t prefix for trading pairs)
		s := strings.ToUpper(strings.ReplaceAll(symbol, "-", ""))
		return "t" + s
	case "XXBTZUSD": // Kraken format (XX prefix for crypto)
		// Handle Kraken's unique naming
		parts := strings.Split(strings.ToUpper(symbol), "-")
		if len(parts) == 2 {
			base := parts[0]
			quote := parts[1]
			// Add XX prefix for crypto assets
			if base == "BTC" {
				base = "XXBT"
			} else if len(base) == 3 {
				base = "X" + base
			}
			// Add Z prefix for fiat
			if quote == "USD" || quote == "EUR" || quote == "GBP" {
				quote = "Z" + quote
			}
			return base + quote
		}
		return strings.ToUpper(symbol)
	default:
		return strings.ToUpper(symbol)
	}
}

// BaseParser provides common parsing functionality
type BaseParser struct {
	quoteCurrencies []string
}

// ParseSymbolPair attempts to split a symbol into base and quote currencies
func (b *BaseParser) ParseSymbolPair(symbol string, format string) (base, quote string) {
	// Handle different symbol formats
	switch format {
	case "BTC-USDT", "BTC-USD":
		parts := strings.Split(symbol, "-")
		if len(parts) == 2 {
			return parts[0], parts[1]
		}
	case "BTC_USDT":
		parts := strings.Split(symbol, "_")
		if len(parts) == 2 {
			return parts[0], parts[1]
		}
	case "tBTCUSD": // Bitfinex
		if strings.HasPrefix(symbol, "t") {
			symbol = symbol[1:]
		}
	case "XXBTZUSD": // Kraken
		// Remove X prefix and Z from fiat
		symbol = strings.ReplaceAll(symbol, "XXBT", "BTC")
		symbol = strings.ReplaceAll(symbol, "ZUSD", "USD")
		symbol = strings.ReplaceAll(symbol, "ZEUR", "EUR")
	}

	// Try to match against known quote currencies
	upperSymbol := strings.ToUpper(symbol)
	for _, quote := range b.quoteCurrencies {
		if strings.HasSuffix(upperSymbol, quote) {
			base := strings.TrimSuffix(upperSymbol, quote)
			return base, quote
		}
	}

	// Fallback: assume last 3-4 chars are quote
	if len(symbol) > 6 {
		return symbol[:len(symbol)-4], symbol[len(symbol)-4:]
	}
	if len(symbol) == 6 {
		return symbol[:3], symbol[3:]
	}
	
	return symbol, ""
}

// Helper function to safely parse decimal
func parseDecimalSafe(value interface{}) decimal.Decimal {
	switch v := value.(type) {
	case string:
		d, err := decimal.NewFromString(v)
		if err != nil {
			return decimal.Zero
		}
		return d
	case float64:
		return decimal.NewFromFloat(v)
	case json.Number:
		d, err := decimal.NewFromString(v.String())
		if err != nil {
			return decimal.Zero
		}
		return d
	default:
		return decimal.Zero
	}
}