package main

import (
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ashmitsharp/trading/internal/currency"
	"github.com/ashmitsharp/trading/internal/reliability"
	"github.com/ashmitsharp/trading/internal/volume_calculator"
	_ "github.com/lib/pq"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// CandleData represents a single candle from CSV
type CandleData struct {
	DateTime    time.Time
	Exchange    string
	Symbol      string
	BaseSymbol  string
	QuoteSymbol string
	Open        decimal.Decimal
	High        decimal.Decimal
	Low         decimal.Decimal
	Close       decimal.Decimal
	Volume      decimal.Decimal
	QuoteVolume decimal.Decimal
	Timeframe   string // Track the timeframe
}

// AggregatedCandle represents cross-exchange aggregated OHLCV (CoinMarketCap style)
type AggregatedCandle struct {
	Timestamp        time.Time
	BaseTokenID      uint32
	QuoteTokenID     uint32
	Open             decimal.Decimal
	High             decimal.Decimal
	Low              decimal.Decimal
	Close            decimal.Decimal
	Volume           decimal.Decimal
	QuoteVolume      decimal.Decimal
	VWAPPrice        decimal.Decimal
	ExchangeCount    uint8
	ContribExchanges []string
	DataQualityScore decimal.Decimal
	TradeCount       uint32
}

// TokenMapping handles symbol to token ID mapping
type TokenMapping struct {
	postgres *sql.DB
	cache    map[string]uint32
}

// Importer handles the CSV import and aggregation process
type Importer struct {
	postgres         *sql.DB
	clickhouse       clickhouse.Conn
	tokenMap         *TokenMapping
	converter        *currency.Converter
	scorer           *reliability.Scorer
	volumeCalculator *volume_calculator.VolumeCalculator
	logger           *zap.Logger
}

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Usage: go run main.go <timeframe>  (e.g., go run main.go 1m)")
	}

	timeframe := os.Args[1]

	// Get the directory where the script is running from
	// Navigate from trading/cmd/import-historical-ohlcv to find exchange-scripts
	currentDir, err := os.Getwd()
	if err != nil {
		log.Fatal("Failed to get current directory:", err)
	}

	// Navigate up from cmd/import-historical-ohlcv to trading, then to parent
	// Current: .../trading/cmd/import-historical-ohlcv
	// Go up to: .../trading/cmd
	cmdDir := filepath.Dir(currentDir)
	// Go up to: .../trading
	tradingDir := filepath.Dir(cmdDir)
	// Go up to parent directory that contains both trading and exchange-scripts
	parentDir := filepath.Dir(tradingDir)
	// Now get exchange-scripts
	exchangeScriptsDir := filepath.Join(parentDir, "exchange-scripts")

	// Check if exchange-scripts directory exists
	if _, err := os.Stat(exchangeScriptsDir); os.IsNotExist(err) {
		log.Fatal("exchange-scripts directory not found at:", exchangeScriptsDir)
	}

	// Initialize logger
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	logger.Info("Starting import",
		zap.String("timeframe", timeframe),
		zap.String("exchange_scripts_dir", exchangeScriptsDir))

	// Database connections
	postgres, err := sql.Open("postgres",
		"postgres://username:password@localhost/trading?sslmode=disable")
	if err != nil {
		logger.Fatal("Failed to connect to PostgreSQL", zap.Error(err))
	}
	defer postgres.Close()

	clickhouse, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{"localhost:9000"},
		Auth: clickhouse.Auth{
			Database: "trading",
		},
	})
	if err != nil {
		logger.Fatal("Failed to connect to ClickHouse", zap.Error(err))
	}
	defer clickhouse.Close()

	// Initialize volume calculator
	volumeConfig := volume_calculator.CalculationConfig{
		MaxBufferSize:      1440,
		FallbackWindow:     7,
		TimestampTolerance: 30 * time.Second,
		MinValidVolume:     decimal.NewFromFloat(0.000001),
		MaxValidVolume:     decimal.NewFromFloat(1e12),
	}

	// Initialize components
	importer := &Importer{
		postgres:         postgres,
		clickhouse:       clickhouse,
		tokenMap:         NewTokenMapping(postgres),
		converter:        currency.NewConverter(logger),
		scorer:           reliability.NewScorer(logger),
		volumeCalculator: volume_calculator.NewVolumeCalculator(volumeConfig, logger),
		logger:           logger,
	}

	ctx := context.Background()

	// Initialize currency rates
	logger.Info("Initializing currency converter...")
	if _, err := importer.converter.GetUSDRate(ctx, "EUR"); err != nil {
		logger.Warn("Failed to initialize currency rates", zap.Error(err))
	}

	// Process all directories for the specified timeframe
	logger.Info("Processing historical OHLCV import",
		zap.String("timeframe", timeframe))

	if err := importer.ProcessTimeframeDirectories(ctx, exchangeScriptsDir, timeframe); err != nil {
		logger.Fatal("Failed to process timeframe directories", zap.Error(err))
	}

	logger.Info("Historical OHLCV import completed successfully!")
}

