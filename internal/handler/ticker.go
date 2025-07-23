// package handler

// import (
// 	"database/sql"
// 	"net/http"
// 	"strings"
// 	"time"

// 	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
// 	"github.com/ashmitsharp/trading/internal/db"
// 	"github.com/ashmitsharp/trading/internal/models"
// 	"github.com/gin-gonic/gin"
// 	"go.uber.org/zap"
// )

// type TickerHandler struct {
// 	clickhouseConn driver.Conn
// 	postgresDB     *sql.DB
// 	logger         *zap.Logger
// }

// func NewTickerHandler(clickhouseConn driver.Conn, postgresDB *sql.DB, logger *zap.Logger) *TickerHandler {
// 	return &TickerHandler{
// 		clickhouseConn: clickhouseConn,
// 		postgresDB:     postgresDB,
// 		logger:         logger,
// 	}
// }

// func (h *TickerHandler) GetTicker(c *gin.Context) {
// 	prices, err := db.GetLatestPrices(h.clickhouseConn)
// 	if err != nil {
// 		h.logger.Error("Failed to get latest prices", zap.Error(err))
// 		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
// 			Error:     "database_error",
// 			Message:   "Failed to retrieve ticker data",
// 			Code:      http.StatusInternalServerError,
// 			Timestamp: time.Now().Unix(),
// 		})
// 		return
// 	}

// 	tokens, err := db.GetAllTokens(h.postgresDB)
// 	if err != nil {
// 		h.logger.Error("Failed to get token metadata", zap.Error(err))
// 	}

// 	tokenMap := make(map[string]db.Token)
// 	for _, token := range tokens {
// 		tokenMap[token.Symbol] = token
// 	}

// 	var tickers []models.TickerResponse
// 	for symbol, price := range prices {
// 		ticker := models.TickerResponse{
// 			Symbol:    symbol,
// 			Price:     price.Price.InexactFloat64(),
// 			Timestamp: price.Timestamp,
// 		}

// 		if token, exists := tokenMap[symbol]; exists {
// 			ticker.Name = token.Name
// 			ticker.Category = token.Category
// 		}

// 		stats, err := h.get24hStats(symbol)
// 		if err == nil {
// 			ticker.PriceChange24h = stats.PriceChange
// 			ticker.PriceChangePercent24h = stats.PriceChangePercent
// 			ticker.Volume24h = stats.Volume
// 			ticker.High24h = stats.High
// 			ticker.Low24h = stats.Low
// 		}

// 		tickers = append(tickers, ticker)
// 	}

// 	c.JSON(http.StatusOK, models.APIResponse{
// 		Success:   true,
// 		Data:      tickers,
// 		Timestamp: time.Now().Unix(),
// 	})
// }

// // GetTickerBySymbol returns the latest price for a specific symbol
// func (h *TickerHandler) GetTickerBySymbol(c *gin.Context) {
// 	symbol := strings.ToUpper(c.Param("symbol"))
// 	if symbol == "" {
// 		c.JSON(http.StatusBadRequest, models.ErrorResponse{
// 			Error:     "invalid_symbol",
// 			Message:   "Symbol parameter is required",
// 			Code:      http.StatusBadRequest,
// 			Timestamp: time.Now().Unix(),
// 		})
// 		return
// 	}

// 	// Get latest prices from ClickHouse
// 	prices, err := db.GetLatestPrices(h.clickhouseConn)
// 	if err != nil {
// 		h.logger.Error("Failed to get latest prices", zap.Error(err))
// 		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
// 			Error:     "database_error",
// 			Message:   "Failed to retrieve ticker data",
// 			Code:      http.StatusInternalServerError,
// 			Timestamp: time.Now().Unix(),
// 		})
// 		return
// 	}

// 	// Check if symbol exists
// 	price, exists := prices[symbol]
// 	if !exists {
// 		c.JSON(http.StatusNotFound, models.ErrorResponse{
// 			Error:     "symbol_not_found",
// 			Message:   "Trading pair not found",
// 			Code:      http.StatusNotFound,
// 			Timestamp: time.Now().Unix(),
// 		})
// 		return
// 	}

// 	// Build ticker response
// 	ticker := models.TickerResponse{
// 		Symbol:    symbol,
// 		Price:     price.Price.InexactFloat64(),
// 		Timestamp: price.Timestamp,
// 	}

