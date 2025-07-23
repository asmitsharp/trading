package handler

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/ashmitsharp/trading/internal/db"
	"github.com/ashmitsharp/trading/internal/models"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// OHLCVHandler handles OHLCV (candlestick) data endpoints
type OHLCVHandler struct {
	clickhouseConn driver.Conn
	logger         *zap.Logger
}

// NewOHLCVHandler creates a new OHLCV handler
func NewOHLCVHandler(clickhouseConn driver.Conn, logger *zap.Logger) *OHLCVHandler {
	return &OHLCVHandler{
		clickhouseConn: clickhouseConn,
		logger:         logger,
	}
}

// GetOHLCV returns OHLCV candlestick data for a symbol
// @Summary Get OHLCV candlestick data
// @Description Get OHLCV (Open, High, Low, Close, Volume) candlestick data for a trading pair
// @Tags ohlcv
// @Accept json
// @Produce json
// @Param symbol path string true "Trading pair symbol (e.g., BTCUSDT)"
// @Param interval query string false "Candlestick interval" Enums(1m, 5m, 15m, 1h, 4h, 1d) default(1h)
// @Param from query int false "Start time (Unix timestamp in seconds)"
// @Param to query int false "End time (Unix timestamp in seconds)"
// @Param limit query int false "Maximum number of candlesticks to return" default(100) maximum(1000)
// @Success 200 {object} models.APIResponse{data=[]models.OHLCVResponse} "Success"
// @Failure 400 {object} models.ErrorResponse "Bad request"
// @Failure 404 {object} models.ErrorResponse "Symbol not found"
// @Failure 500 {object} models.ErrorResponse "Internal server error"
// @Router /ohlcv/{symbol} [get]
func (h *OHLCVHandler) GetOHLCV(c *gin.Context) {
	symbol := strings.ToUpper(c.Param("symbol"))
	if symbol == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:     "invalid_symbol",
			Message:   "Symbol parameter is required",
			Code:      http.StatusBadRequest,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	// New: Check for 'minutes' param
	minutesStr := c.Query("minutes")
	var from, to int64
	now := time.Now().Unix()
	if minutesStr != "" {
		minutes, err := strconv.Atoi(minutesStr)
		if err != nil || minutes <= 0 {
			c.JSON(http.StatusBadRequest, models.ErrorResponse{
				Error:     "invalid_minutes",
				Message:   "Minutes must be a positive integer",
				Code:      http.StatusBadRequest,
				Timestamp: time.Now().Unix(),
			})
			return
		}
		from = now - int64(minutes*60)
		to = now
	} else {
		// Fallback to existing logic (parse 'from' and 'to' from query)
		params, err := h.parseOHLCVParams(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, models.ErrorResponse{
				Error:     "invalid_parameters",
				Message:   err.Error(),
				Code:      http.StatusBadRequest,
				Timestamp: time.Now().Unix(),
			})
			return
		}
		from = params.From
		to = params.To
	}

	// Parse interval and limit as before
	interval := c.DefaultQuery("interval", "1h")
	limitStr := c.DefaultQuery("limit", "100")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 || limit > 1000 {
		limit = 100
	}

	// Validate time range
	if to <= from {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:     "invalid_time_range",
			Message:   "End time must be after start time",
			Code:      http.StatusBadRequest,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	// Check if time range is not too large
	maxRange := h.getMaxTimeRange(interval)
	if to-from > maxRange {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:     "time_range_too_large",
			Message:   "Time range exceeds maximum allowed for this interval",
			Code:      http.StatusBadRequest,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	// Get OHLCV data from ClickHouse
	ohlcvData, err := db.GetOHLCVData(
		h.clickhouseConn,
		symbol,
		from, // Convert to milliseconds
		to,
		interval,
	)
	if err != nil {
		h.logger.Error("Failed to get OHLCV data",
			zap.Error(err),
			zap.String("symbol", symbol),
			zap.String("interval", interval))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:     "database_error",
			Message:   "Failed to retrieve OHLCV data",
			Code:      http.StatusInternalServerError,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	// Check if symbol has any data
	if len(ohlcvData) == 0 {
		// Check if symbol exists at all
		if !h.symbolExists(symbol) {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:     "symbol_not_found",
				Message:   "Trading pair not found",
				Code:      http.StatusNotFound,
				Timestamp: time.Now().Unix(),
			})
			return
		}

		// Symbol exists but no data in time range
		c.JSON(http.StatusOK, models.APIResponse{
			Success:   true,
			Data:      []models.OHLCVResponse{},
			Message:   "No data found for the specified time range",
			Timestamp: time.Now().Unix(),
		})
		return
	}

	// Apply limit if specified
	if limit > 0 && len(ohlcvData) > limit {
		ohlcvData = ohlcvData[:limit]
	}

	// Convert to response format
	var response []models.OHLCVResponse
	for _, data := range ohlcvData {
		response = append(response, models.OHLCVResponse{
			Symbol:      data.Symbol,
			Interval:    interval,
			Timestamp:   data.Timestamp / 1000, // Convert back to seconds
			Open:        data.Open.InexactFloat64(),
			High:        data.High.InexactFloat64(),
			Low:         data.Low.InexactFloat64(),
			Close:       data.Close.InexactFloat64(),
			Volume:      data.Volume.InexactFloat64(),
			TradesCount: int64(data.TradesCount),
		})
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success:   true,
		Data:      response,
		Timestamp: time.Now().Unix(),
	})
}