// ProcessTimeframeDirectories processes all directories matching the timeframe pattern
func (i *Importer) ProcessTimeframeDirectories(ctx context.Context, baseDir string, timeframe string) error {
	// Find all directories that contain the timeframe
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return fmt.Errorf("failed to read directory %s: %w", baseDir, err)
	}

	var matchingDirs []string

	// Look for directories like data_1m, data_binance_1m, data_kraken_1m, etc.
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dirName := entry.Name()
		// Check if directory name starts with "data" and contains the timeframe
		if strings.HasPrefix(dirName, "data") && strings.Contains(dirName, timeframe) {
			// Verify it ends with the timeframe to avoid false matches
			// (e.g., we don't want data_15m when looking for 5m)
			if strings.HasSuffix(dirName, timeframe) {
				matchingDirs = append(matchingDirs, filepath.Join(baseDir, dirName))
			}
		}
	}

	if len(matchingDirs) == 0 {
		return fmt.Errorf("no directories found for timeframe %s in %s", timeframe, baseDir)
	}

	i.logger.Info("Found directories for timeframe",
		zap.String("timeframe", timeframe),
		zap.Int("count", len(matchingDirs)),
		zap.Strings("directories", matchingDirs))

	// Collect all candles from all matching directories
	var allCandles []CandleData

	for _, dir := range matchingDirs {
		i.logger.Info("Processing directory", zap.String("directory", dir))

		candles, err := i.collectAllCandlesFromDirectory(ctx, dir, timeframe)
		if err != nil {
			i.logger.Error("Failed to process directory",
				zap.String("directory", dir),
				zap.Error(err))
			continue
		}

		allCandles = append(allCandles, candles...)

		i.logger.Info("Collected candles from directory",
			zap.String("directory", dir),
			zap.Int("candle_count", len(candles)))
	}

	if len(allCandles) == 0 {
		return fmt.Errorf("no candles found across all directories for timeframe %s", timeframe)
	}

	i.logger.Info("Total candles collected across all exchanges",
		zap.String("timeframe", timeframe),
		zap.Int("total_candles", len(allCandles)))

	// Aggregate candles across all exchanges
	aggregatedCandles, err := i.aggregateCandles(ctx, allCandles)
	if err != nil {
		return fmt.Errorf("failed to aggregate candles: %w", err)
	}

	i.logger.Info("Aggregated candles across all exchanges",
		zap.Int("aggregated_count", len(aggregatedCandles)))

	// Insert into ClickHouse
	tableName := i.getTableNameForTimeframe(timeframe)
	if err := i.insertAggregatedCandles(ctx, aggregatedCandles, tableName); err != nil {
		return fmt.Errorf("failed to insert aggregated candles: %w", err)
	}

	return nil
}

// collectAllCandlesFromDirectory collects all candles from a directory without aggregating yet
func (i *Importer) collectAllCandlesFromDirectory(ctx context.Context, dir string, timeframe string) ([]CandleData, error) {
	var allCandles []CandleData

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !strings.HasSuffix(strings.ToLower(path), ".csv") {
			return nil
		}

		i.logger.Debug("Reading CSV file", zap.String("file", path))

		candles, err := i.parseCSV(path)
		if err != nil {
			i.logger.Error("Failed to parse CSV",
				zap.String("file", path),
				zap.Error(err))
			return nil // Continue with other files
		}

		// Add timeframe to each candle
		for idx := range candles {
			candles[idx].Timeframe = timeframe
		}

		allCandles = append(allCandles, candles...)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return allCandles, nil
}

// NewTokenMapping creates a new token mapping instance
func NewTokenMapping(postgres *sql.DB) *TokenMapping {
	return &TokenMapping{
		postgres: postgres,
		cache:    make(map[string]uint32),
	}
}