// 	// Get token metadata
// 	token, err := db.GetTokenBySymbol(h.postgresDB, symbol)
// 	if err == nil {
// 		ticker.Name = token.Name
// 		ticker.Category = token.Category
// 	}

// 	// Calculate 24h stats
// 	stats, err := h.get24hStats(symbol)
// 	if err == nil {
// 		ticker.PriceChange24h = stats.PriceChange
// 		ticker.PriceChangePercent24h = stats.PriceChangePercent
// 		ticker.Volume24h = stats.Volume
// 		ticker.High24h = stats.High
// 		ticker.Low24h = stats.Low
// 	}

// 	c.JSON(http.StatusOK, models.APIResponse{
// 		Success:   true,
// 		Data:      ticker,
// 		Timestamp: time.Now().Unix(),
// 	})
// }

// type Stats struct {
// 	PriceChange        float64
// 	PriceChangePercent float64
// 	Volume             float64
// 	High               float64
// 	Low                float64
// }

// // get24hStats calculates 24-hour statistics for a symbol
// func (h *TickerHandler) get24hStats(symbol string) (*Stats, error) {
// 	now := time.Now()
// 	yesterday := now.Add(-24 * time.Hour)

// 	// Get 24h data from ClickHouse
// 	ohlcvData, err := db.GetOHLCVData(
// 		h.clickhouseConn,
// 		symbol,
// 		yesterday.Unix()*1000, // Convert to milliseconds
// 		now.Unix()*1000,
// 		"1h", // 1-hour intervals for better granularity
// 	)
// 	if err != nil || len(ohlcvData) == 0 {
// 		return nil, err
// 	}

// 	// Calculate stats from OHLCV data
// 	var high, low, volume float64
// 	var open, close float64

// 	first := true
// 	for _, data := range ohlcvData {
// 		if first {
// 			high = data.High.InexactFloat64()
// 			low = data.Low.InexactFloat64()
// 			open = data.Open.InexactFloat64()
// 			first = false
// 		}

// 		if data.High.InexactFloat64() > high {
// 			high = data.High.InexactFloat64()
// 		}
// 		if data.Low.InexactFloat64() < low {
// 			low = data.Low.InexactFloat64()
// 		}

// 		volume += data.Volume.InexactFloat64()
// 		close = data.Close.InexactFloat64() // Last close price
// 	}

// 	// Calculate price change and percentage
// 	priceChange := close - open
// 	priceChangePercent := 0.0
// 	if open > 0 {
// 		priceChangePercent = (priceChange / open) * 100
// 	}

// 	return &Stats{
// 		PriceChange:        priceChange,
// 		PriceChangePercent: priceChangePercent,
// 		Volume:             volume,
// 		High:               high,
// 		Low:                low,
// 	}, nil
// }

package handler

import (
	"database/sql"
	"net/http"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/ashmitsharp/trading/internal/db"
	"github.com/ashmitsharp/trading/internal/models"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type TickerHandler struct {
	clickhouseConn driver.Conn
	postgresDB     *sql.DB
	logger         *zap.Logger
}

func NewTickerHandler(clickhouseConn driver.Conn, postgresDB *sql.DB, logger *zap.Logger) *TickerHandler {
	if clickhouseConn == nil {
		panic("clickhouseConn cannot be nil")
	}
	if postgresDB == nil {
		panic("postgresDB cannot be nil")
	}
	return &TickerHandler{
		clickhouseConn: clickhouseConn,
		postgresDB:     postgresDB,
		logger:         logger,
	}
}

func (h *TickerHandler) GetTicker(c *gin.Context) {
	prices, err := db.GetLatestPrices(h.clickhouseConn)
	if err != nil {
		h.logger.Error("Failed to get latest prices", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:     "database_error",
			Message:   "Failed to retrieve ticker data",
			Code:      http.StatusInternalServerError,
			Timestamp: time.Now().Unix(),
		})
		return
	}
	if prices == nil {
		h.logger.Error("GetLatestPrices returned nil map")
		prices = make(map[string]db.LatestPrice)
	}

	tokenMap := make(map[string]db.Token)
	if h.postgresDB != nil {
		tokens, err := db.GetAllTokens(h.postgresDB)
		if err != nil {
			h.logger.Error("Failed to get token metadata", zap.Error(err))
		} else {
			for _, token := range tokens {
				tokenMap[token.Symbol] = token
			}
		}
	}

	var tickers []models.TickerResponse
	for symbol, price := range prices {
		ticker := models.TickerResponse{
			Symbol:    symbol,
			Price:     price.Price.InexactFloat64(),
			Timestamp: price.Timestamp,
		}
		if token, exists := tokenMap[symbol]; exists {
			ticker.Name = token.Name
			ticker.Category = token.Category
		}
		stats, err := h.get24hStats(symbol)
		if err == nil && stats != nil {
			ticker.PriceChange24h = stats.PriceChange
			ticker.PriceChangePercent24h = stats.PriceChangePercent
			ticker.Volume24h = stats.Volume
			ticker.High24h = stats.High
			ticker.Low24h = stats.Low
		} else if err != nil {
			h.logger.Debug("Failed to get 24h stats for symbol",
				zap.String("symbol", symbol),
				zap.Error(err))
		}
		tickers = append(tickers, ticker)
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success:   true,
		Data:      tickers,
		Timestamp: time.Now().Unix(),
	})
}

