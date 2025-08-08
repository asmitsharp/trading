package exchanges

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
)

// ExchangeClient interface for all exchange REST clients
type ExchangeClient interface {
	GetName() string
	GetID() string
	GetWeight() float64
	GetTickers(ctx context.Context, symbols []string) ([]TickerData, error)
	GetAllTickers(ctx context.Context) ([]TickerData, error)
	GetSymbols(ctx context.Context) ([]ExchangeSymbol, error)
	GetRateLimit() time.Duration
	IsHealthy() bool
	UpdateHealth(success bool, responseTime time.Duration)
}

// TickerData represents unified ticker data from any exchange
type TickerData struct {
	ExchangeID     string          `json:"exchange_id"`
	Symbol         string          `json:"symbol"`
	BaseSymbol     string          `json:"base_symbol"`
	QuoteSymbol    string          `json:"quote_symbol"`
	BaseTokenID    int             `json:"base_token_id"`    // Added token ID
	QuoteTokenID   int             `json:"quote_token_id"`   // Added token ID
	Price          decimal.Decimal `json:"price"`
	Volume24h      decimal.Decimal `json:"volume_24h"`
	QuoteVolume24h decimal.Decimal `json:"quote_volume_24h"`
	PriceChange24h decimal.Decimal `json:"price_change_24h"`
	High24h        decimal.Decimal `json:"high_24h"`
	Low24h         decimal.Decimal `json:"low_24h"`
	Timestamp      time.Time       `json:"timestamp"`
}

// ExchangeSymbol represents a trading pair on an exchange
type ExchangeSymbol struct {
	ExchangeID  string `json:"exchange_id"`
	Symbol      string `json:"symbol"`
	BaseSymbol  string `json:"base_symbol"`
	QuoteSymbol string `json:"quote_symbol"`
	IsActive    bool   `json:"is_active"`
	MinQuantity string `json:"min_quantity"`
	MinNotional string `json:"min_notional"`
}

// ExchangeConfig represents configuration for an exchange
type ExchangeConfig struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	BaseURL            string   `json:"base_url"`
	TickerEndpoint     string   `json:"ticker_endpoint"`
	SymbolsEndpoint    string   `json:"symbols_endpoint"`
	RateLimitPerMinute int      `json:"rate_limit_per_minute"`
	Weight             float64  `json:"weight"`
	RequestTimeout     int      `json:"request_timeout"`
	RetryAttempts      int      `json:"retry_attempts"`
	SymbolFormat       string   `json:"symbol_format"`
	QuoteCurrencies    []string `json:"quote_currencies"`
	Disabled           bool     `json:"disabled"`
}

// Health represents exchange health status
type Health struct {
	IsHealthy          bool
	LastSuccessfulPoll time.Time
	ConsecutiveErrors  int
	AverageResponseMs  int64
}