// GetTokenID gets or creates a token ID for a symbol
func (tm *TokenMapping) GetTokenID(ctx context.Context, symbol string) (uint32, error) {
	symbol = strings.ToUpper(symbol)

	// Check cache
	if id, exists := tm.cache[symbol]; exists {
		return id, nil
	}

	// Query database
	var tokenID uint32
	err := tm.postgres.QueryRowContext(ctx,
		"SELECT id FROM tokens WHERE UPPER(symbol) = $1 AND is_active = true",
		symbol).Scan(&tokenID)

	if err == sql.ErrNoRows {
		// Create new token
		err = tm.postgres.QueryRowContext(ctx, `
			INSERT INTO tokens (symbol, name, is_active) 
			VALUES ($1, $1, true) 
			RETURNING id`, symbol).Scan(&tokenID)
		if err != nil {
			return 0, fmt.Errorf("failed to create token %s: %w", symbol, err)
		}
		log.Printf("Created new token: %s (ID: %d)", symbol, tokenID)
	} else if err != nil {
		return 0, fmt.Errorf("failed to query token %s: %w", symbol, err)
	}

	// Cache result
	tm.cache[symbol] = tokenID
	return tokenID, nil
}

// parseCSV parses a CSV file into CandleData (handles EXCHANGE:SYMBOL format)
func (i *Importer) parseCSV(csvPath string) ([]CandleData, error) {
	file, err := os.Open(csvPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	if len(records) < 2 {
		return nil, nil // Empty file
	}

	var candles []CandleData
	// Skip header row (datetime,symbol,open,high,low,close,volume)
	for _, record := range records[1:] {
		if len(record) < 7 {
			continue
		}

		// Parse datetime
		datetime, err := time.Parse("2006-01-02 15:04:05", record[0])
		if err != nil {
			i.logger.Debug("Failed to parse datetime",
				zap.String("datetime", record[0]), zap.Error(err))
			continue
		}

		// Extract exchange and symbol from symbol column (e.g., "BINANCE:1INCHBTC")
		symbolParts := strings.Split(record[1], ":")
		if len(symbolParts) != 2 {
			i.logger.Debug("Invalid symbol format", zap.String("symbol", record[1]))
			continue
		}

		exchange := strings.ToUpper(symbolParts[0])
		symbol := symbolParts[1]

		// Parse symbol into base and quote
		baseSymbol, quoteSymbol := i.parseSymbolPair(symbol)

		// Parse OHLCV
		open, err := decimal.NewFromString(record[2])
		if err != nil {
			continue
		}
		high, err := decimal.NewFromString(record[3])
		if err != nil {
			continue
		}
		low, err := decimal.NewFromString(record[4])
		if err != nil {
			continue
		}
		close, err := decimal.NewFromString(record[5])
		if err != nil {
			continue
		}
		volume, err := decimal.NewFromString(record[6])
		if err != nil {
			continue
		}

		// Skip invalid data (zero prices or negative volume)
		if open.LessThanOrEqual(decimal.Zero) || high.LessThanOrEqual(decimal.Zero) ||
			low.LessThanOrEqual(decimal.Zero) || close.LessThanOrEqual(decimal.Zero) ||
			volume.LessThan(decimal.Zero) {
			continue
		}

		// Calculate quote volume
		quoteVolume := volume.Mul(close)

		candles = append(candles, CandleData{
			DateTime:    datetime,
			Exchange:    exchange,
			Symbol:      symbol,
			BaseSymbol:  baseSymbol,
			QuoteSymbol: quoteSymbol,
			Open:        open,
			High:        high,
			Low:         low,
			Close:       close,
			Volume:      volume,
			QuoteVolume: quoteVolume,
		})
	}

	return candles, nil
}

// parseSymbolPair splits a symbol into base and quote currencies
func (i *Importer) parseSymbolPair(symbol string) (base, quote string) {
	// Common quote currencies (longest first to avoid partial matches)
	quoteCurrencies := []string{
		"USDT", "USDC", "BUSD", "TUSD", "FDUSD", "DAI",
		"BTC", "ETH", "BNB", "XRP", "ADA", "DOT", "SOL",
		"EUR", "USD", "GBP", "JPY", "KRW", "CNY", "CAD", "AUD",
		"TRY", "BRL", "UAH", "ZAR", "NGN", "INR", "MXN", "ARS",
	}

	for _, quoteCurrency := range quoteCurrencies {
		if strings.HasSuffix(symbol, quoteCurrency) {
			potentialBase := strings.TrimSuffix(symbol, quoteCurrency)
			if len(potentialBase) > 0 {
				return potentialBase, quoteCurrency
			}
		}
	}

	// Fallback: assume last 3-4 characters are quote
	if len(symbol) > 6 {
		return symbol[:len(symbol)-4], symbol[len(symbol)-4:]
	}
	if len(symbol) > 3 {
		return symbol[:len(symbol)-3], symbol[len(symbol)-3:]
	}

	return symbol, "USD" // Default quote
}

// aggregateCandles aggregates candles by token pair, timestamp, combining volumes from different exchanges
func (i *Importer) aggregateCandles(ctx context.Context, candles []CandleData) ([]AggregatedCandle, error) {
	// Group by (base_token_id, quote_token_id, timestamp) - combine exchanges for same symbol/time
	type groupKey struct {
		BaseTokenID  uint32
		QuoteTokenID uint32
		Timestamp    int64 // Unix timestamp
	}

	groups := make(map[groupKey][]CandleData)

	// Group candles by symbol and timestamp
	for _, candle := range candles {
		baseTokenID, err := i.tokenMap.GetTokenID(ctx, candle.BaseSymbol)
		if err != nil {
			i.logger.Debug("Failed to get base token ID",
				zap.String("symbol", candle.BaseSymbol), zap.Error(err))
			continue
		}

		quoteTokenID, err := i.tokenMap.GetTokenID(ctx, candle.QuoteSymbol)
		if err != nil {
			i.logger.Debug("Failed to get quote token ID",
				zap.String("symbol", candle.QuoteSymbol), zap.Error(err))
			continue
		}

		key := groupKey{
			BaseTokenID:  baseTokenID,
			QuoteTokenID: quoteTokenID,
			Timestamp:    candle.DateTime.Unix(),
		}

		groups[key] = append(groups[key], candle)
	}

	i.logger.Info("Grouped candles for aggregation",
		zap.Int("unique_symbol_timestamp_groups", len(groups)))

	var result []AggregatedCandle

	// Aggregate each group (combining volumes from different exchanges for same symbol/timestamp)
	for key, groupCandles := range groups {
		timestamp := time.Unix(key.Timestamp, 0)
		aggregated, err := i.aggregateGroupWithVolumeCalculation(ctx, key.BaseTokenID, key.QuoteTokenID, timestamp, groupCandles)
		if err != nil {
			i.logger.Error("Failed to aggregate group",
				zap.Uint32("base_token_id", key.BaseTokenID),
				zap.Uint32("quote_token_id", key.QuoteTokenID),
				zap.Time("timestamp", timestamp),
				zap.Error(err))
			continue
		}

		if aggregated != nil {
			result = append(result, *aggregated)
		}
	}

	return result, nil
}

// prioritizeExchangeData prioritizes exchanges based on reliability and data history
func (i *Importer) prioritizeExchangeData(candles []CandleData, date time.Time) []CandleData {
	if len(candles) <= 1 {
		return candles
	}

	// Group candles by exchange
	exchangeGroups := make(map[string][]CandleData)
	for _, candle := range candles {
		exchangeGroups[candle.Exchange] = append(exchangeGroups[candle.Exchange], candle)
	}

	// Score each exchange
	type ExchangeScore struct {
		Exchange string
		Score    decimal.Decimal
		Candles  []CandleData
	}

	var scores []ExchangeScore
	for exchange, exchangeCandles := range exchangeGroups {
		// Calculate total volume for this exchange
		totalVolume := decimal.Zero
		for _, candle := range exchangeCandles {
			totalVolume = totalVolume.Add(candle.Volume)
		}

		// Get exchange reliability metrics
		metrics := i.scorer.CalculateScore(exchange, totalVolume, date)

		// Calculate additional factors
		dataCount := decimal.NewFromInt(int64(len(exchangeCandles)))
		historyBonus := decimal.NewFromFloat(1.0) // Base bonus

		// Bonus for exchanges with longer history (more data points)
		if len(exchangeCandles) > 100 {
			historyBonus = decimal.NewFromFloat(1.2)
		} else if len(exchangeCandles) > 50 {
			historyBonus = decimal.NewFromFloat(1.1)
		}

		// Final score: reliability * data_count * history_bonus
		finalScore := metrics.FinalScore.Mul(dataCount).Mul(historyBonus)

		scores = append(scores, ExchangeScore{
			Exchange: exchange,
			Score:    finalScore,
			Candles:  exchangeCandles,
		})
	}

	// Sort by score (highest first)
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].Score.GreaterThan(scores[j].Score)
	})

	// Return candles in prioritized order
	var result []CandleData
	for _, score := range scores {
		result = append(result, score.Candles...)
	}

	i.logger.Debug("Prioritized exchanges for aggregation",
		zap.String("date", date.Format("2006-01-02")),
		zap.Int("total_exchanges", len(scores)))

	return result
}

