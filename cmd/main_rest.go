package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/ashmitsharp/trading/internal/calculator"
	"github.com/ashmitsharp/trading/internal/chart_api"
	"github.com/ashmitsharp/trading/internal/exchanges"
	"github.com/ashmitsharp/trading/internal/handler"
	"github.com/ashmitsharp/trading/internal/ohlcv"
	"github.com/ashmitsharp/trading/internal/outlier"
	"github.com/ashmitsharp/trading/internal/storage"
	"github.com/ashmitsharp/trading/internal/symbol"
)

type Application struct {
	logger               *zap.Logger
	postgresDB           *sql.DB
	clickhouseDB         clickhouse.Conn
	factory              *exchanges.ExchangeFactory
	vwapCalc             *calculator.EnhancedVWAPCalculator
	priceStorage         *storage.PriceStorage
	vwapStorage          *storage.VWAPStorage
	symbolResolver       *symbol.Resolver
	outlierDetector      *outlier.Detector
	verificationHandler  *handler.VerificationHandler
	volumeNormalizer     *exchanges.VolumeNormalizer
	ohlcvService         *ohlcv.Service
	chartService         *chart_api.ChartService
}

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Printf("No .env file found: %v", err)
	}

	// Initialize logger
	var logger *zap.Logger
	var err error
	logToFile := os.Getenv("LOG_TO_FILE")
	
	if logToFile == "true" {
		config := zap.NewProductionConfig()
		config.OutputPaths = []string{"app.log"}
		config.ErrorOutputPaths = []string{"app.log"}
		logger, err = config.Build()
	} else {
		logger, err = zap.NewProduction()
	}
	
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer logger.Sync()

	// Create application
	app := &Application{
		logger: logger,
	}

	// Initialize databases
	if err := app.initDatabases(); err != nil {
		logger.Fatal("Failed to initialize databases", zap.Error(err))
	}
	defer app.closeDatabases()

	// Initialize exchange factory
	factory, err := exchanges.NewExchangeFactory("configs/exchanges.json", logger)
	if err != nil {
		logger.Fatal("Failed to create exchange factory", zap.Error(err))
	}
	app.factory = factory

	// Initialize symbol resolver
	app.symbolResolver = symbol.NewResolver(app.postgresDB, logger)
	
	// Initialize volume normalizer
	app.volumeNormalizer = exchanges.NewVolumeNormalizer(logger)

	// Initialize enhanced VWAP calculator with configuration
	vwapConfig := &calculator.VWAPConfig{
		MaxTickers:                 600,
		StaleDataThreshold:         8 * time.Hour,
		MADMultiplier:              decimal.NewFromInt(4),
		MADConsistencyConstant:     decimal.NewFromFloat(1.4826),
		MinExchanges:               2, // Reduced from 3 for testing - many pairs only have 2 exchanges
		VolumeManipulationThreshold: decimal.NewFromInt(50),
		EnableDetailedStats:        true,
	}
	app.vwapCalc = calculator.NewEnhancedVWAPCalculator(logger, vwapConfig)

	// Initialize storage services
	app.priceStorage = storage.NewPriceStorage(app.clickhouseDB, logger)
	app.vwapStorage = storage.NewVWAPStorage(app.clickhouseDB, logger)

	// Initialize outlier detector
	app.outlierDetector = outlier.NewDetector(app.postgresDB, app.clickhouseDB, logger)

	// Initialize verification handler
	app.verificationHandler = handler.NewVerificationHandler(app.postgresDB, app.outlierDetector, logger)

	// Initialize OHLCV service
	app.ohlcvService = ohlcv.NewService(app.clickhouseDB, logger)

	// Initialize chart service (WebSocket + Historical API)
	app.chartService = chart_api.NewChartService(app.clickhouseDB, logger)

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Determine service mode
	serviceMode := os.Getenv("SERVICE_MODE")
	if serviceMode == "" {
		serviceMode = "all"
	}

	// Start services
	var wg sync.WaitGroup

	// Start chart service (always runs for API)
	if serviceMode == "api" || serviceMode == "all" {
		if err := app.chartService.Start(); err != nil {
			logger.Fatal("Failed to start chart service", zap.Error(err))
		}
		defer app.chartService.Stop()
	}

	switch serviceMode {
	case "poller":
		wg.Add(2)
		go app.runPoller(ctx, &wg)
		go app.runOHLCV(ctx, &wg)
	case "api":
		wg.Add(1)
		go app.runAPI(ctx, &wg)
	case "all":
		wg.Add(3)
		go app.runPoller(ctx, &wg)
		go app.runAPI(ctx, &wg)
		go app.runOHLCV(ctx, &wg)
	default:
		logger.Fatal("Invalid SERVICE_MODE", zap.String("mode", serviceMode))
	}

	// Wait for shutdown signal
	<-sigChan
	logger.Info("Shutting down services...")
	cancel()

	// Wait for services to finish
	wg.Wait()
	logger.Info("All services stopped")
}

