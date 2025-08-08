package exchanges

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// ExchangeFactory creates exchange clients based on configuration
type ExchangeFactory struct {
	logger  *zap.Logger
	configs map[string]ExchangeConfig
}

// NewExchangeFactory creates a new exchange factory
func NewExchangeFactory(configPath string, logger *zap.Logger) (*ExchangeFactory, error) {
	configs, err := loadExchangeConfigs(configPath)
	if err != nil {
		return nil, fmt.Errorf("loading exchange configs: %w", err)
	}

	return &ExchangeFactory{
		logger:  logger,
		configs: configs,
	}, nil
}

// CreateClient creates an exchange client for the given exchange ID
func (f *ExchangeFactory) CreateClient(exchangeID string) (ExchangeClient, error) {
	config, ok := f.configs[exchangeID]
	if !ok {
		return nil, fmt.Errorf("unknown exchange: %s", exchangeID)
	}

	parser := f.createParser(exchangeID, config)
	return NewGenericRESTClient(config, parser, f.logger), nil
}

// CreateAllClients creates clients for all configured exchanges
func (f *ExchangeFactory) CreateAllClients() map[string]ExchangeClient {
	clients := make(map[string]ExchangeClient)

	for exchangeID := range f.configs {
		client, err := f.CreateClient(exchangeID)
		if err != nil {
			f.logger.Error("Failed to create client",
				zap.String("exchange", exchangeID),
				zap.Error(err))
			continue
		}
		clients[exchangeID] = client
	}

	return clients
}

// GetActiveExchanges returns a list of active exchange IDs
func (f *ExchangeFactory) GetActiveExchanges() []string {
	exchanges := make([]string, 0, len(f.configs))
	for id := range f.configs {
		exchanges = append(exchanges, id)
	}
	return exchanges
}

// createParser creates the appropriate parser for the exchange
func (f *ExchangeFactory) createParser(exchangeID string, config ExchangeConfig) ResponseParser {
	// Define quote currencies for the parser
	quoteCurrencies := config.QuoteCurrencies
	if len(quoteCurrencies) == 0 {
		// Default quote currencies
		quoteCurrencies = []string{
			// Stablecoins
			"USDT", "USDC", "USD", "BUSD", "DAI", "TUSD", "FDUSD", "EURI",
			// Fiat currencies
			"EUR", "GBP", "JPY", "KRW", "INR", "TRY", "BRL", "MXN", 
			"ARS", "ZAR", "UAH", "COP", "SGD", "AUD", "CAD", "CHF",
			"PLN", "RUB", "CNY", "HKD", "NZD", "THB", "IDR", "PHP",
			// Crypto quote pairs
			"BTC", "ETH", "BNB", "SOL", "DOGE", "SHIB",
		}
	}

	// Select parser based on exchange ID or response format
	switch exchangeID {
	case "binance", "mexc":
		return &BinanceStyleParser{
			StandardParser: StandardParser{
				BaseParser: BaseParser{quoteCurrencies: quoteCurrencies},
			},
		}
	case "coinbase", "gemini":
		return &CoinbaseStyleParser{
			StandardParser: StandardParser{
				BaseParser: BaseParser{quoteCurrencies: quoteCurrencies},
			},
		}
	case "kraken":
		return &KrakenStyleParser{
			StandardParser: StandardParser{
				BaseParser: BaseParser{quoteCurrencies: quoteCurrencies},
			},
		}
	case "okx", "bybit", "bitget", "gateio", "huobi", "kucoin":
		// These exchanges have similar response formats
		return &UnifiedParser{
			StandardParser: StandardParser{
				BaseParser: BaseParser{quoteCurrencies: quoteCurrencies},
			},
			symbolFormat: config.SymbolFormat,
		}
	default:
		// Default to unified parser for other exchanges
		return &UnifiedParser{
			StandardParser: StandardParser{
				BaseParser: BaseParser{quoteCurrencies: quoteCurrencies},
			},
			symbolFormat: config.SymbolFormat,
		}
	}
}

