package volume_calculator

import (
	"time"

	"github.com/shopspring/decimal"
)

// VolumeSnapshot represents a 24h volume snapshot at a specific time
type VolumeSnapshot struct {
	Timestamp    time.Time
	Symbol       string
	Volume24h    decimal.Decimal
	ExchangeID   string
	BaseTokenID  uint32
	QuoteTokenID uint32
}

// OHLCVCandle represents a 1-minute OHLCV candle
type OHLCVCandle struct {
	Timestamp    time.Time
	Symbol       string
	ExchangeID   string
	BaseTokenID  uint32
	QuoteTokenID uint32
	Open         decimal.Decimal
	High         decimal.Decimal
	Low          decimal.Decimal
	Close        decimal.Decimal
	Volume       decimal.Decimal // The actual 1-minute volume
	QuoteVolume  decimal.Decimal
	TradeCount   uint32
}

// VolumeCalculationResult represents the result of 1-minute volume calculation
type VolumeCalculationResult struct {
	Timestamp        time.Time
	Symbol           string
	ExchangeID       string
	BaseTokenID      uint32
	QuoteTokenID     uint32
	CalculatedVolume decimal.Decimal
	Method           string // "standard", "fallback_average", "simple_diff"
	IsValid          bool
	ErrorMessage     string
	InputData        VolumeInputData
}

// VolumeInputData stores the input data used for calculation
type VolumeInputData struct {
	Current24hVolume  decimal.Decimal
	Previous24hVolume decimal.Decimal
	Volume1440MinAgo  decimal.Decimal
	BufferSize        int
	FallbackAverage   decimal.Decimal
	UsedFallback      bool
}

// CandleBuffer manages a circular buffer of OHLCV candles
type CandleBuffer struct {
	candles   []OHLCVCandle
	capacity  int
	size      int
	writePos  int
	symbol    string
	exchangeID string
}

// CalculationConfig holds configuration for volume calculations
type CalculationConfig struct {
	MaxBufferSize      int           // Maximum candles to keep in buffer (default: 1440)
	FallbackWindow     int           // Number of recent candles for fallback average (5-10)
	TimestampTolerance time.Duration // Tolerance for timestamp alignment (default: 30s)
	MinValidVolume     decimal.Decimal // Minimum volume to consider valid
	MaxValidVolume     decimal.Decimal // Maximum volume to consider valid
}