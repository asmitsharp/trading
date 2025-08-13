package ohlcv

import (
	"time"

	"github.com/shopspring/decimal"
)

// Timeframe represents OHLCV timeframe
type Timeframe string

const (
	Timeframe1m  Timeframe = "1m"
	Timeframe5m  Timeframe = "5m"
	Timeframe15m Timeframe = "15m"
	Timeframe1h  Timeframe = "1h"
	Timeframe4h  Timeframe = "4h"
	Timeframe1d  Timeframe = "1d"
	Timeframe1w  Timeframe = "1w"
)

// Candle represents OHLCV data
type Candle struct {
	Timestamp    time.Time
	BaseTokenID  uint32
	QuoteTokenID uint32
	ExchangeID   string
	Open         decimal.Decimal
	High         decimal.Decimal
	Low          decimal.Decimal
	Close        decimal.Decimal
	Volume       decimal.Decimal
	QuoteVolume  decimal.Decimal
	TradeCount   uint32
	VWAPPrice    decimal.Decimal
	Version      uint64
}