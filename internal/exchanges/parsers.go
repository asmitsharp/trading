package exchanges

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

// StandardParser handles exchanges with standard JSON array responses
type StandardParser struct {
	BaseParser
}

// NewStandardParser creates a parser for standard exchange formats
func NewStandardParser(quoteCurrencies []string) *StandardParser {
	return &StandardParser{
		BaseParser: BaseParser{
			quoteCurrencies: quoteCurrencies,
		},
	}
}

// BinanceStyleParser handles Binance-style responses
type BinanceStyleParser struct {
	StandardParser
}

func (p *BinanceStyleParser) ParseTickers(data []byte, exchangeID string) ([]TickerData, error) {
	var rawTickers []map[string]interface{}
	if err := json.Unmarshal(data, &rawTickers); err != nil {
		return nil, fmt.Errorf("unmarshaling tickers: %w", err)
	}

	tickers := make([]TickerData, 0, len(rawTickers))
	for _, raw := range rawTickers {
		ticker := p.parseTickerMap(raw, exchangeID)
		if ticker.Price.IsPositive() {
			tickers = append(tickers, ticker)
		}
	}

	return tickers, nil
}

func (p *BinanceStyleParser) parseTickerMap(data map[string]interface{}, exchangeID string) TickerData {
	symbol := getStringField(data, "symbol")
	base, quote := p.ParseSymbolPair(symbol, "BTCUSDT")

	return TickerData{
		ExchangeID:     exchangeID,
		Symbol:         symbol,
		BaseSymbol:     base,
		QuoteSymbol:    quote,
		Price:          parseDecimalField(data, "lastPrice"),
		Volume24h:      parseDecimalField(data, "volume"),
		QuoteVolume24h: parseDecimalField(data, "quoteVolume"),
		PriceChange24h: parseDecimalField(data, "priceChange"),
		High24h:        parseDecimalField(data, "highPrice"),
		Low24h:         parseDecimalField(data, "lowPrice"),
		Timestamp:      time.Now(),
	}
}

func (p *BinanceStyleParser) ParseSymbols(data []byte, exchangeID string) ([]ExchangeSymbol, error) {
	var response struct {
		Symbols []struct {
			Symbol     string `json:"symbol"`
			Status     string `json:"status"`
			BaseAsset  string `json:"baseAsset"`
			QuoteAsset string `json:"quoteAsset"`
		} `json:"symbols"`
	}

	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("unmarshaling symbols: %w", err)
	}

	symbols := make([]ExchangeSymbol, 0, len(response.Symbols))
	for _, s := range response.Symbols {
		if s.Status == "TRADING" {
			symbols = append(symbols, ExchangeSymbol{
				ExchangeID:  exchangeID,
				Symbol:      s.Symbol,
				BaseSymbol:  s.BaseAsset,
				QuoteSymbol: s.QuoteAsset,
				IsActive:    true,
			})
		}
	}

	return symbols, nil
}

// CoinbaseStyleParser handles Coinbase-style responses
type CoinbaseStyleParser struct {
	StandardParser
}

func (p *CoinbaseStyleParser) ParseTickers(data []byte, exchangeID string) ([]TickerData, error) {
	var products []map[string]interface{}
	if err := json.Unmarshal(data, &products); err != nil {
		return nil, fmt.Errorf("unmarshaling products: %w", err)
	}

	tickers := make([]TickerData, 0, len(products))
	for _, product := range products {
		if getStringField(product, "status") != "online" {
			continue
		}

		// Safely check if stats exists and is a map
		statsInterface, ok := product["stats"]
		if !ok || statsInterface == nil {
			continue
		}
		stats, ok := statsInterface.(map[string]interface{})
		if !ok {
			continue
		}

		symbol := getStringField(product, "id")
		base, quote := p.ParseSymbolPair(symbol, "BTC-USD")

		ticker := TickerData{
			ExchangeID:     exchangeID,
			Symbol:         symbol,
			BaseSymbol:     base,
			QuoteSymbol:    quote,
			Price:          parseDecimalField(stats, "last"),
			Volume24h:      parseDecimalField(stats, "volume"),
			QuoteVolume24h: parseDecimalField(stats, "volume_30day"),
			High24h:        parseDecimalField(stats, "high"),
			Low24h:         parseDecimalField(stats, "low"),
			Timestamp:      time.Now(),
		}

		if ticker.Price.IsPositive() {
			tickers = append(tickers, ticker)
		}
	}

	return tickers, nil
}