// GetTickerBySymbol returns the latest price for a specific symbol
func (h *TickerHandler) GetTickerBySymbol(c *gin.Context) {
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

	// Get latest prices from ClickHouse
	prices, err := db.GetLatestPrices(h.clickhouseConn)
	if err != nil {
		h.logger.Error("Failed to get latest prices", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:     "database_error",
			Message:   "Failed to retrieve ticker data",
			Code:      http.StatusInternalServerError,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	// Check if symbol exists
	price, exists := prices[symbol]
	if !exists {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:     "symbol_not_found",
			Message:   "Trading pair not found",
			Code:      http.StatusNotFound,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	// Build ticker response
	ticker := models.TickerResponse{
		Symbol:    symbol,
		Price:     price.Price.InexactFloat64(),
		Timestamp: price.Timestamp,
	}

	// Get token metadata with nil check
	if h.postgresDB != nil {
		token, err := db.GetTokenBySymbol(h.postgresDB, symbol)
		if err == nil {
			ticker.Name = token.Name
			ticker.Category = token.Category
		} else {
			h.logger.Debug("Failed to get token metadata",
				zap.String("symbol", symbol),
				zap.Error(err))
		}
	}

	// Calculate 24h stats with error handling
	stats, err := h.get24hStats(symbol)
	if err == nil && stats != nil {
		ticker.PriceChange24h = stats.PriceChange
		ticker.PriceChangePercent24h = stats.PriceChangePercent
		ticker.Volume24h = stats.Volume
		ticker.High24h = stats.High
		ticker.Low24h = stats.Low
	} else if err != nil {
		h.logger.Debug("Failed to get 24h stats for symbol",
			zap.String("symbol", symbol),
			zap.Error(err))
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success:   true,
		Data:      ticker,
		Timestamp: time.Now().Unix(),
	})
}

type Stats struct {
	PriceChange        float64
	PriceChangePercent float64
	Volume             float64
	High               float64
	Low                float64
}

// get24hStats calculates 24-hour statistics for a symbol
func (h *TickerHandler) get24hStats(symbol string) (*Stats, error) {
	now := time.Now()
	yesterday := now.Add(-24 * time.Hour)

	// Get 24h data from ClickHouse
	ohlcvData, err := db.GetOHLCVData(
		h.clickhouseConn,
		symbol,
		yesterday.Unix()*1000, // Convert to milliseconds
		now.Unix()*1000,
		"1h", // 1-hour intervals for better granularity
	)
	if err != nil || len(ohlcvData) == 0 {
		return nil, err
	}

	// Calculate stats from OHLCV data
	var high, low, volume float64
	var open, close float64

	first := true
	for _, data := range ohlcvData {
		if first {
			high = data.High.InexactFloat64()
			low = data.Low.InexactFloat64()
			open = data.Open.InexactFloat64()
			first = false
		}

		if data.High.InexactFloat64() > high {
			high = data.High.InexactFloat64()
		}
		if data.Low.InexactFloat64() < low {
			low = data.Low.InexactFloat64()
		}

		volume += data.Volume.InexactFloat64()
		close = data.Close.InexactFloat64() // Last close price
	}

	// Calculate price change and percentage
	priceChange := close - open
	priceChangePercent := 0.0
	if open > 0 {
		priceChangePercent = (priceChange / open) * 100
	}

	return &Stats{
		PriceChange:        priceChange,
		PriceChangePercent: priceChangePercent,
		Volume:             volume,
		High:               high,
		Low:                low,
	}, nil
}
