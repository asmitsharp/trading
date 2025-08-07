package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
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
	"github.com/ashmitsharp/trading/internal/exchanges"
	"github.com/ashmitsharp/trading/internal/storage"
)

type Application struct {
	logger       *zap.Logger
	postgresDB   *sql.DB
	clickhouseDB clickhouse.Conn
	factory      *exchanges.ExchangeFactory
	vwapCalc     *calculator.VWAPCalculator
	priceStorage *storage.PriceStorage
	vwapStorage  *storage.VWAPStorage
}

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Printf("No .env file found: %v", err)
	}

	// Initialize logger
	logger, err := zap.NewProduction()
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

	// Initialize VWAP calculator
	app.vwapCalc = calculator.NewVWAPCalculator(logger)

	// Initialize storage services
	app.priceStorage = storage.NewPriceStorage(app.clickhouseDB, logger)
	app.vwapStorage = storage.NewVWAPStorage(app.clickhouseDB, logger)

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

	switch serviceMode {
	case "poller":
		wg.Add(1)
		go app.runPoller(ctx, &wg)
	case "api":
		wg.Add(1)
		go app.runAPI(ctx, &wg)
	case "all":
		wg.Add(2)
		go app.runPoller(ctx, &wg)
		go app.runAPI(ctx, &wg)
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
	chPort := getEnv("CLICKHOUSE_PORT", "9001")
	chDB := getEnv("CLICKHOUSE_DATABASE", "crypto_platform")
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
		DialTimeout: 5 * time.Second,
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

func (app *Application) pollExchanges(ctx context.Context, clients map[string]exchanges.ExchangeClient) {
	app.logger.Debug("Starting poll cycle")

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

	// Store raw price tickers in ClickHouse
	if err := app.priceStorage.StorePriceTickers(ctx, allPrices); err != nil {
		app.logger.Error("Failed to store price tickers", zap.Error(err))
	}

	// Group prices by symbol for VWAP calculation
	pricesBySymbol := make(map[string][]calculator.PriceData)
	for _, ticker := range allPrices {
		// Skip if base/quote symbols are not properly parsed
		if ticker.BaseSymbol == "" || ticker.QuoteSymbol == "" {
			continue
		}
		symbol := fmt.Sprintf("%s-%s", ticker.BaseSymbol, ticker.QuoteSymbol)

		// Get exchange weight from client
		weight := decimal.NewFromFloat(0.01) // Default weight
		if client, ok := clients[ticker.ExchangeID]; ok {
			weight = decimal.NewFromFloat(client.GetWeight())
		}

		pricesBySymbol[symbol] = append(pricesBySymbol[symbol], calculator.PriceData{
			ExchangeID: ticker.ExchangeID,
			Symbol:     ticker.Symbol,
			Price:      ticker.Price,
			Volume:     ticker.Volume24h,
			Weight:     weight, // Use exchange weight from config
			Timestamp:  ticker.Timestamp,
		})
	}

	// Calculate VWAP for each symbol
	vwapResults := app.vwapCalc.CalculateBatch(pricesBySymbol)

	// Store VWAP prices in ClickHouse
	app.storeVWAPPrices(ctx, vwapResults)
}

func (app *Application) storeVWAPPrices(ctx context.Context, results map[string]*calculator.VWAPResult) {
	if len(results) == 0 {
		return
	}

	// Use the storage service to store VWAP results
	if err := app.vwapStorage.StoreVWAPResults(ctx, results); err != nil {
		app.logger.Error("Failed to store VWAP results", zap.Error(err))
	}
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
	}
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

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