// aggregateGroupWithVolumeCalculation aggregates candles for same symbol/timestamp, combining volumes from different exchanges
func (i *Importer) aggregateGroupWithVolumeCalculation(ctx context.Context, baseTokenID, quoteTokenID uint32, timestamp time.Time, candles []CandleData) (*AggregatedCandle, error) {
	if len(candles) == 0 {
		return nil, nil
	}

	// First, prioritize exchanges by data quality and history length
	prioritizedCandles := i.prioritizeExchangeData(candles, timestamp)

	i.logger.Debug("Aggregating candles with volume combination",
		zap.Uint32("base_token_id", baseTokenID),
		zap.Uint32("quote_token_id", quoteTokenID),
		zap.Time("timestamp", timestamp),
		zap.Int("exchange_count", len(prioritizedCandles)))

	var (
		totalVolumeUSD      = decimal.Zero
		totalQuoteVolumeUSD = decimal.Zero
		weightedPriceSum    = decimal.Zero
		totalWeight         = decimal.Zero

		highPriceUSD  = decimal.Zero
		lowPriceUSD   = decimal.NewFromFloat(1e18) // Very high initial value
		openPriceUSD  = decimal.Zero
		closePriceUSD = decimal.Zero

		exchanges = make(map[string]bool)
		metrics   = make([]*reliability.ExchangeMetrics, 0)
		volumes   = make([]decimal.Decimal, 0)

		earliestTime = time.Now()
		latestTime   = time.Time{}
	)

	// Combine volumes from all exchanges for the same symbol/timestamp
	combinedVolume := decimal.Zero

	// Process each exchange's data (now prioritized)
	for _, candle := range prioritizedCandles {
		// Convert prices to USD
		priceUSD, volumeUSD, err := i.converter.ConvertPrice(ctx,
			candle.Close, candle.Volume, candle.QuoteSymbol)
		if err != nil {
			i.logger.Debug("Failed to convert currency",
				zap.String("quote", candle.QuoteSymbol), zap.Error(err))
			// Use original values if conversion fails
			priceUSD = candle.Close
			volumeUSD = candle.Volume
		}

		// Convert OHLC to USD
		openUSD, _, _ := i.converter.ConvertPrice(ctx, candle.Open, decimal.Zero, candle.QuoteSymbol)
		highUSD, _, _ := i.converter.ConvertPrice(ctx, candle.High, decimal.Zero, candle.QuoteSymbol)
		lowUSD, _, _ := i.converter.ConvertPrice(ctx, candle.Low, decimal.Zero, candle.QuoteSymbol)

		// Use original if conversion fails
		if openUSD.IsZero() {
			openUSD = candle.Open
		}
		if highUSD.IsZero() {
			highUSD = candle.High
		}
		if lowUSD.IsZero() {
			lowUSD = candle.Low
		}

		// Get exchange reliability score and weight
		metric := i.scorer.CalculateScore(candle.Exchange, volumeUSD, timestamp)
		weight := i.scorer.GetExchangeWeight(candle.Exchange, volumeUSD, timestamp)

		// Track exchanges
		exchanges[candle.Exchange] = true
		metrics = append(metrics, metric)
		volumes = append(volumes, volumeUSD)

		// COMBINE VOLUMES: Add volume from this exchange to the total
		combinedVolume = combinedVolume.Add(volumeUSD)

		i.logger.Debug("Adding exchange volume",
			zap.String("exchange", candle.Exchange),
			zap.String("volume_usd", volumeUSD.String()),
			zap.String("combined_total", combinedVolume.String()))

		// VWAP calculation (price weighted by volume and exchange reliability)
		effectiveVolume := volumeUSD.Mul(weight)
		contribution := priceUSD.Mul(effectiveVolume)

		weightedPriceSum = weightedPriceSum.Add(contribution)
		totalWeight = totalWeight.Add(effectiveVolume)
		totalVolumeUSD = totalVolumeUSD.Add(volumeUSD)
		totalQuoteVolumeUSD = totalQuoteVolumeUSD.Add(volumeUSD.Mul(priceUSD))

		// Track high/low across all exchanges
		if highUSD.GreaterThan(highPriceUSD) {
			highPriceUSD = highUSD
		}
		if lowUSD.LessThan(lowPriceUSD) {
			lowPriceUSD = lowUSD
		}

		// Track open/close by time
		if candle.DateTime.Before(earliestTime) || earliestTime.IsZero() {
			earliestTime = candle.DateTime
			openPriceUSD = openUSD
		}
		if candle.DateTime.After(latestTime) {
			latestTime = candle.DateTime
			closePriceUSD = priceUSD
		}
	}

	// Calculate VWAP
	vwapPrice := decimal.Zero
	if totalWeight.IsPositive() {
		vwapPrice = weightedPriceSum.Div(totalWeight)
	}

	// Calculate data quality score
	qualityScore := i.scorer.GetQualityScore(metrics, volumes)

	// Convert exchanges map to slice
	exchangeList := make([]string, 0, len(exchanges))
	for ex := range exchanges {
		exchangeList = append(exchangeList, ex)
	}

	return &AggregatedCandle{
		Timestamp:        timestamp,
		BaseTokenID:      baseTokenID,
		QuoteTokenID:     quoteTokenID,
		Open:             openPriceUSD,
		High:             highPriceUSD,
		Low:              lowPriceUSD,
		Close:            closePriceUSD,
		Volume:           combinedVolume, // Use the combined volume from all exchanges
		QuoteVolume:      totalQuoteVolumeUSD,
		VWAPPrice:        vwapPrice,
		ExchangeCount:    uint8(len(exchanges)),
		ContribExchanges: exchangeList,
		DataQualityScore: qualityScore,
		TradeCount:       uint32(len(candles)),
	}, nil
}

