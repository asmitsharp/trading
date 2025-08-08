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

	for exchangeID, config := range f.configs {
		// Skip disabled exchanges
		if config.Disabled {
			f.logger.Info("Skipping disabled exchange",
				zap.String("exchange", exchangeID))
			continue
		}
		
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
	case "okx", "bitget", "gateio", "huobi":
		// These exchanges have similar response formats
		return &UnifiedParser{
			StandardParser: StandardParser{
				BaseParser: BaseParser{quoteCurrencies: quoteCurrencies},
			},
			symbolFormat: config.SymbolFormat,
		}
	case "bybit":
		// Bybit has result.list structure
		return &BybitParser{
			StandardParser: StandardParser{
				BaseParser: BaseParser{quoteCurrencies: quoteCurrencies},
			},
		}
	case "whitebit":
		// WhiteBIT returns object with symbols as keys
		return &WhiteBitParser{
			StandardParser: StandardParser{
				BaseParser: BaseParser{quoteCurrencies: quoteCurrencies},
			},
		}
	case "coinw":
		// CoinW returns data object with symbols as keys
		return &CoinWParser{
			StandardParser: StandardParser{
				BaseParser: BaseParser{quoteCurrencies: quoteCurrencies},
			},
		}
	case "bitmart":
		// BitMart returns data.tickers array
		return &BitMartParser{
			StandardParser: StandardParser{
				BaseParser: BaseParser{quoteCurrencies: quoteCurrencies},
			},
		}
	case "kucoin":
		// KuCoin returns data.ticker array
		return &KuCoinParser{
			StandardParser: StandardParser{
				BaseParser: BaseParser{quoteCurrencies: quoteCurrencies},
			},
		}
	case "pionex":
		// Pionex returns data.tickers array
		return &PionexParser{
			StandardParser: StandardParser{
				BaseParser: BaseParser{quoteCurrencies: quoteCurrencies},
			},
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

// BybitParser handles Bybit's result.list response format
type BybitParser struct {
	StandardParser
}

func (p *BybitParser) ParseTickers(data []byte, exchangeID string) ([]TickerData, error) {
	var response struct {
		RetCode int    `json:"retCode"`
		RetMsg  string `json:"retMsg"`
		Result  struct {
			List []map[string]interface{} `json:"list"`
		} `json:"result"`
	}

	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("unmarshaling bybit response: %w", err)
	}

	if response.RetCode != 0 {
		return nil, fmt.Errorf("bybit API error: %s", response.RetMsg)
	}

	tickers := make([]TickerData, 0, len(response.Result.List))
	for _, raw := range response.Result.List {
		symbol := getStringField(raw, "symbol")
		if symbol == "" {
			continue
		}

		base, quote := p.ParseSymbolPair(symbol, "BTCUSDT")

		ticker := TickerData{
			ExchangeID:     exchangeID,
			Symbol:         symbol,
			BaseSymbol:     base,
			QuoteSymbol:    quote,
			Price:          parseDecimalField(raw, "lastPrice"),
			Volume24h:      parseDecimalField(raw, "volume24h"),
			QuoteVolume24h: parseDecimalField(raw, "turnover24h"),
			PriceChange24h: parseDecimalField(raw, "price24hPcnt"),
			High24h:        parseDecimalField(raw, "highPrice24h"),
			Low24h:         parseDecimalField(raw, "lowPrice24h"),
			Timestamp:      time.Now(),
		}

		if ticker.Price.IsPositive() {
			tickers = append(tickers, ticker)
		}
	}

	return tickers, nil
}

func (p *BybitParser) ParseSymbols(data []byte, exchangeID string) ([]ExchangeSymbol, error) {
	// Bybit symbols are extracted from ticker data
	tickers, err := p.ParseTickers(data, exchangeID)
	if err != nil {
		return nil, err
	}

	symbols := make([]ExchangeSymbol, 0, len(tickers))
	for _, ticker := range tickers {
		symbols = append(symbols, ExchangeSymbol{
			ExchangeID:  exchangeID,
			Symbol:      ticker.Symbol,
			BaseSymbol:  ticker.BaseSymbol,
			QuoteSymbol: ticker.QuoteSymbol,
			IsActive:    true,
		})
	}
	return symbols, nil
}

// WhiteBitParser handles WhiteBIT's object-based response format
type WhiteBitParser struct {
	StandardParser
}

func (p *WhiteBitParser) ParseTickers(data []byte, exchangeID string) ([]TickerData, error) {
	var response map[string]map[string]interface{}

	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("unmarshaling whitebit response: %w", err)
	}

	tickers := make([]TickerData, 0, len(response))
	for symbol, raw := range response {
		// Convert symbol format from BTC_USDT to standard
		base, quote := p.ParseSymbolPair(symbol, "BTC_USDT")

		ticker := TickerData{
			ExchangeID:     exchangeID,
			Symbol:         symbol,
			BaseSymbol:     base,
			QuoteSymbol:    quote,
			Price:          parseDecimalField(raw, "last_price"),
			Volume24h:      parseDecimalField(raw, "base_volume"),
			QuoteVolume24h: parseDecimalField(raw, "quote_volume"),
			PriceChange24h: parseDecimalField(raw, "change"),
			Timestamp:      time.Now(),
		}

		if ticker.Price.IsPositive() {
			tickers = append(tickers, ticker)
		}
	}

	return tickers, nil
}