func (app *Application) initDatabases() error {
	// Initialize PostgreSQL
	pgHost := getEnv("POSTGRES_HOST", "localhost")
	pgPort := getEnv("POSTGRES_PORT", "5432")
	pgUser := getEnv("POSTGRES_USER", "crypto_user")
	pgPass := getEnv("POSTGRES_PASSWORD", "crypto_password")
	pgDB := getEnv("POSTGRES_DB", "crypto_platform")

	pgDSN := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		pgHost, pgPort, pgUser, pgPass, pgDB)

	postgresDB, err := sql.Open("postgres", pgDSN)
	if err != nil {
		return fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}

	if err := postgresDB.Ping(); err != nil {
		return fmt.Errorf("failed to ping PostgreSQL: %w", err)
	}

	app.postgresDB = postgresDB
	app.logger.Info("Connected to PostgreSQL")

	// Initialize ClickHouse
	chHost := getEnv("CLICKHOUSE_HOST", "localhost")
	chPort := getEnv("CLICKHOUSE_PORT", "9001")  // Use port from .env
	chDB := getEnv("CLICKHOUSE_DATABASE", "crypto_platform")  // Use existing database
	chUser := getEnv("CLICKHOUSE_USER", "default")
	chPassword := getEnv("CLICKHOUSE_PASSWORD", "clickhouse123")

	clickhouseDB, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{fmt.Sprintf("%s:%s", chHost, chPort)},
		Auth: clickhouse.Auth{
			Database: chDB,
			Username: chUser,
			Password: chPassword,
		},
		Settings: clickhouse.Settings{
			"max_execution_time": 60,
		},
		DialTimeout:      30 * time.Second,
		ConnMaxLifetime:  time.Hour,
		MaxOpenConns:     5,
		MaxIdleConns:     2,
	})
	if err != nil {
		return fmt.Errorf("failed to connect to ClickHouse: %w", err)
	}

	if err := clickhouseDB.Ping(context.Background()); err != nil {
		return fmt.Errorf("failed to ping ClickHouse: %w", err)
	}

	app.clickhouseDB = clickhouseDB
	app.logger.Info("Connected to ClickHouse")

	return nil
}

func (app *Application) closeDatabases() {
	if app.postgresDB != nil {
		app.postgresDB.Close()
	}
	if app.clickhouseDB != nil {
		app.clickhouseDB.Close()
	}
}