// insertAggregatedCandles inserts aggregated candles into ClickHouse
func (i *Importer) insertAggregatedCandles(ctx context.Context, candles []AggregatedCandle, tableName string) error {
	if len(candles) == 0 {
		return nil
	}

	batch, err := i.clickhouse.PrepareBatch(ctx, fmt.Sprintf(`
		INSERT INTO %s (
			timestamp, base_token_id, quote_token_id,
			open, high, low, close, volume, quote_volume,
			vwap_price, exchange_count, contributing_exchanges,
			data_quality_score, trade_count
		)`, tableName))
	if err != nil {
		return fmt.Errorf("failed to prepare batch: %w", err)
	}

	insertCount := 0
	for _, candle := range candles {
		err := batch.Append(
			candle.Timestamp,
			candle.BaseTokenID,
			candle.QuoteTokenID,
			candle.Open,
			candle.High,
			candle.Low,
			candle.Close,
			candle.Volume,
			candle.QuoteVolume,
			candle.VWAPPrice,
			candle.ExchangeCount,
			candle.ContribExchanges,
			candle.DataQualityScore,
			candle.TradeCount,
		)
		if err != nil {
			i.logger.Error("Failed to append candle", zap.Error(err))
			continue
		}
		insertCount++
	}

	if err := batch.Send(); err != nil {
		return fmt.Errorf("failed to send batch: %w", err)
	}

	i.logger.Info("Inserted aggregated candles",
		zap.String("table", tableName),
		zap.Int("count", insertCount))

	return nil
}

// getTableNameForTimeframe returns the appropriate table name for a timeframe
func (i *Importer) getTableNameForTimeframe(timeframe string) string {
	// Map timeframes to table names
	tableMap := map[string]string{
		"1m":  "ohlcv_1m",
		"5m":  "ohlcv_5m",
		"15m": "ohlcv_15m",
		"30m": "ohlcv_30m",
		"1h":  "ohlcv_1h",
		"4h":  "ohlcv_4h",
		"1d":  "ohlcv_1d",
	}

	if table, ok := tableMap[timeframe]; ok {
		return table
	}

	// Default to 1d table for unknown timeframes
	i.logger.Warn("Unknown timeframe, using default table",
		zap.String("timeframe", timeframe))
	return "ohlcv_1d"
}