func (p *CoinbaseStyleParser) ParseSymbols(data []byte, exchangeID string) ([]ExchangeSymbol, error) {
	var products []struct {
		ID             string `json:"id"`
		BaseCurrency   string `json:"base_currency"`
		QuoteCurrency  string `json:"quote_currency"`
		Status         string `json:"status"`
		MinMarketFunds string `json:"min_market_funds"`
		MinSize        string `json:"min_size"`
	}

	if err := json.Unmarshal(data, &products); err != nil {
		return nil, fmt.Errorf("unmarshaling products: %w", err)
	}

	symbols := make([]ExchangeSymbol, 0, len(products))
	for _, p := range products {
		if p.Status == "online" {
			symbols = append(symbols, ExchangeSymbol{
				ExchangeID:  exchangeID,
				Symbol:      p.ID,
				BaseSymbol:  p.BaseCurrency,
				QuoteSymbol: p.QuoteCurrency,
				IsActive:    true,
				MinQuantity: p.MinSize,
				MinNotional: p.MinMarketFunds,
			})
		}
	}

	return symbols, nil
}

// KrakenStyleParser handles Kraken-style responses
type KrakenStyleParser struct {
	StandardParser
}

func (p *KrakenStyleParser) ParseTickers(data []byte, exchangeID string) ([]TickerData, error) {
	var response struct {
		Error  []string               `json:"error"`
		Result map[string]interface{} `json:"result"`
	}

	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("unmarshaling response: %w", err)
	}

	if len(response.Error) > 0 {
		return nil, fmt.Errorf("kraken API error: %v", response.Error)
	}

	tickers := make([]TickerData, 0)
	for symbol, data := range response.Result {
		tickerData, ok := data.(map[string]interface{})
		if !ok {
			continue
		}

		base, quote := p.ParseSymbolPair(symbol, "XXBTZUSD")
		
		ticker := TickerData{
			ExchangeID:  exchangeID,
			Symbol:      symbol,
			BaseSymbol:  base,
			QuoteSymbol: quote,
			Price:       parseArrayField(tickerData, "c", 0), // close price
			Volume24h:   parseArrayField(tickerData, "v", 1), // volume last 24h
			High24h:     parseArrayField(tickerData, "h", 1), // high last 24h
			Low24h:      parseArrayField(tickerData, "l", 1), // low last 24h
			Timestamp:   time.Now(),
		}

		if ticker.Price.IsPositive() {
			tickers = append(tickers, ticker)
		}
	}

	return tickers, nil
}

func (p *KrakenStyleParser) ParseSymbols(data []byte, exchangeID string) ([]ExchangeSymbol, error) {
	var response struct {
		Error  []string                       `json:"error"`
		Result map[string]map[string]interface{} `json:"result"`
	}

	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("unmarshaling response: %w", err)
	}

	symbols := make([]ExchangeSymbol, 0)
	for symbol, info := range response.Result {
		status := getStringField(info, "status")
		if status == "online" {
			base, quote := p.ParseSymbolPair(symbol, "XXBTZUSD")
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

// Helper functions for parsing fields
func getStringField(data map[string]interface{}, field string) string {
	if val, ok := data[field]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

func parseDecimalField(data map[string]interface{}, field string) decimal.Decimal {
	if val, ok := data[field]; ok {
		return parseDecimalSafe(val)
	}
	return decimal.Zero
}

func parseArrayField(data map[string]interface{}, field string, index int) decimal.Decimal {
	if val, ok := data[field]; ok {
		if arr, ok := val.([]interface{}); ok && len(arr) > index {
			return parseDecimalSafe(arr[index])
		}
	}
	return decimal.Zero
}