// OHLCVParams represents parsed OHLCV query parameters
type OHLCVParams struct {
	Interval string
	From     int64
	To       int64
	Limit    int
}

// parseOHLCVParams parses and validates OHLCV query parameters
func (h *OHLCVHandler) parseOHLCVParams(c *gin.Context) (*OHLCVParams, error) {
	params := &OHLCVParams{}

	// Parse interval (default: 1h)
	params.Interval = c.DefaultQuery("interval", "1h")
	if !h.isValidInterval(params.Interval) {
		return nil, &ValidationError{Field: "interval", Message: "Invalid interval. Supported: 1m, 5m, 15m, 1h, 4h, 1d"}
	}

	// Parse time range
	now := time.Now()

	// Parse 'from' parameter (default: 24 hours ago)
	fromStr := c.Query("from")
	if fromStr != "" {
		from, err := strconv.ParseInt(fromStr, 10, 64)
		if err != nil {
			return nil, &ValidationError{Field: "from", Message: "Invalid from timestamp"}
		}
		params.From = from
	} else {
		params.From = now.Add(-24 * time.Hour).Unix()
	}

	// Parse 'to' parameter (default: now)
	toStr := c.Query("to")
	if toStr != "" {
		to, err := strconv.ParseInt(toStr, 10, 64)
		if err != nil {
			return nil, &ValidationError{Field: "to", Message: "Invalid to timestamp"}
		}
		params.To = to
	} else {
		params.To = now.Unix()
	}

	// Parse limit (default: 100, max: 1000)
	limitStr := c.DefaultQuery("limit", "100")
	if limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil {
			return nil, &ValidationError{Field: "limit", Message: "Invalid limit value"}
		}
		if limit < 1 || limit > 1000 {
			return nil, &ValidationError{Field: "limit", Message: "Limit must be between 1 and 1000"}
		}
		params.Limit = limit
	}

	return params, nil
}

// ValidationError represents a parameter validation error
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}

// isValidInterval checks if the interval is supported
func (h *OHLCVHandler) isValidInterval(interval string) bool {
	validIntervals := map[string]bool{
		"1m":  true,
		"5m":  true,
		"15m": true,
		"1h":  true,
		"4h":  true,
		"1d":  true,
	}
	return validIntervals[interval]
}

// getMaxTimeRange returns the maximum allowed time range for an interval (in seconds)
func (h *OHLCVHandler) getMaxTimeRange(interval string) int64 {
	maxRanges := map[string]int64{
		"1m":  7 * 24 * 3600,       // 7 days
		"5m":  30 * 24 * 3600,      // 30 days
		"15m": 90 * 24 * 3600,      // 90 days
		"1h":  365 * 24 * 3600,     // 1 year
		"4h":  2 * 365 * 24 * 3600, // 2 years
		"1d":  5 * 365 * 24 * 3600, // 5 years
	}

	if maxRange, exists := maxRanges[interval]; exists {
		return maxRange
	}
	return 30 * 24 * 3600 // Default: 30 days
}

// symbolExists checks if a symbol has any data in the database
func (h *OHLCVHandler) symbolExists(symbol string) bool {
	// Get latest prices to check if symbol exists
	prices, err := db.GetLatestPrices(h.clickhouseConn)
	if err != nil {
		return false
	}

	_, exists := prices[symbol]
	return exists
}

// GetSupportedSymbols returns a list of supported symbols
// @Summary Get supported trading pairs
// @Description Get a list of all supported trading pairs
// @Tags ohlcv
// @Accept json
// @Produce json
// @Success 200 {object} models.APIResponse{data=[]string} "Success"
// @Failure 500 {object} models.ErrorResponse "Internal server error"
// @Router /ohlcv/symbols [get]
func (h *OHLCVHandler) GetSupportedSymbols(c *gin.Context) {
	// Get latest prices to extract supported symbols
	prices, err := db.GetLatestPrices(h.clickhouseConn)
	if err != nil {
		h.logger.Error("Failed to get supported symbols", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:     "database_error",
			Message:   "Failed to retrieve supported symbols",
			Code:      http.StatusInternalServerError,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	var symbols []string
	for symbol := range prices {
		symbols = append(symbols, symbol)
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success:   true,
		Data:      symbols,
		Timestamp: time.Now().Unix(),
	})
}
