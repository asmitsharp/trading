package models

import "time"

type Trade struct {
	Symbol       string    `json:"symbol" db:"symbol"`
	Price        float64   `json:"price" db:"price"`
	Quantity     float64   `json:"quantity" db:"quantity"`
	TradeID      uint64    `json:"trade_id" db:"trade_id"`
	Timestamp    time.Time `json:"timestamp" db:"timestamp"`
	IsBuyerMaker bool      `json:"is_buyer_maker" db:"is_buyer_maker"`
}

type BinanceTradeEvent struct {
	EventType     string `json:"e"`
	EventTime     string `json:"E"`
	Symbol        string `json:"s"`
	TradeID       int64  `json:"t"`
	Price         string `json:"p"`
	Quantity      string `json:"q"`
	BuyerOrderID  int64  `json:"b"`
	SellerOrderID int64  `json:"a"`
	TradeTime     int64  `json:"T"`
	IsBuyerMaker  bool   `json:"m"`
	Ignore        bool   `json:"M"`
}

type BinanceCombinedStreamEvent struct {
	Stream string            `json:"stream"`
	Data   BinanceTradeEvent `json:"data"`
}

type TickerResponse struct {
	Symbol                string  `json:"symbol"`
	Price                 float64 `json:"price"`
	PriceChange24h        float64 `json:"price_change_24h,omitempty"`
	PriceChangePercent24h float64 `json:"price_change_percent_24h,omitempty"`
	Volume24h             float64 `json:"volume_24h,omitempty"`
	High24h               float64 `json:"high_24h,omitempty"`
	Low24h                float64 `json:"low_24h,omitempty"`
	Timestamp             int64   `json:"timestamp"`
	Name                  string  `json:"name,omitempty"`
	Category              string  `json:"category,omitempty"`
}

type OHLCVResponse struct {
	Symbol      string  `json:"symbol"`
	Interval    string  `json:"interval"`
	Timestamp   int64   `json:"timestamp"`
	Open        float64 `json:"open"`
	High        float64 `json:"high"`
	Low         float64 `json:"low"`
	Close       float64 `json:"close"`
	Volume      float64 `json:"volume"`
	TradesCount int64   `json:"trades_count"`
}

type APIResponse struct {
	Success   bool        `json:"success"`
	Data      interface{} `json:"data,omitempty"`
	Error     string      `json:"error,omitempty"`
	Message   string      `json:"message,omitempty"`
	Timestamp int64       `json:"timestamp"`
}

type ErrorResponse struct {
	Error     string `json:"error"`
	Message   string `json:"message"`
	Code      int    `json:"code"`
	Timestamp int64  `json:"timestamp"`
}

type HealthResponse struct {
	Status    string `json:"status"`
	Version   string `json:"version"`
	Timestamp int64  `json:"timestamp"`
	Uptime    int64  `json:"uptime,omitempty"`
}

type TradeStats struct {
	Symbol         string  `json:"symbol"`
	TotalTrades    int64   `json:"total_trades"`
	TotalVolume    float64 `json:"total_volume"`
	AvgPrice       float64 `json:"avg_price"`
	MinPrice       float64 `json:"min_price"`
	MaxPrice       float64 `json:"max_price"`
	FirstTradeTime int64   `json:"first_trade_time"`
	LastTradeTime  int64   `json:"last_trade_time"`
}

type MarketSummary struct {
	TotalSymbols   int     `json:"total_symbols"`
	TotalTrades24h int64   `json:"total_trades_24h"`
	TotalVolume24h float64 `json:"total_volume_24h"`
	ActiveSymbols  int     `json:"active_symbols"`
	LastUpdateTime int64   `json:"last_update_time"`
}