func (app *Application) runPoller(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	app.logger.Info("Starting polling service...")

	// Get all exchange clients
	clients := app.factory.CreateAllClients()
	app.logger.Info("Created exchange clients", zap.Int("count", len(clients)))

	// Polling interval
	pollInterval := 15 * time.Second
	if interval := os.Getenv("POLL_INTERVAL"); interval != "" {
		if d, err := time.ParseDuration(interval); err == nil {
			pollInterval = d
		}
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	// Initial poll
	app.pollExchanges(ctx, clients)

	for {
		select {
		case <-ctx.Done():
			app.logger.Info("Polling service stopped")
			return
		case <-ticker.C:
			app.pollExchanges(ctx, clients)
		}
	}
}

func (app *Application) resolveTokenIDs(tickers []exchanges.TickerData) {
	for i := range tickers {
		ticker := &tickers[i]
		
		// First try to resolve as a trading pair
		pair, err := app.symbolResolver.ResolveTradingPair(ticker.ExchangeID, ticker.Symbol)
		if err == nil {
			ticker.BaseTokenID = pair.BaseTokenID
			ticker.QuoteTokenID = pair.QuoteTokenID
			continue
		}
		
		// Fallback: try to resolve individual symbols
		// Note: In the future, we should check if we have slug data from exchange
		baseID, baseMethod, err1 := app.resolveToken(ticker.ExchangeID, ticker.BaseSymbol, "")
		quoteID, quoteMethod, err2 := app.resolveToken(ticker.ExchangeID, ticker.QuoteSymbol, "")
		
		if err1 == nil && err2 == nil {
			ticker.BaseTokenID = baseID
			ticker.QuoteTokenID = quoteID
			
			// Determine overall mapping method for the pair
			method := "slug"
			if baseMethod == "symbol" || quoteMethod == "symbol" {
				method = "symbol"
			}
			
			// Add this pair to the database for future use
			app.symbolResolver.AddTradingPair(baseID, quoteID, ticker.ExchangeID, ticker.Symbol)
			
			// Log if symbol-based mapping was used
			if method == "symbol" {
				app.logger.Info("Symbol-based mapping used",
					zap.String("exchange", ticker.ExchangeID),
					zap.String("pair", ticker.Symbol),
					zap.String("base_method", baseMethod),
					zap.String("quote_method", quoteMethod))
			}
		}
		
		// Log unresolved pairs for investigation
		if ticker.BaseTokenID == 0 || ticker.QuoteTokenID == 0 {
			app.logger.Warn("Failed to resolve token IDs",
				zap.String("exchange", ticker.ExchangeID),
				zap.String("symbol", ticker.Symbol),
				zap.String("base", ticker.BaseSymbol),
				zap.String("quote", ticker.QuoteSymbol))
		}
	}
}

// resolveToken attempts to resolve a single token with method tracking
func (app *Application) resolveToken(exchangeID, symbol, slug string) (int, string, error) {
	// If we have a slug, use the slug-based resolver
	if slug != "" {
		return app.symbolResolver.ResolveWithSlug(exchangeID, symbol, slug)
	}
	
	// Try direct symbol resolution
	tokenID, err := app.symbolResolver.ResolveSymbol(exchangeID, symbol)
	if err == nil {
		return tokenID, "symbol", nil
	}
	
	// Try normalized symbol as last resort
	if id, err := app.symbolResolver.GetTokenByNormalizedSymbol(symbol); err == nil {
		// Add mapping for future use with lower confidence
		app.symbolResolver.AddSymbolMappingWithMethod(id, exchangeID, symbol, 
			symbol, "symbol", 0.75)
		return id, "symbol", nil
	}
	
	return 0, "", fmt.Errorf("unable to resolve %s", symbol)
}

func (app *Application) pollExchanges(ctx context.Context, clients map[string]exchanges.ExchangeClient) {
	pollTime := time.Now()
	
	// Get CPU stats before polling
	var memStatsBefore runtime.MemStats
	runtime.ReadMemStats(&memStatsBefore)
	goroutinesBefore := runtime.NumGoroutine()
	
	app.logger.Info("Starting poll cycle",
		zap.Time("poll_time", pollTime),
		zap.String("minute", pollTime.Format("15:04")),
		zap.Int("goroutines_before", goroutinesBefore),
		zap.Uint64("alloc_mb_before", memStatsBefore.Alloc/1024/1024))

	// Collect prices from all exchanges
	var wg sync.WaitGroup
	pricesChan := make(chan []exchanges.TickerData, len(clients))

	for id, client := range clients {
		if !client.IsHealthy() {
			app.logger.Warn("Skipping unhealthy exchange", zap.String("exchange", id))
			continue
		}

		wg.Add(1)
		go func(exchangeID string, c exchanges.ExchangeClient) {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()

			tickers, err := c.GetAllTickers(ctx)
			if err != nil {
				app.logger.Error("Failed to get tickers",
					zap.String("exchange", exchangeID),
					zap.Error(err))
				return
			}

			pricesChan <- tickers
		}(id, client)
	}

	// Wait for all exchanges
	go func() {
		wg.Wait()
		close(pricesChan)
	}()

	// Collect all prices
	var allPrices []exchanges.TickerData
	for prices := range pricesChan {
		allPrices = append(allPrices, prices...)
	}

	app.logger.Info("Collected prices",
		zap.Int("total", len(allPrices)),
		zap.Int("exchanges", len(clients)))

	// Normalize volumes to USD for all tickers
	for i := range allPrices {
		app.volumeNormalizer.NormalizeVolume(&allPrices[i])
	}

	// Resolve token IDs for all tickers
	app.resolveTokenIDs(allPrices)

	// Store raw price tickers in ClickHouse
	if err := app.priceStorage.StorePriceTickers(ctx, allPrices); err != nil {
		app.logger.Error("Failed to store price tickers", zap.Error(err))
	}

	// Group prices by token pair for VWAP calculation
	pricesByPair := make(map[string][]calculator.PriceData)
	for _, ticker := range allPrices {
		// Skip if tokens are not resolved
		if ticker.BaseTokenID == 0 || ticker.QuoteTokenID == 0 {
			continue
		}
		// Use token IDs as the key for consistent grouping
		pairKey := fmt.Sprintf("%d-%d", ticker.BaseTokenID, ticker.QuoteTokenID)

		// NO MANUAL WEIGHTS - let volume determine influence
		pricesByPair[pairKey] = append(pricesByPair[pairKey], calculator.PriceData{
			ExchangeID:   ticker.ExchangeID,
			Symbol:       ticker.Symbol,
			BaseTokenID:  ticker.BaseTokenID,
			QuoteTokenID: ticker.QuoteTokenID,
			Price:        ticker.Price,
			Volume:       ticker.Volume24h,
			Weight:       decimal.NewFromInt(1), // Equal weight - volume determines influence
			Timestamp:    ticker.Timestamp,
		})
	}

	// Calculate VWAP for each token pair
	vwapResults := app.vwapCalc.CalculateBatch(pricesByPair)

	// Store VWAP prices in ClickHouse
	app.storeVWAPPrices(ctx, vwapResults)
	
	// Get CPU stats after polling
	var memStatsAfter runtime.MemStats
	runtime.ReadMemStats(&memStatsAfter)
	goroutinesAfter := runtime.NumGoroutine()
	
	// Log poll completion with resource usage
	app.logger.Info("Poll cycle completed",
		zap.Time("completed_at", time.Now()),
		zap.Duration("duration", time.Since(pollTime)),
		zap.Int("pairs_processed", len(vwapResults)),
		zap.Int("goroutines_after", goroutinesAfter),
		zap.Uint64("alloc_mb_after", memStatsAfter.Alloc/1024/1024),
		zap.Uint64("total_alloc_mb", memStatsAfter.TotalAlloc/1024/1024),
		zap.Uint32("num_gc", memStatsAfter.NumGC))
}

func (app *Application) storeVWAPPrices(ctx context.Context, results map[string]*calculator.EnhancedVWAPResult) {
	if len(results) == 0 {
		return
	}

	// Convert enhanced results to basic results for storage
	basicResults := make(map[string]*calculator.VWAPResult)
	for key, enhanced := range results {
		basicResults[key] = &enhanced.VWAPResult
		
		// Log additional metrics for monitoring
		if enhanced.QualityIndicator == "low" || enhanced.QualityIndicator == "insufficient" {
			app.logger.Warn("Low quality VWAP result",
				zap.String("pair", key),
				zap.String("quality", enhanced.QualityIndicator),
				zap.Float64("confidence", enhanced.ConfidenceScore),
				zap.Int("exchanges", enhanced.ExchangeCount))
		}
	}

	// Use the storage service to store VWAP results
	if err := app.vwapStorage.StoreVWAPResults(ctx, basicResults); err != nil {
		app.logger.Error("Failed to store VWAP results", zap.Error(err))
	}
	
	// Log statistics periodically
	if time.Now().Unix()%60 == 0 { // Every minute
		stats := app.vwapCalc.GetStatistics()
		app.logger.Info("VWAP Calculator Statistics",
			zap.Int64("calculations", stats.CalculationsPerformed),
			zap.String("avg_confidence", stats.AverageConfidence.String()),
			zap.Int64("low_confidence", stats.LowConfidenceCount),
			zap.Int64("insufficient_data", stats.InsufficientDataCount))
	}
}

func (app *Application) runOHLCV(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	app.logger.Info("Starting OHLCV service...")
	
	if err := app.ohlcvService.Start(ctx); err != nil {
		app.logger.Fatal("Failed to start OHLCV service", zap.Error(err))
	}
	
	// Wait for context cancellation
	<-ctx.Done()
	
	// Stop the service gracefully
	app.ohlcvService.Stop()
	app.logger.Info("OHLCV service stopped")
}

func (app *Application) runAPI(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	app.logger.Info("Starting API service...")

	// Create Gin router
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(gin.Logger())

	// Setup routes
	app.setupRoutes(router)

	// Start server
	port := getEnv("SERVER_PORT", ":8080")
	srv := &http.Server{
		Addr:    port,
		Handler: router,
	}

	// Start server in goroutine
	go func() {
		app.logger.Info("API server starting", zap.String("port", port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			app.logger.Fatal("Failed to start API server", zap.Error(err))
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()

	// Shutdown server gracefully
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		app.logger.Error("Failed to shutdown server gracefully", zap.Error(err))
	}

	app.logger.Info("API service stopped")
}

func (app *Application) setupRoutes(router *gin.Engine) {
	// Health check
	router.GET("/health", app.healthCheck)

	// Serve admin dashboard
	router.Static("/admin", "./web/admin")
	
	// Serve charts dashboard
	router.Static("/charts", "./web/charts")

	// Initialize chart handler
	chartHandler := handler.NewChartHandler(app.chartService, app.logger)

	// API v1 routes
	v1 := router.Group("/api/v1")
	{
		// Exchange endpoints
		v1.GET("/exchanges", app.getExchanges)
		v1.GET("/exchanges/:id", app.getExchange)

		// Token endpoints
		v1.GET("/tokens", app.getTokens)
		v1.GET("/tokens/:id", app.getToken)

		// Ticker endpoints
		v1.GET("/tickers", app.getAllTickers)
		v1.GET("/tickers/:symbol", app.getTicker)

		// VWAP endpoints
		v1.GET("/vwap/:symbol", app.getVWAPPrice)
		v1.GET("/vwap/stats", app.getVWAPStats)
		
		// OHLCV endpoints (legacy)
		v1.GET("/ohlcv/:base/:quote", app.getOHLCV)
		v1.POST("/ohlcv/backfill", app.backfillOHLCV)
		
		// Chart endpoints (new WebSocket + historical API)
		chart := v1.Group("/chart")
		{
			// WebSocket endpoint for live streaming
			chart.GET("/ws", chartHandler.HandleWebSocket)
			
			// Historical candle data
			chart.GET("/candles", chartHandler.GetHistoricalCandles)
			
			// Latest price
			chart.GET("/price/:baseTokenID/:quoteTokenID", chartHandler.GetLatestPrice)
			
			// Metadata endpoints
			chart.GET("/timeframes", chartHandler.GetSupportedTimeframes)
			chart.GET("/stats", chartHandler.GetChartStats)
		}
		
		// Verification endpoints (admin)
		admin := v1.Group("/admin")
		{
			admin.GET("/mappings/unverified", app.getUnverifiedMappings)
			admin.POST("/mappings/:id/verify", app.verifyMapping)
			admin.POST("/mappings/:id/flag", app.flagMapping)
			admin.GET("/outliers", app.getOutliers)
			admin.POST("/outliers/:id/resolve", app.resolveOutlier)
		}
	}
	
	// Redirect root to charts
	router.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/charts/")
	})
}

// Handler functions
func (app *Application) healthCheck(c *gin.Context) {
	// Check database connections
	pgHealthy := app.postgresDB.Ping() == nil
	chHealthy := app.clickhouseDB.Ping(c.Request.Context()) == nil

	status := "healthy"
	if !pgHealthy || !chHealthy {
		status = "degraded"
	}

	c.JSON(http.StatusOK, gin.H{
		"status": status,
		"services": gin.H{
			"postgres":   pgHealthy,
			"clickhouse": chHealthy,
		},
		"timestamp": time.Now().Unix(),
	})
}

func (app *Application) getExchanges(c *gin.Context) {
	query := `
		SELECT exchange_id, name, is_active, last_successful_poll, consecutive_failures
		FROM exchanges
		WHERE is_active = true
		ORDER BY weight DESC
	`

	rows, err := app.postgresDB.Query(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var exchanges []map[string]interface{}
	for rows.Next() {
		var id, name string
		var isActive bool
		var lastPoll sql.NullTime
		var failures int

		if err := rows.Scan(&id, &name, &isActive, &lastPoll, &failures); err != nil {
			continue
		}

		exchange := map[string]interface{}{
			"id":                   id,
			"name":                 name,
			"is_active":            isActive,
			"consecutive_failures": failures,
		}

		if lastPoll.Valid {
			exchange["last_successful_poll"] = lastPoll.Time
		}

		exchanges = append(exchanges, exchange)
	}

	c.JSON(http.StatusOK, exchanges)
}

func (app *Application) getExchange(c *gin.Context) {
	exchangeID := c.Param("id")

	var id, name string
	var isActive bool
	var weight float64

	query := `
		SELECT exchange_id, name, is_active, weight
		FROM exchanges
		WHERE exchange_id = $1
	`

	err := app.postgresDB.QueryRow(query, exchangeID).Scan(&id, &name, &isActive, &weight)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Exchange not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":        id,
		"name":      name,
		"is_active": isActive,
		"weight":    weight,
	})
}

