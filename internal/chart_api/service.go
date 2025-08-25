package chart_api

import (
	"github.com/ClickHouse/clickhouse-go/v2"
	"go.uber.org/zap"
)

// ChartService combines WebSocket streaming and historical API
type ChartService struct {
	wsServer       *WebSocketServer
	historicalAPI  *HistoricalAPI
	liveDataService *LiveDataService
	logger         *zap.Logger
}

// NewChartService creates a new chart service
func NewChartService(clickhouse clickhouse.Conn, logger *zap.Logger) *ChartService {
	// Initialize WebSocket server
	wsServer := NewWebSocketServer(logger)
	
	// Initialize historical API
	historicalAPI := NewHistoricalAPI(clickhouse, logger)
	
	// Initialize live data service
	liveDataService := NewLiveDataService(clickhouse, wsServer, logger)
	
	return &ChartService{
		wsServer:        wsServer,
		historicalAPI:   historicalAPI,
		liveDataService: liveDataService,
		logger:          logger,
	}
}

// Start starts all chart services
func (cs *ChartService) Start() error {
	cs.logger.Info("Starting chart service")
	
	// Start live data streaming
	if err := cs.liveDataService.Start(); err != nil {
		return err
	}
	
	cs.logger.Info("Chart service started successfully")
	return nil
}

// Stop stops all chart services
func (cs *ChartService) Stop() {
	cs.logger.Info("Stopping chart service")
	
	// Stop live data service
	cs.liveDataService.Stop()
	
	// Shutdown WebSocket server
	cs.wsServer.Shutdown()
	
	cs.logger.Info("Chart service stopped")
}

// GetWebSocketServer returns the WebSocket server for HTTP handler registration
func (cs *ChartService) GetWebSocketServer() *WebSocketServer {
	return cs.wsServer
}

// GetHistoricalAPI returns the historical API for HTTP handler registration
func (cs *ChartService) GetHistoricalAPI() *HistoricalAPI {
	return cs.historicalAPI
}

// GetStats returns statistics about the chart service
func (cs *ChartService) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"websocket_stats": cs.wsServer.GetStats(),
		"supported_timeframes": cs.historicalAPI.GetSupportedTimeframes(),
		"timeframe_limits": cs.historicalAPI.GetTimeframeLimits(),
	}
}