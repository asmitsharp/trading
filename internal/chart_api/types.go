package chart_api

import (
	"time"

	"github.com/shopspring/decimal"
)

// WebSocketMessage represents a WebSocket message
type WebSocketMessage struct {
	Type      string      `json:"type"`
	Channel   string      `json:"channel,omitempty"`
	Data      interface{} `json:"data,omitempty"`
	Timestamp int64       `json:"timestamp"`
	Error     string      `json:"error,omitempty"`
}

// SubscriptionRequest represents a subscription request
type SubscriptionRequest struct {
	Type         string `json:"type"`         // "subscribe" or "unsubscribe"
	Channel      string `json:"channel"`      // "candles" or "price"
	BaseTokenID  uint32 `json:"base_token_id"`
	QuoteTokenID uint32 `json:"quote_token_id"`
	Timeframe    string `json:"timeframe,omitempty"` // for candles: "1m", "5m", etc.
}

// LivePrice represents live price data
type LivePrice struct {
	BaseTokenID       uint32          `json:"base_token_id"`
	QuoteTokenID      uint32          `json:"quote_token_id"`
	Symbol            string          `json:"symbol"`
	Price             decimal.Decimal `json:"price"`
	VWAPPrice         decimal.Decimal `json:"vwap_price"`
	Volume24h         decimal.Decimal `json:"volume_24h"`
	PriceChange24h    decimal.Decimal `json:"price_change_24h"`
	PriceChangePercent decimal.Decimal `json:"price_change_percent"`
	High24h           decimal.Decimal `json:"high_24h"`
	Low24h            decimal.Decimal `json:"low_24h"`
	ExchangeCount     uint8           `json:"exchange_count"`
	Timestamp         time.Time       `json:"timestamp"`
	DataQualityScore  decimal.Decimal `json:"data_quality_score"`
}

// LiveCandle represents a live candle update
type LiveCandle struct {
	BaseTokenID       uint32          `json:"base_token_id"`
	QuoteTokenID      uint32          `json:"quote_token_id"`
	Symbol            string          `json:"symbol"`
	Timeframe         string          `json:"timeframe"`
	Timestamp         time.Time       `json:"timestamp"`
	Open              decimal.Decimal `json:"open"`
	High              decimal.Decimal `json:"high"`
	Low               decimal.Decimal `json:"low"`
	Close             decimal.Decimal `json:"close"`
	Volume            decimal.Decimal `json:"volume"`
	QuoteVolume       decimal.Decimal `json:"quote_volume"`
	VWAPPrice         decimal.Decimal `json:"vwap_price"`
	ExchangeCount     uint8           `json:"exchange_count"`
	ContributingExchanges []string    `json:"contributing_exchanges"`
	DataQualityScore  decimal.Decimal `json:"data_quality_score"`
	TradeCount        uint32          `json:"trade_count"`
	IsComplete        bool            `json:"is_complete"` // true for completed candles, false for updating
}

// HistoricalCandleRequest represents a request for historical candle data
type HistoricalCandleRequest struct {
	BaseTokenID  uint32 `json:"base_token_id" form:"base_token_id" binding:"required"`
	QuoteTokenID uint32 `json:"quote_token_id" form:"quote_token_id" binding:"required"`
	Timeframe    string `json:"timeframe" form:"timeframe" binding:"required"`
	StartTime    int64  `json:"start_time,omitempty" form:"start_time"`
	EndTime      int64  `json:"end_time,omitempty" form:"end_time"`
	Limit        int    `json:"limit,omitempty" form:"limit"`
}

// HistoricalCandleResponse represents the response for historical candle data
type HistoricalCandleResponse struct {
	Success    bool         `json:"success"`
	Data       []LiveCandle `json:"data"`
	Symbol     string       `json:"symbol"`
	Timeframe  string       `json:"timeframe"`
	Count      int          `json:"count"`
	StartTime  int64        `json:"start_time"`
	EndTime    int64        `json:"end_time"`
	Timestamp  int64        `json:"timestamp"`
	DataSource string       `json:"data_source"` // "aggregated_exchanges"
}

// Client represents a WebSocket client connection
type Client struct {
	ID           string
	Connection   interface{} // WebSocket connection (will be *websocket.Conn)
	Subscriptions map[string]bool // subscribed channels
	LastPing     time.Time
	IsAlive      bool
}

// Subscription represents a channel subscription
type Subscription struct {
	Channel      string
	BaseTokenID  uint32
	QuoteTokenID uint32
	Timeframe    string
	Clients      map[string]*Client
}

// SupportedTimeframes lists all supported chart timeframes
var SupportedTimeframes = []string{"1m", "5m", "15m", "1h", "4h", "1d"}

// TimeframeDurations maps timeframes to their durations
var TimeframeDurations = map[string]time.Duration{
	"1m":  1 * time.Minute,
	"5m":  5 * time.Minute,
	"15m": 15 * time.Minute,
	"1h":  1 * time.Hour,
	"4h":  4 * time.Hour,
	"1d":  24 * time.Hour,
}

// MaxCandleLimits defines maximum number of candles to return for each timeframe
var MaxCandleLimits = map[string]int{
	"1m":  1440,  // 1 day
	"5m":  2016,  // 7 days 
	"15m": 2688,  // 4 weeks
	"1h":  2190,  // ~3 months
	"4h":  2190,  // ~1 year
	"1d":  1825,  // ~5 years
}

// DefaultCandleLimits defines default number of candles to return
var DefaultCandleLimits = map[string]int{
	"1m":  480,   // 8 hours
	"5m":  288,   // 24 hours
	"15m": 384,   // 4 days
	"1h":  168,   // 7 days
	"4h":  180,   // 30 days
	"1d":  365,   // 1 year
}