func (p *WhiteBitParser) ParseSymbols(data []byte, exchangeID string) ([]ExchangeSymbol, error) {
	// WhiteBIT symbols are extracted from ticker data
	tickers, err := p.ParseTickers(data, exchangeID)
	if err != nil {
		return nil, err
	}

	symbols := make([]ExchangeSymbol, 0, len(tickers))
	for _, ticker := range tickers {
		symbols = append(symbols, ExchangeSymbol{
			ExchangeID:  exchangeID,
			Symbol:      ticker.Symbol,
			BaseSymbol:  ticker.BaseSymbol,
			QuoteSymbol: ticker.QuoteSymbol,
			IsActive:    true,
		})
	}
	return symbols, nil
}

// CoinWParser handles CoinW's data object response format
type CoinWParser struct {
	StandardParser
}

func (p *CoinWParser) ParseTickers(data []byte, exchangeID string) ([]TickerData, error) {
	var response struct {
		Code string                            `json:"code"`
		Data map[string]map[string]interface{} `json:"data"`
	}

	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("unmarshaling coinw response: %w", err)
	}

	if response.Code != "200" {
		return nil, fmt.Errorf("coinw API error: code %s", response.Code)
	}

	tickers := make([]TickerData, 0, len(response.Data))
	for symbol, raw := range response.Data {
		// Convert symbol format from BTC_USDT to standard
		base, quote := p.ParseSymbolPair(symbol, "BTC_USDT")

		ticker := TickerData{
			ExchangeID:     exchangeID,
			Symbol:         symbol,
			BaseSymbol:     base,
			QuoteSymbol:    quote,
			Price:          parseDecimalField(raw, "last"),
			Volume24h:      parseDecimalField(raw, "baseVolume"),
			PriceChange24h: parseDecimalField(raw, "percentChange"),
			High24h:        parseDecimalField(raw, "high24hr"),
			Low24h:         parseDecimalField(raw, "low24hr"),
			Timestamp:      time.Now(),
		}

		if ticker.Price.IsPositive() {
			tickers = append(tickers, ticker)
		}
	}

	return tickers, nil
}

func (p *CoinWParser) ParseSymbols(data []byte, exchangeID string) ([]ExchangeSymbol, error) {
	// CoinW symbols are extracted from ticker data
	tickers, err := p.ParseTickers(data, exchangeID)
	if err != nil {
		return nil, err
	}

	symbols := make([]ExchangeSymbol, 0, len(tickers))
	for _, ticker := range tickers {
		symbols = append(symbols, ExchangeSymbol{
			ExchangeID:  exchangeID,
			Symbol:      ticker.Symbol,
			BaseSymbol:  ticker.BaseSymbol,
			QuoteSymbol: ticker.QuoteSymbol,
			IsActive:    true,
		})
	}
	return symbols, nil
}

// BitMartParser handles BitMart's data.tickers response format
type BitMartParser struct {
	StandardParser
}

func (p *BitMartParser) ParseTickers(data []byte, exchangeID string) ([]TickerData, error) {
	var response struct {
		Code int    `json:"code"`
		Msg  string `json:"message"`
		Data struct {
			Tickers []map[string]interface{} `json:"tickers"`
		} `json:"data"`
	}

	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("unmarshaling bitmart response: %w", err)
	}

	if response.Code != 1000 {
		return nil, fmt.Errorf("bitmart API error: %s", response.Msg)
	}

	tickers := make([]TickerData, 0, len(response.Data.Tickers))
	for _, raw := range response.Data.Tickers {
		symbol := getStringField(raw, "symbol")
		if symbol == "" {
			continue
		}

		// Convert symbol format from BTC_USDT to standard
		base, quote := p.ParseSymbolPair(symbol, "BTC_USDT")

		ticker := TickerData{
			ExchangeID:     exchangeID,
			Symbol:         symbol,
			BaseSymbol:     base,
			QuoteSymbol:    quote,
			Price:          parseDecimalField(raw, "last_price"),
			Volume24h:      parseDecimalField(raw, "base_volume_24h"),
			QuoteVolume24h: parseDecimalField(raw, "quote_volume_24h"),
			PriceChange24h: parseDecimalField(raw, "fluctuation"),
			High24h:        parseDecimalField(raw, "high_24h"),
			Low24h:         parseDecimalField(raw, "low_24h"),
			Timestamp:      time.Now(),
		}

		if ticker.Price.IsPositive() {
			tickers = append(tickers, ticker)
		}
	}

	return tickers, nil
}

