package ingester

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/ashmitsharp/trading/internal/config"
	"github.com/ashmitsharp/trading/internal/db"
	"github.com/ashmitsharp/trading/internal/models"
	"github.com/gorilla/websocket"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

const (
	pingPeriod     = 20 * time.Second
	pongWait       = 60 * time.Second
	writeWait      = 10 * time.Second
	maxMessageSize = 4096

	// batch settings
	batchSize    = 1000
	batchTimeout = 5 * time.Second

	// reconnection
	maxReconnectAttempts = 10
	baseReconnectDelay   = 2 * time.Second
	maxReconnectDelay    = 5 * time.Second
)

type BinanceIngester struct {
	conn   driver.Conn
	logger *zap.Logger
	config config.BinanceConfig
	wsConn *websocket.Conn
	ctx    context.Context
	cancel context.CancelFunc

	tradeBatch        []db.TradeData
	batchMutex        sync.Mutex
	reconnectAttempts int
	isRunning         bool
	mu                sync.RWMutex
}

// create a new binance data ingester
func NewBinanceIngester(conn driver.Conn, logger *zap.Logger, config config.BinanceConfig) *BinanceIngester {
	ctx, cancel := context.WithCancel(context.Background())

	return &BinanceIngester{
		conn:   conn,
		logger: logger,
		config: config,
		ctx:    ctx,
		cancel: cancel,
	}
}

func (bi *BinanceIngester) Start() {
	bi.mu.Lock()
	if bi.isRunning {
		bi.mu.Unlock()
		return
	}

	bi.isRunning = true
	bi.mu.Unlock()

	bi.logger.Info("Starting Binance ingester")

	// start the batch processor
	go bi.processBatches()

	// websocket conn with retry logic
	go bi.connectWithRetry()
}

func (bi *BinanceIngester) Stop() {
	bi.mu.Lock()
	defer bi.mu.Unlock()

	if !bi.isRunning {
		return
	}

	bi.logger.Info("Stopping Binance Ingester")
	bi.isRunning = false
	bi.cancel()

	if bi.wsConn != nil {
		bi.wsConn.Close()
	}

	// process remaining batch
	bi.flushBatch()
}

func (bi *BinanceIngester) connectWithRetry() {
	for {
		select {
		case <-bi.ctx.Done():
			return
		default:
		}
		if err := bi.connect(); err != nil {
			bi.reconnectAttempts++
			if bi.reconnectAttempts > maxReconnectAttempts {
				bi.logger.Error("Max reconnection attempts reached", zap.Error(err))
				return
			}

			delay := bi.calculateBackoffDelay()
			bi.logger.Warn("Websocket connection failed, retrying",
				zap.Error(err),
				zap.Int("attempt", bi.reconnectAttempts),
				zap.Duration("retry_in", delay),
			)

			select {
			case <-time.After(delay):
				continue
			case <-bi.ctx.Done():
				return
			}
		} else {
			bi.reconnectAttempts = 0
		}
	}
}

// connect establishes WebSocket connection and starts listening
func (bi *BinanceIngester) connect() error {
	// Build combined stream URL
	streamURL := bi.buildStreamURL()

	bi.logger.Info("Connecting to Binance WebSocket", zap.String("url", streamURL))

	dialer := websocket.Dialer{
		HandshakeTimeout: 45 * time.Second,
	}

	conn, _, err := dialer.Dial(streamURL, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to WebSocket: %w", err)
	}

	bi.wsConn = conn

	// Configure connection
	bi.wsConn.SetReadLimit(maxMessageSize)
	bi.wsConn.SetReadDeadline(time.Now().Add(pongWait))
	bi.wsConn.SetPongHandler(func(string) error {
		bi.wsConn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	// Start ping routine
	go bi.pingRoutine()

	// Start reading messages
	return bi.readMessages()
}

func (bi *BinanceIngester) buildStreamURL() string {
	streams := make([]string, len(bi.config.Symbols))
	for i, symbol := range bi.config.Symbols {
		streams[i] = fmt.Sprintf("%s@trade", strings.ToLower(symbol))
	}

	u, _ := url.Parse(bi.config.WSBaseURL)
	u.Path = "/stream"
	q := u.Query()
	q.Set("streams", strings.Join(streams, "/"))
	u.RawQuery = q.Encode()

	return u.String()
}

func (bi *BinanceIngester) pingRoutine() {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := bi.wsConn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(writeWait)); err != nil {
				bi.logger.Error("Failed to send ping", zap.Error(err))
				return
			}
		case <-bi.ctx.Done():
			return
		}
	}
}

