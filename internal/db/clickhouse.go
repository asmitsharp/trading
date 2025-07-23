package db

import (
	"context"
	"fmt"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/ashmitsharp/trading/internal/config"
)

// InitClickHouse initializes ClickHouse connection and creates necessary tables
func InitClickHouse(cfg config.ClickhouseConfig) (driver.Conn, error) {
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)},
		Auth: clickhouse.Auth{
			Database: cfg.Database,
			Username: cfg.Username,
			Password: cfg.Password,
		},
		Debug: cfg.Debug,
		Debugf: func(format string, v ...interface{}) {
			if cfg.Debug {
				fmt.Printf("[ClickHouse Debug] "+format+"\n", v...)
			}
		},
		Settings: clickhouse.Settings{
			"max_execution_time": 60,
		},
		Compression: &clickhouse.Compression{
			Method: clickhouse.CompressionLZ4,
		},
	})

	if err != nil {
		return nil, fmt.Errorf("failed to connect to ClickHouse: %w", err)
	}

	// Test connection
	if err := conn.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to ping ClickHouse: %w", err)
	}

	return conn, nil
}

// CreateClickHouseTables creates the required ClickHouse tables
func CreateClickHouseTables(conn driver.Conn) error {
	ctx := context.Background()

	// Create trades table with optimized schema for time-series data
	tradesTableSQL := `
		CREATE TABLE IF NOT EXISTS trades (
			symbol       LowCardinality(String),
			price        Decimal(20, 8),
			quantity     Decimal(20, 8),
			trade_id     UInt64,
			timestamp    DateTime64(3, 'UTC'),
			is_buyer_maker UInt8
		) ENGINE = MergeTree()
		PARTITION BY symbol
		ORDER BY (symbol, timestamp)
		SETTINGS index_granularity = 8192
	`

	if err := conn.Exec(ctx, tradesTableSQL); err != nil {
		return fmt.Errorf("failed to create trades table: %w", err)
	}

	// Create materialized view for OHLCV data (1-minute intervals)
	ohlcvViewSQL := `
		CREATE MATERIALIZED VIEW IF NOT EXISTS trades_ohlcv_1m
		ENGINE = AggregatingMergeTree()
		PARTITION BY symbol
		ORDER BY (symbol, minute)
		AS SELECT
			symbol,
			toStartOfMinute(timestamp) as minute,
			argMinState(price, timestamp) as open,
			maxState(price) as high,
			minState(price) as low,
			argMaxState(price, timestamp) as close,
			sumState(quantity) as volume,
			countState() as trades_count
		FROM trades
		GROUP BY symbol, minute
	`

	if err := conn.Exec(ctx, ohlcvViewSQL); err != nil {
		return fmt.Errorf("failed to create OHLCV materialized view: %w", err)
	}

	return nil
}

// InsertTrades inserts trade data into ClickHouse in batches
func InsertTrades(conn driver.Conn, trades []TradeData) error {
	if len(trades) == 0 {
		return nil
	}

	ctx := context.Background()

	batch, err := conn.PrepareBatch(ctx, "INSERT INTO trades")
	if err != nil {
		return fmt.Errorf("failed to prepare batch: %w", err)
	}

	for _, trade := range trades {
		if err := batch.Append(
			trade.Symbol,
			trade.Price,
			trade.Quantity,
			trade.TradeID,
			trade.Timestamp,
			trade.IsBuyerMaker,
		); err != nil {
			return fmt.Errorf("failed to append trade to batch: %w", err)
		}
	}

	if err := batch.Send(); err != nil {
		return fmt.Errorf("failed to send batch: %w", err)
	}

	return nil
}

// TradeData represents a single trade record
type TradeData struct {
	Symbol       string
	Price        float64
	Quantity     float64
	TradeID      uint64
	Timestamp    int64
	IsBuyerMaker uint8
}