// UnifiedParser handles most modern exchange formats
type UnifiedParser struct {
	StandardParser
	symbolFormat string
}

func (p *UnifiedParser) ParseTickers(data []byte, exchangeID string) ([]TickerData, error) {
	// Try to parse as array first
	var arrayTickers []map[string]interface{}
	if err := json.Unmarshal(data, &arrayTickers); err == nil {
		return p.parseArrayTickers(arrayTickers, exchangeID)
	}

	// Try to parse as object with data field
	var objResponse map[string]interface{}
	if err := json.Unmarshal(data, &objResponse); err == nil {
		// Check for common data field names
		for _, field := range []string{"data", "result", "tickers", "ticker"} {
			if val, ok := objResponse[field]; ok {
				if tickers, ok := val.([]interface{}); ok {
					return p.parseInterfaceTickers(tickers, exchangeID)
				}
			}
		}
	}

	return nil, fmt.Errorf("unable to parse ticker response")
}

func (p *UnifiedParser) parseArrayTickers(tickers []map[string]interface{}, exchangeID string) ([]TickerData, error) {
	result := make([]TickerData, 0, len(tickers))

	for _, raw := range tickers {
		// Try different field names for symbol
		symbol := p.getSymbolField(raw)
		if symbol == "" {
			continue
		}

		base, quote := p.ParseSymbolPair(symbol, p.symbolFormat)

		ticker := TickerData{
			ExchangeID:     exchangeID,
			Symbol:         symbol,
			BaseSymbol:     base,
			QuoteSymbol:    quote,
			Price:          p.getPriceField(raw),
			Volume24h:      p.getVolumeField(raw),
			QuoteVolume24h: p.getQuoteVolumeField(raw),
			PriceChange24h: p.getPriceChangeField(raw),
			High24h:        p.getHighField(raw),
			Low24h:         p.getLowField(raw),
			Timestamp:      time.Now(),
		}

		if ticker.Price.IsPositive() {
			result = append(result, ticker)
		}
	}

	return result, nil
}

func (p *UnifiedParser) parseInterfaceTickers(tickers []interface{}, exchangeID string) ([]TickerData, error) {
	result := make([]TickerData, 0, len(tickers))

	for _, t := range tickers {
		if raw, ok := t.(map[string]interface{}); ok {
			symbol := p.getSymbolField(raw)
			if symbol == "" {
				continue
			}

			base, quote := p.ParseSymbolPair(symbol, p.symbolFormat)

			ticker := TickerData{
				ExchangeID:     exchangeID,
				Symbol:         symbol,
				BaseSymbol:     base,
				QuoteSymbol:    quote,
				Price:          p.getPriceField(raw),
				Volume24h:      p.getVolumeField(raw),
				QuoteVolume24h: p.getQuoteVolumeField(raw),
				PriceChange24h: p.getPriceChangeField(raw),
				High24h:        p.getHighField(raw),
				Low24h:         p.getLowField(raw),
				Timestamp:      time.Now(),
			}

			if ticker.Price.IsPositive() {
				result = append(result, ticker)
			}
		}
	}

	return result, nil
}

