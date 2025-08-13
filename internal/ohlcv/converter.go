package ohlcv

import (
	"github.com/ClickHouse/clickhouse-go/v2"
	"go.uber.org/zap"
)

// Converter handles OHLCV conversion and aggregation
type Converter struct {
	clickhouseDB clickhouse.Conn
	logger       *zap.Logger
}

// NewConverter creates a new OHLCV converter
func NewConverter(db clickhouse.Conn, logger *zap.Logger) *Converter {
	return &Converter{
		clickhouseDB: db,
		logger:       logger,
	}
}
