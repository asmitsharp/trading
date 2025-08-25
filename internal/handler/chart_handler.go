package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/ashmitsharp/trading/internal/chart_api"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// ChartHandler handles chart-related HTTP requests
type ChartHandler struct {
	chartService *chart_api.ChartService
	logger       *zap.Logger
}

// NewChartHandler creates a new chart handler
func NewChartHandler(chartService *chart_api.ChartService, logger *zap.Logger) *ChartHandler {
	return &ChartHandler{
		chartService: chartService,
		logger:       logger,
	}
}

// HandleWebSocket handles WebSocket upgrade requests
func (ch *ChartHandler) HandleWebSocket(c *gin.Context) {
	wsServer := ch.chartService.GetWebSocketServer()
	wsServer.HandleConnection(c.Writer, c.Request)
}

// GetHistoricalCandles handles GET /api/v1/chart/candles
func (ch *ChartHandler) GetHistoricalCandles(c *gin.Context) {
	var request chart_api.HistoricalCandleRequest
	
	// Parse query parameters
	if baseTokenIDStr := c.Query("base_token_id"); baseTokenIDStr != "" {
		if id, err := strconv.ParseUint(baseTokenIDStr, 10, 32); err == nil {
			request.BaseTokenID = uint32(id)
		}
	}
	
	if quoteTokenIDStr := c.Query("quote_token_id"); quoteTokenIDStr != "" {
		if id, err := strconv.ParseUint(quoteTokenIDStr, 10, 32); err == nil {
			request.QuoteTokenID = uint32(id)
		}
	}
	
	request.Timeframe = c.DefaultQuery("timeframe", "1h")
	
	if startTimeStr := c.Query("start_time"); startTimeStr != "" {
		if startTime, err := strconv.ParseInt(startTimeStr, 10, 64); err == nil {
			request.StartTime = startTime
		}
	}
	
	if endTimeStr := c.Query("end_time"); endTimeStr != "" {
		if endTime, err := strconv.ParseInt(endTimeStr, 10, 64); err == nil {
			request.EndTime = endTime
		}
	}
	
	if limitStr := c.Query("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil {
			request.Limit = limit
		}
	}

	// Validate required parameters
	if request.BaseTokenID == 0 {
		ch.respondError(c, http.StatusBadRequest, "base_token_id is required")
		return
	}
	
	if request.QuoteTokenID == 0 {
		ch.respondError(c, http.StatusBadRequest, "quote_token_id is required")
		return
	}

	ch.logger.Info("Historical candles request",
		zap.Uint32("base_token_id", request.BaseTokenID),
		zap.Uint32("quote_token_id", request.QuoteTokenID),
		zap.String("timeframe", request.Timeframe),
		zap.Int("limit", request.Limit))

	// Get historical data
	historicalAPI := ch.chartService.GetHistoricalAPI()
	response, err := historicalAPI.GetHistoricalCandles(c.Request.Context(), request)
	if err != nil {
		ch.logger.Error("Failed to get historical candles", zap.Error(err))
		ch.respondError(c, http.StatusInternalServerError, "Failed to retrieve candle data")
		return
	}

	c.JSON(http.StatusOK, response)
}

// GetSupportedTimeframes handles GET /api/v1/chart/timeframes
func (ch *ChartHandler) GetSupportedTimeframes(c *gin.Context) {
	historicalAPI := ch.chartService.GetHistoricalAPI()
	
	response := gin.H{
		"success":    true,
		"timeframes": historicalAPI.GetSupportedTimeframes(),
		"limits":     historicalAPI.GetTimeframeLimits(),
		"timestamp":  time.Now().Unix() * 1000,
	}
	
	c.JSON(http.StatusOK, response)
}

// GetChartStats handles GET /api/v1/chart/stats
func (ch *ChartHandler) GetChartStats(c *gin.Context) {
	stats := ch.chartService.GetStats()
	
	response := gin.H{
		"success":   true,
		"stats":     stats,
		"timestamp": time.Now().Unix() * 1000,
	}
	
	c.JSON(http.StatusOK, response)
}

// GetLatestPrice handles GET /api/v1/chart/price/{baseTokenID}/{quoteTokenID}
func (ch *ChartHandler) GetLatestPrice(c *gin.Context) {
	baseTokenIDStr := c.Param("baseTokenID")
	quoteTokenIDStr := c.Param("quoteTokenID")
	
	baseTokenID, err := strconv.ParseUint(baseTokenIDStr, 10, 32)
	if err != nil {
		ch.respondError(c, http.StatusBadRequest, "Invalid base token ID")
		return
	}
	
	quoteTokenID, err := strconv.ParseUint(quoteTokenIDStr, 10, 32)
	if err != nil {
		ch.respondError(c, http.StatusBadRequest, "Invalid quote token ID")
		return
	}

	// Get latest price using the historical API
	historicalAPI := ch.chartService.GetHistoricalAPI()
	vwapPrice, volume, exchangeCount, timestamp, err := historicalAPI.GetLatestPrice(
		c.Request.Context(), uint32(baseTokenID), uint32(quoteTokenID))
	
	if err != nil {
		ch.logger.Error("Failed to get latest price", zap.Error(err))
		ch.respondError(c, http.StatusNotFound, "Price data not found")
		return
	}

	response := gin.H{
		"success": true,
		"data": gin.H{
			"base_token_id":  uint32(baseTokenID),
			"quote_token_id": uint32(quoteTokenID),
			"symbol":         fmt.Sprintf("%d/%d", baseTokenID, quoteTokenID),
			"price":          vwapPrice.String(),
			"vwap_price":     vwapPrice.String(),
			"volume_24h":     volume.String(),
			"exchange_count": exchangeCount,
			"timestamp":      timestamp.Unix() * 1000,
		},
		"timestamp": time.Now().Unix() * 1000,
	}

	c.JSON(http.StatusOK, response)
}

// respondError sends an error response
func (ch *ChartHandler) respondError(c *gin.Context, status int, message string) {
	response := gin.H{
		"success":   false,
		"error":     message,
		"timestamp": time.Now().Unix() * 1000,
	}
	
	c.JSON(status, response)
}