func (bi *BinanceIngester) readMessages() error {
	for {
		select {
		case <-bi.ctx.Done():
			return nil
		default:
		}

		_, message, err := bi.wsConn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				return fmt.Errorf("WebSocket connection closed unexpectedly: %w", err)
			}
			return err
		}

		if err := bi.processMessage(message); err != nil {
			bi.logger.Error("Failed to process message", zap.Error(err), zap.String("message", string(message)))
		}
	}
}

// processMessage processes incoming trade messages
func (bi *BinanceIngester) processMessage(message []byte) error {
	var streamEvent models.BinanceCombinedStreamEvent
	if err := json.Unmarshal(message, &streamEvent); err != nil {
		return fmt.Errorf("failed to unmarshal stream event: %w", err)
	}

	// Parse trade data
	trade, err := bi.parseTradeEvent(streamEvent.Data)
	if err != nil {
		return fmt.Errorf("failed to parse trade event: %w", err)
	}

	// Add to batch
	bi.addToBatch(trade)

	return nil
}

// parseTradeEvent converts Binance trade event to internal trade data
func (bi *BinanceIngester) parseTradeEvent(event models.BinanceTradeEvent) (db.TradeData, error) {
	price, err := decimal.NewFromString(event.Price)
	if err != nil {
		return db.TradeData{}, fmt.Errorf("failed to parse price: %w", err)
	}

	quantity, err := decimal.NewFromString(event.Quantity)
	if err != nil {
		return db.TradeData{}, fmt.Errorf("failed to parse quantity: %w", err)
	}

	var isBuyerMaker uint8
	if event.IsBuyerMaker {
		isBuyerMaker = 1
	}

	return db.TradeData{
		Symbol:       strings.ToUpper(event.Symbol),
		Price:        price,
		Quantity:     quantity,
		TradeID:      uint64(event.TradeID),
		Timestamp:    event.TradeTime,
		IsBuyerMaker: isBuyerMaker,
	}, nil
}

// addToBatch adds a trade to the current batch
func (bi *BinanceIngester) addToBatch(trade db.TradeData) {
	bi.batchMutex.Lock()
	defer bi.batchMutex.Unlock()

	bi.tradeBatch = append(bi.tradeBatch, trade)

	// Flush if batch is full
	if len(bi.tradeBatch) >= batchSize {
		go bi.flushBatch()
	}
}

// processBatches periodically flushes batches
func (bi *BinanceIngester) processBatches() {
	ticker := time.NewTicker(batchTimeout)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			bi.flushBatch()
		case <-bi.ctx.Done():
			return
		}
	}
}

// flushBatch writes the current batch to ClickHouse
func (bi *BinanceIngester) flushBatch() {
	bi.batchMutex.Lock()
	if len(bi.tradeBatch) == 0 {
		bi.batchMutex.Unlock()
		return
	}

	batch := make([]db.TradeData, len(bi.tradeBatch))
	copy(batch, bi.tradeBatch)
	bi.tradeBatch = bi.tradeBatch[:0] // Reset slice
	bi.batchMutex.Unlock()

	if err := db.InsertTrades(bi.conn, batch); err != nil {
		bi.logger.Error("Failed to insert batch",
			zap.Error(err),
			zap.Int("batch_size", len(batch)))
		return
	}

	bi.logger.Debug("Batch inserted successfully",
		zap.Int("trades_count", len(batch)))
}

// calculateBackoffDelay calculates exponential backoff delay
func (bi *BinanceIngester) calculateBackoffDelay() time.Duration {
	delay := baseReconnectDelay
	for i := 1; i < bi.reconnectAttempts; i++ {
		delay *= 2
		if delay > maxReconnectDelay {
			delay = maxReconnectDelay
			break
		}
	}
	return delay
}

// IsRunning returns whether the ingester is currently running
func (bi *BinanceIngester) IsRunning() bool {
	bi.mu.RLock()
	defer bi.mu.RUnlock()
	return bi.isRunning
}

// GetStats returns ingestion statistics
func (bi *BinanceIngester) GetStats() map[string]interface{} {
	bi.batchMutex.Lock()
	batchSize := len(bi.tradeBatch)
	bi.batchMutex.Unlock()

	return map[string]interface{}{
		"is_running":         bi.IsRunning(),
		"current_batch_size": batchSize,
		"reconnect_attempts": bi.reconnectAttempts,
		"symbols":            bi.config.Symbols,
	}
}