func (app *Application) getTokens(c *gin.Context) {
	query := `
		SELECT id, symbol, name, current_price, market_cap, market_cap_rank
		FROM tokens
		WHERE is_active = true
		ORDER BY market_cap_rank ASC NULLS LAST
		LIMIT 100
	`

	rows, err := app.postgresDB.Query(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var tokens []map[string]interface{}
	for rows.Next() {
		var id, symbol, name string
		var price, marketCap sql.NullFloat64
		var rank sql.NullInt64

		if err := rows.Scan(&id, &symbol, &name, &price, &marketCap, &rank); err != nil {
			continue
		}

		token := map[string]interface{}{
			"id":     id,
			"symbol": symbol,
			"name":   name,
		}

		if price.Valid {
			token["price"] = price.Float64
		}
		if marketCap.Valid {
			token["market_cap"] = marketCap.Float64
		}
		if rank.Valid {
			token["rank"] = rank.Int64
		}

		tokens = append(tokens, token)
	}

	c.JSON(http.StatusOK, tokens)
}

func (app *Application) getToken(c *gin.Context) {
	tokenID := c.Param("id")

	var id, symbol, name string
	var price sql.NullFloat64

	query := `
		SELECT id, symbol, name, current_price
		FROM tokens
		WHERE id = $1
	`

	err := app.postgresDB.QueryRow(query, tokenID).Scan(&id, &symbol, &name, &price)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Token not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	result := gin.H{
		"id":     id,
		"symbol": symbol,
		"name":   name,
	}

	if price.Valid {
		result["price"] = price.Float64
	}

	c.JSON(http.StatusOK, result)
}

func (app *Application) getAllTickers(c *gin.Context) {
	// For now, return from PostgreSQL tokens table
	query := `
		SELECT symbol, name, current_price, price_change_24h, trading_volume_24h
		FROM tokens
		WHERE is_active = true AND current_price > 0
		ORDER BY market_cap_rank ASC NULLS LAST
		LIMIT 100
	`

	rows, err := app.postgresDB.Query(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var tickers []map[string]interface{}
	for rows.Next() {
		var symbol, name string
		var price, priceChange, volume sql.NullFloat64

		if err := rows.Scan(&symbol, &name, &price, &priceChange, &volume); err != nil {
			continue
		}

		ticker := map[string]interface{}{
			"symbol": symbol,
			"name":   name,
		}

		if price.Valid {
			ticker["price"] = price.Float64
		}
		if priceChange.Valid {
			ticker["price_change_24h"] = priceChange.Float64
		}
		if volume.Valid {
			ticker["volume_24h"] = volume.Float64
		}

		tickers = append(tickers, ticker)
	}

	c.JSON(http.StatusOK, tickers)
}

func (app *Application) getTicker(c *gin.Context) {
	symbol := c.Param("symbol")

	// Try to get from latest VWAP prices in ClickHouse
	c.JSON(http.StatusOK, gin.H{
		"symbol":  symbol,
		"message": "VWAP price calculation coming soon",
	})
}

func (app *Application) getVWAPPrice(c *gin.Context) {
	symbol := c.Param("symbol")

	// Query latest VWAP price from ClickHouse
	c.JSON(http.StatusOK, gin.H{
		"symbol":  symbol,
		"message": "VWAP price endpoint coming soon",
	})
}

func (app *Application) getVWAPStats(c *gin.Context) {
	stats := app.vwapCalc.GetStatistics()
	
	c.JSON(http.StatusOK, gin.H{
		"calculations_performed": stats.CalculationsPerformed,
		"average_mad":           stats.AverageMAD.String(),
		"average_confidence":    stats.AverageConfidence.String(),
		"low_confidence_count":  stats.LowConfidenceCount,
		"insufficient_data_count": stats.InsufficientDataCount,
		"tickers_received":      stats.TotalTickersReceived,
		"tickers_after_quality": stats.TickersAfterQuality,
		"tickers_after_mad":     stats.TickersAfterMAD,
		"contributing_exchanges": stats.ContributingExchanges,
	})
}

// Admin verification endpoints
func (app *Application) getUnverifiedMappings(c *gin.Context) {
	app.verificationHandler.GetUnverifiedMappings(c)
}

func (app *Application) verifyMapping(c *gin.Context) {
	app.verificationHandler.VerifyMapping(c)
}

func (app *Application) flagMapping(c *gin.Context) {
	app.verificationHandler.FlagMapping(c)
}

func (app *Application) getOutliers(c *gin.Context) {
	app.verificationHandler.GetOutliers(c)
}

func (app *Application) resolveOutlier(c *gin.Context) {
	app.verificationHandler.ResolveOutlier(c)
}

// OHLCV endpoints
func (app *Application) getOHLCV(c *gin.Context) {
	baseSymbol := c.Param("base")
	quoteSymbol := c.Param("quote")
	
	// Get optional query parameters
	timeframe := c.DefaultQuery("timeframe", "1m")
	exchangeID := c.Query("exchange")
	limitStr := c.DefaultQuery("limit", "100")
	
	// Convert limit to int
	limit := 100
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 1000 {
		limit = l
	}
	
	// Get token IDs
	var baseTokenID, quoteTokenID uint32
	err := app.postgresDB.QueryRow(`
		SELECT id FROM tokens WHERE symbol = $1
	`, baseSymbol).Scan(&baseTokenID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Base token not found"})
		return
	}
	
	err = app.postgresDB.QueryRow(`
		SELECT id FROM tokens WHERE symbol = $1
	`, quoteSymbol).Scan(&quoteTokenID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Quote token not found"})
		return
	}
	
	// Convert timeframe string to enum
	var tf ohlcv.Timeframe
	switch timeframe {
	case "1m":
		tf = ohlcv.Timeframe1m
	case "5m":
		tf = ohlcv.Timeframe5m
	case "15m":
		tf = ohlcv.Timeframe15m
	case "1h":
		tf = ohlcv.Timeframe1h
	case "4h":
		tf = ohlcv.Timeframe4h
	case "1d":
		tf = ohlcv.Timeframe1d
	case "1w":
		tf = ohlcv.Timeframe1w
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid timeframe"})
		return
	}
	
	// Get candles
	candles, err := app.ohlcvService.GetLatestCandles(
		c.Request.Context(),
		baseTokenID,
		quoteTokenID,
		tf,
		exchangeID,
		limit,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	
	// Format response
	result := make([]map[string]interface{}, len(candles))
	for i, candle := range candles {
		result[i] = map[string]interface{}{
			"timestamp":    candle.Timestamp.Unix(),
			"open":         candle.Open.String(),
			"high":         candle.High.String(),
			"low":          candle.Low.String(),
			"close":        candle.Close.String(),
			"volume":       candle.Volume.String(),
			"quote_volume": candle.QuoteVolume.String(),
			"vwap":         candle.VWAPPrice.String(),
			"trade_count":  candle.TradeCount,
		}
		if exchangeID != "" {
			result[i]["exchange"] = candle.ExchangeID
		}
	}
	
	c.JSON(http.StatusOK, gin.H{
		"pair":      baseSymbol + "/" + quoteSymbol,
		"timeframe": timeframe,
		"candles":   result,
	})
}

func (app *Application) backfillOHLCV(c *gin.Context) {
	var req struct {
		StartDate string `json:"start_date" binding:"required"`
		EndDate   string `json:"end_date" binding:"required"`
	}
	
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	// Parse dates
	startDate, err := time.Parse("2006-01-02", req.StartDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid start_date format (use YYYY-MM-DD)"})
		return
	}
	
	endDate, err := time.Parse("2006-01-02", req.EndDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid end_date format (use YYYY-MM-DD)"})
		return
	}
	
	// Run backfill in background
	go func() {
		if err := app.ohlcvService.BackfillHistoricalData(
			context.Background(),
			startDate,
			endDate,
		); err != nil {
			app.logger.Error("Backfill failed", zap.Error(err))
		}
	}()
	
	c.JSON(http.StatusAccepted, gin.H{
		"message": "Backfill started",
		"start":   startDate,
		"end":     endDate,
	})
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