func (p *UnifiedParser) ParseSymbols(data []byte, exchangeID string) ([]ExchangeSymbol, error) {
	// Generic symbol parsing - can be extended for specific exchanges
	var symbols []ExchangeSymbol

	// Try different response formats
	var arrayResponse []map[string]interface{}
	if err := json.Unmarshal(data, &arrayResponse); err == nil {
		for _, item := range arrayResponse {
			symbol := p.getSymbolField(item)
			if symbol != "" {
				base, quote := p.ParseSymbolPair(symbol, p.symbolFormat)
				symbols = append(symbols, ExchangeSymbol{
					ExchangeID:  exchangeID,
					Symbol:      symbol,
					BaseSymbol:  base,
					QuoteSymbol: quote,
					IsActive:    true,
				})
			}
		}
		return symbols, nil
	}

	// Try object with data field
	var objResponse map[string]interface{}
	if err := json.Unmarshal(data, &objResponse); err == nil {
		for _, field := range []string{"data", "result", "symbols"} {
			if val, ok := objResponse[field]; ok {
				if items, ok := val.([]interface{}); ok {
					for _, item := range items {
						if m, ok := item.(map[string]interface{}); ok {
							symbol := p.getSymbolField(m)
							if symbol != "" {
								base, quote := p.ParseSymbolPair(symbol, p.symbolFormat)
								symbols = append(symbols, ExchangeSymbol{
									ExchangeID:  exchangeID,
									Symbol:      symbol,
									BaseSymbol:  base,
									QuoteSymbol: quote,
									IsActive:    true,
								})
							}
						}
					}
					return symbols, nil
				}
			}
		}
	}

	return nil, fmt.Errorf("unable to parse symbols response")
}

// Field extraction helpers
func (p *UnifiedParser) getSymbolField(data map[string]interface{}) string {
	fields := []string{"symbol", "Symbol", "pair", "market", "instId", "ticker_id", "id"}
	for _, field := range fields {
		if val := getStringField(data, field); val != "" {
			return val
		}
	}
	return ""
}

func (p *UnifiedParser) getPriceField(data map[string]interface{}) decimal.Decimal {
	fields := []string{"last", "lastPrice", "last_price", "price", "close", "lastTrade"}
	for _, field := range fields {
		if val := parseDecimalField(data, field); val.IsPositive() {
			return val
		}
	}
	return decimal.Zero
}

func (p *UnifiedParser) getVolumeField(data map[string]interface{}) decimal.Decimal {
	fields := []string{"volume", "vol", "volume_24h", "baseVolume", "base_volume", "vol24h"}
	for _, field := range fields {
		if val := parseDecimalField(data, field); val.IsPositive() {
			return val
		}
	}
	return decimal.Zero
}

func (p *UnifiedParser) getQuoteVolumeField(data map[string]interface{}) decimal.Decimal {
	fields := []string{"quoteVolume", "quote_volume", "volCcy", "volume_usd", "quoteVol"}
	for _, field := range fields {
		if val := parseDecimalField(data, field); val.IsPositive() {
			return val
		}
	}
	return decimal.Zero
}

func (p *UnifiedParser) getPriceChangeField(data map[string]interface{}) decimal.Decimal {
	fields := []string{"priceChange", "price_change", "change", "priceChange24h"}
	for _, field := range fields {
		if val := parseDecimalField(data, field); !val.IsZero() {
			return val
		}
	}
	return decimal.Zero
}

func (p *UnifiedParser) getHighField(data map[string]interface{}) decimal.Decimal {
	fields := []string{"high", "highPrice", "high_24h", "high24h", "h"}
	for _, field := range fields {
		if val := parseDecimalField(data, field); val.IsPositive() {
			return val
		}
	}
	return decimal.Zero
}

func (p *UnifiedParser) getLowField(data map[string]interface{}) decimal.Decimal {
	fields := []string{"low", "lowPrice", "low_24h", "low24h", "l"}
	for _, field := range fields {
		if val := parseDecimalField(data, field); val.IsPositive() {
			return val
		}
	}
	return decimal.Zero
}

// loadExchangeConfigs loads exchange configurations from JSON file
func loadExchangeConfigs(configPath string) (map[string]ExchangeConfig, error) {
	file, err := os.Open(configPath)
	if err != nil {
		return nil, fmt.Errorf("opening config file: %w", err)
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var config struct {
		Exchanges []ExchangeConfig `json:"exchanges"`
	}

	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	configs := make(map[string]ExchangeConfig)
	for _, exc := range config.Exchanges {
		configs[exc.ID] = exc
	}

	return configs, nil
}
