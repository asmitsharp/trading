package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/ashmitsharp/trading/internal/config"
	"github.com/ashmitsharp/trading/internal/db"
	"github.com/ashmitsharp/trading/internal/handler"
	"github.com/ashmitsharp/trading/internal/ingester"
	"github.com/ashmitsharp/trading/internal/scheduler"
	"github.com/ashmitsharp/trading/pkg/utils"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"go.uber.org/zap"
)

// @title           Crypto Backend API
// @version         1.0
// @description     Real-time cryptocurrency data ingestion and API service
// @termsOfService  http://swagger.io/terms/

// @contact.name   API Support
// @contact.url    http://www.swagger.io/support
// @contact.email  support@swagger.io

// @license.name  Apache 2.0
// @license.url   http://www.apache.org/licenses/LICENSE-2.0.html

// @host      localhost:8080
// @BasePath  /api/v1

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using system environment variables")
	}

	// Initialize logger
	logger := utils.InitLogger()
	defer func() {
		if err := logger.Sync(); err != nil {
			log.Printf("Failed to sync logger: %v", err)
		}
	}()

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("Failed to load configuration", zap.Error(err))
	}

	logger.Info("Starting application with configuration",
		zap.String("clickhouse_host", cfg.ClickHouse.Host),
		zap.String("postgres_host", cfg.Postgres.Host),
		zap.String("environment", cfg.Server.Environment))

	// Initialize ClickHouse
	logger.Info("Initializing ClickHouse connection...")
	clickhouseDB, err := db.InitClickHouse(cfg.ClickHouse)
	if err != nil {
		logger.Fatal("Failed to initialize ClickHouse", zap.Error(err))
	}
	defer clickhouseDB.Close()
	logger.Info("ClickHouse connection established successfully")

	// Initialize PostgreSQL
	logger.Info("Initializing PostgreSQL connection...")
	postgresDB, err := db.InitPostgres(cfg.Postgres)
	if err != nil {
		logger.Fatal("Failed to initialize PostgreSQL", zap.Error(err))
	}
	defer postgresDB.Close()
	logger.Info("PostgreSQL connection established successfully")

	// Test database connections
	logger.Info("Testing database connections...")
	if err := testDatabaseConnections(clickhouseDB, postgresDB, logger); err != nil {
		logger.Fatal("Database connection test failed", zap.Error(err))
	}
	logger.Info("All database connections are healthy")

	// Initialize schemas
	logger.Info("Initializing database schemas...")
	if err := db.InitSchemas(clickhouseDB, postgresDB); err != nil {
		logger.Fatal("Failed to initialize database schemas", zap.Error(err))
	}
	logger.Info("Database schemas initialized successfully")

	// Start data ingester
	logger.Info("Starting Binance data ingester...")
	binanceIngester := ingester.NewBinanceIngester(clickhouseDB, logger, cfg.Binance)
	go binanceIngester.Start()

	// Start scheduler
	logger.Info("Starting cron scheduler...")
	cronScheduler := scheduler.NewScheduler(postgresDB, logger)
	cronScheduler.Start()
	defer cronScheduler.Stop()

	// Initialize Gin router
	if cfg.Server.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.New()

	// Add middleware
	router.Use(gin.Logger())
	router.Use(gin.Recovery())
	router.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusOK)
			return
		}
		c.Next()
	})

	// Initialize handlers with connection validation
	logger.Info("Initializing API handlers...")
	if clickhouseDB == nil {
		logger.Fatal("ClickHouse connection is nil")
	}
	if postgresDB == nil {
		logger.Fatal("PostgreSQL connection is nil")
	}

	tickerHandler := handler.NewTickerHandler(clickhouseDB, postgresDB, logger)
	ohlcvHandler := handler.NewOHLCVHandler(clickhouseDB, logger)
	logger.Info("API handlers initialized successfully")

	// API routes
	v1 := router.Group("/api/v1")
	{
		v1.GET("/ticker", tickerHandler.GetTicker)
		v1.GET("/ticker/:symbol", tickerHandler.GetTickerBySymbol)
		v1.GET("/ohlcv/:symbol", ohlcvHandler.GetOHLCV)
		v1.GET("/ohlcv/symbols", ohlcvHandler.GetSupportedSymbols)
	}

	// Health check endpoint
	router.GET("/health", func(c *gin.Context) {
		// Test both database connections
		clickhouseHealthy := testClickHouseConnection(clickhouseDB)
		postgresHealthy := testPostgresConnection(postgresDB)

		status := "ok"
		httpStatus := http.StatusOK

		if !clickhouseHealthy || !postgresHealthy {
			status = "degraded"
			httpStatus = http.StatusServiceUnavailable
		}

		c.JSON(httpStatus, gin.H{
			"status":             status,
			"timestamp":          time.Now().Unix(),
			"version":            "1.0.0",
			"clickhouse_healthy": clickhouseHealthy,
			"postgres_healthy":   postgresHealthy,
		})
	})

	// Swagger documentation
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Create HTTP server
	srv := &http.Server{
		Addr:           cfg.Server.Port,
		Handler:        router,
		ReadTimeout:    cfg.Server.ReadTimeout,
		WriteTimeout:   cfg.Server.WriteTimeout,
		MaxHeaderBytes: 1 << 20, // 1 MB
	}

	// Start server in a goroutine
	go func() {
		logger.Info("Starting HTTP server", zap.String("port", cfg.Server.Port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Failed to start server", zap.Error(err))
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server...")

	// Stop ingester
	binanceIngester.Stop()

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("Server forced to shutdown", zap.Error(err))
	}

	logger.Info("Server exited")
}

// testDatabaseConnections tests all database connections
func testDatabaseConnections(clickhouseDB driver.Conn, postgresDB *sql.DB, logger *zap.Logger) error {
	// Test ClickHouse connection
	if !testClickHouseConnection(clickhouseDB) {
		return fmt.Errorf("ClickHouse connection test failed")
	}

	// Test PostgreSQL connection
	if !testPostgresConnection(postgresDB) {
		return fmt.Errorf("PostgreSQL connection test failed")
	}

	return nil
}

// testClickHouseConnection tests ClickHouse connection
func testClickHouseConnection(conn driver.Conn) bool {
	if conn == nil {
		return false
	}

	// Try to get latest prices to test the connection
	prices, err := db.GetLatestPrices(conn)
	if err != nil {
		return false
	}

	// Connection is healthy if we can query (even if no data)
	_ = prices
	return true
}

// testPostgresConnection tests PostgreSQL connection
func testPostgresConnection(conn *sql.DB) bool {
	if conn == nil {
		return false
	}

	// Try to get all tokens to test the connection
	tokens, err := db.GetAllTokens(conn)
	if err != nil {
		return false
	}

	// Connection is healthy if we can query (even if no data)
	_ = tokens
	return true
}
