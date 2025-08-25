package main

import (
	"database/sql"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ashmitsharp/trading/internal/exchanges"
	"github.com/ashmitsharp/trading/internal/handler"
	"github.com/ashmitsharp/trading/internal/live_ohlcv"
	"github.com/ashmitsharp/trading/internal/reliability"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	_ "github.com/lib/pq"
)

func main() {
	// Initialize logger
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	logger.Info("Starting live OHLCV server")

	// Database connections
	postgres, err := sql.Open("postgres", 
		"postgres://username:password@localhost/trading?sslmode=disable")
	if err != nil {
		logger.Fatal("Failed to connect to PostgreSQL", zap.Error(err))
	}
	defer postgres.Close()

	clickhouseConn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{"localhost:9000"},
		Auth: clickhouse.Auth{
			Database: "trading",
		},
	})
	if err != nil {
		logger.Fatal("Failed to connect to ClickHouse", zap.Error(err))
	}
	defer clickhouseConn.Close()

	// Initialize components
	exchFactory := exchanges.NewFactory(logger)
	scorer := reliability.NewScorer(logger)

	// Initialize live OHLCV service
	liveOHLCVService := live_ohlcv.NewService(
		clickhouseConn,
		exchFactory,
		scorer,
		logger,
	)

	// Start live OHLCV collection and aggregation
	if err := liveOHLCVService.Start(); err != nil {
		logger.Fatal("Failed to start live OHLCV service", zap.Error(err))
	}

	// Setup HTTP server
	router := gin.New()
	router.Use(gin.Logger())
	router.Use(gin.Recovery())

	// Initialize handlers
	liveOHLCVHandler := handler.NewLiveOHLCVHandler(liveOHLCVService, logger)

	// Setup routes
	api := router.Group("/api/v1")
	{
		// OHLCV endpoints
		ohlcv := api.Group("/ohlcv")
		{
			ohlcv.GET("/timeframes", liveOHLCVHandler.GetSupportedTimeframes)
			ohlcv.GET("/limits", liveOHLCVHandler.GetCandleLimits)
			ohlcv.GET("/:baseTokenID/:quoteTokenID", liveOHLCVHandler.GetCandles)
			ohlcv.GET("/:baseTokenID/:quoteTokenID/latest", liveOHLCVHandler.GetLatestCandle)
		}

		// Health check
		api.GET("/health", func(c *gin.Context) {
			c.JSON(200, gin.H{
				"status":    "ok",
				"service":   "live-ohlcv",
				"timestamp": gin.H{},
			})
		})
	}

	// Start HTTP server in goroutine
	go func() {
		logger.Info("Starting HTTP server on :8080")
		if err := router.Run(":8080"); err != nil {
			logger.Error("HTTP server error", zap.Error(err))
		}
	}()

	// Wait for interrupt signal to gracefully shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down live OHLCV server...")

	// Stop live OHLCV service
	liveOHLCVService.Stop()

	logger.Info("Live OHLCV server shutdown complete")
}