func (p *BitMartParser) ParseSymbols(data []byte, exchangeID string) ([]ExchangeSymbol, error) {
	// BitMart symbols are extracted from ticker data
	tickers, err := p.ParseTickers(data, exchangeID)
	if err != nil {
		return nil, err
	}

	symbols := make([]ExchangeSymbol, 0, len(tickers))
	for _, ticker := range tickers {
		symbols = append(symbols, ExchangeSymbol{
			ExchangeID:  exchangeID,
			Symbol:      ticker.Symbol,
			BaseSymbol:  ticker.BaseSymbol,
			QuoteSymbol: ticker.QuoteSymbol,
			IsActive:    true,
		})
	}
	return symbols, nil
}

// KuCoinParser handles KuCoin's data.ticker response format
type KuCoinParser struct {
	StandardParser
}

func (p *KuCoinParser) ParseTickers(data []byte, exchangeID string) ([]TickerData, error) {
	var response struct {
		Code string `json:"code"`
		Data struct {
			Time   int64                    `json:"time"`
			Ticker []map[string]interface{} `json:"ticker"`
		} `json:"data"`
	}

	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("unmarshaling kucoin response: %w", err)
	}

	if response.Code != "200000" {
		return nil, fmt.Errorf("kucoin API error: code %s", response.Code)
	}

	tickers := make([]TickerData, 0, len(response.Data.Ticker))
	for _, raw := range response.Data.Ticker {
		symbol := getStringField(raw, "symbol")
		if symbol == "" {
			continue
		}

		// Convert symbol format from BTC-USDT to standard
		base, quote := p.ParseSymbolPair(symbol, "BTC-USDT")

		ticker := TickerData{
			ExchangeID:     exchangeID,
			Symbol:         symbol,
			BaseSymbol:     base,
			QuoteSymbol:    quote,
			Price:          parseDecimalField(raw, "last"),
			Volume24h:      parseDecimalField(raw, "vol"),
			QuoteVolume24h: parseDecimalField(raw, "volValue"),
			PriceChange24h: parseDecimalField(raw, "changeRate"),
			High24h:        parseDecimalField(raw, "high"),
			Low24h:         parseDecimalField(raw, "low"),
			Timestamp:      time.Now(),
		}

		if ticker.Price.IsPositive() {
			tickers = append(tickers, ticker)
		}
	}

	return tickers, nil
}

func (p *KuCoinParser) ParseSymbols(data []byte, exchangeID string) ([]ExchangeSymbol, error) {
	// KuCoin symbols are extracted from ticker data
	tickers, err := p.ParseTickers(data, exchangeID)
	if err != nil {
		return nil, err
	}

	symbols := make([]ExchangeSymbol, 0, len(tickers))
	for _, ticker := range tickers {
		symbols = append(symbols, ExchangeSymbol{
			ExchangeID:  exchangeID,
			Symbol:      ticker.Symbol,
			BaseSymbol:  ticker.BaseSymbol,
			QuoteSymbol: ticker.QuoteSymbol,
			IsActive:    true,
		})
	}
	return symbols, nil
}

// PionexParser handles Pionex's data.tickers response format
type PionexParser struct {
	StandardParser
}

func (p *PionexParser) ParseTickers(data []byte, exchangeID string) ([]TickerData, error) {
	var response struct {
		Result bool `json:"result"`
		Data   struct {
			Tickers []map[string]interface{} `json:"tickers"`
		} `json:"data"`
	}

	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("unmarshaling pionex response: %w", err)
	}

	if !response.Result {
		return nil, fmt.Errorf("pionex API error: result false")
	}

	tickers := make([]TickerData, 0, len(response.Data.Tickers))
	for _, raw := range response.Data.Tickers {
		symbol := getStringField(raw, "symbol")
		if symbol == "" {
			continue
		}

		// Convert symbol format from BTC_USDT to standard
		base, quote := p.ParseSymbolPair(symbol, "BTC_USDT")

		ticker := TickerData{
			ExchangeID:     exchangeID,
			Symbol:         symbol,
			BaseSymbol:     base,
			QuoteSymbol:    quote,
			Price:          parseDecimalField(raw, "close"),
			Volume24h:      parseDecimalField(raw, "volume"),
			QuoteVolume24h: parseDecimalField(raw, "amount"),
			High24h:        parseDecimalField(raw, "high"),
			Low24h:         parseDecimalField(raw, "low"),
			Timestamp:      time.Now(),
		}

		if ticker.Price.IsPositive() {
			tickers = append(tickers, ticker)
		}
	}

	return tickers, nil
}

func (p *PionexParser) ParseSymbols(data []byte, exchangeID string) ([]ExchangeSymbol, error) {
	// Pionex symbols are extracted from ticker data
	tickers, err := p.ParseTickers(data, exchangeID)
	if err != nil {
		return nil, err
	}

	symbols := make([]ExchangeSymbol, 0, len(tickers))
	for _, ticker := range tickers {
		symbols = append(symbols, ExchangeSymbol{
			ExchangeID:  exchangeID,
			Symbol:      ticker.Symbol,
			BaseSymbol:  ticker.BaseSymbol,
			QuoteSymbol: ticker.QuoteSymbol,
			IsActive:    true,
		})
	}
	return symbols, nil
}
