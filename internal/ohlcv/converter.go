package ohlcv

import (
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ashmitsharp/trading/internal/volume_calculator"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// Converter handles OHLCV conversion and aggregation
type Converter struct {
	clickhouseDB     clickhouse.Conn
	logger           *zap.Logger
	volumeCalculator *volume_calculator.VolumeCalculator
}

// NewConverter creates a new OHLCV converter
func NewConverter(db clickhouse.Conn, logger *zap.Logger) *Converter {
	// Initialize volume calculator
	volumeConfig := volume_calculator.CalculationConfig{
		MaxBufferSize:      1440,                           // 1440 minutes = 24 hours
		FallbackWindow:     7,                              // Use 7 recent candles for fallback
		TimestampTolerance: 30 * time.Second,              // 30 second tolerance
		MinValidVolume:     decimal.NewFromFloat(0.000001), // Minimum valid volume
		MaxValidVolume:     decimal.NewFromFloat(1e12),     // Maximum valid volume
	}

	return &Converter{
		clickhouseDB:     db,
		logger:           logger,
		volumeCalculator: volume_calculator.NewVolumeCalculator(volumeConfig, logger),
	}
}