// GetLatestPrices gets the latest price for each symbol
func GetLatestPrices(conn driver.Conn) (map[string]LatestPrice, error) {
	ctx := context.Background()

	query := `
		SELECT 
			symbol,
			anyLast(price) as price,
			anyLast(timestamp) as timestamp,
			anyLast(quantity) as volume
		FROM trades
		GROUP BY symbol
	`

	rows, err := conn.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query latest prices: %w", err)
	}
	defer rows.Close()

	prices := make(map[string]LatestPrice)

	for rows.Next() {
		var symbol string
		var price float64
		var timestamp int64
		var volume float64

		if err := rows.Scan(&symbol, &price, &timestamp, &volume); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		prices[symbol] = LatestPrice{
			Symbol:    symbol,
			Price:     price,
			Timestamp: timestamp,
			Volume:    volume,
		}
	}

	return prices, nil
}

// LatestPrice represents the latest price data for a symbol
type LatestPrice struct {
	Symbol    string  `json:"symbol"`
	Price     float64 `json:"price"`
	Timestamp int64   `json:"timestamp"`
	Volume    float64 `json:"volume"`
}

// GetOHLCVData gets OHLCV data for a symbol within a time range
func GetOHLCVData(conn driver.Conn, symbol string, fromTime, toTime int64, interval string) ([]OHLCVData, error) {
	ctx := context.Background()

	var query string
	switch interval {
	case "1m":
		query = `
			SELECT 
				symbol,
				minute,
				argMinMerge(open) as open,
				maxMerge(high) as high,
				minMerge(low) as low,
				argMaxMerge(close) as close,
				sumMerge(volume) as volume,
				countMerge(trades_count) as trades_count
			FROM trades_ohlcv_1m
			WHERE symbol = ? AND minute >= toDateTime64(?, 3) AND minute <= toDateTime64(?, 3)
			GROUP BY symbol, minute
			ORDER BY minute
		`
	default:
		// For other intervals, aggregate from trades table directly
		query = `
			SELECT 
				symbol,
				toStartOfInterval(timestamp, INTERVAL ? MINUTE) as interval_start,
				argMin(price, timestamp) as open,
				max(price) as high,
				min(price) as low,
				argMax(price, timestamp) as close,
				sum(quantity) as volume,
				count() as trades_count
			FROM trades
			WHERE symbol = ? AND timestamp >= toDateTime64(?, 3) AND timestamp <= toDateTime64(?, 3)
			GROUP BY symbol, interval_start
			ORDER BY interval_start
		`
	}

	var rows driver.Rows
	var err error

	if interval == "1m" {
		rows, err = conn.Query(ctx, query, symbol, fromTime, toTime)
	} else {
		intervalMinutes := parseInterval(interval)
		rows, err = conn.Query(ctx, query, intervalMinutes, symbol, fromTime, toTime)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to query OHLCV data: %w", err)
	}
	defer rows.Close()

	var data []OHLCVData

	for rows.Next() {
		var ohlcv OHLCVData
		var timestamp int64

		if err := rows.Scan(&ohlcv.Symbol, &timestamp, &ohlcv.Open, &ohlcv.High,
			&ohlcv.Low, &ohlcv.Close, &ohlcv.Volume, &ohlcv.TradesCount); err != nil {
			return nil, fmt.Errorf("failed to scan OHLCV row: %w", err)
		}

		ohlcv.Timestamp = timestamp
		data = append(data, ohlcv)
	}

	return data, nil
}

// OHLCVData represents OHLCV candlestick data
type OHLCVData struct {
	Symbol      string  `json:"symbol"`
	Timestamp   int64   `json:"timestamp"`
	Open        float64 `json:"open"`
	High        float64 `json:"high"`
	Low         float64 `json:"low"`
	Close       float64 `json:"close"`
	Volume      float64 `json:"volume"`
	TradesCount int64   `json:"trades_count"`
}

// parseInterval converts interval string to minutes
func parseInterval(interval string) int {
	switch interval {
	case "5m":
		return 5
	case "15m":
		return 15
	case "1h":
		return 60
	case "4h":
		return 240
	case "1d":
		return 1440
	default:
		return 1
	}
}
