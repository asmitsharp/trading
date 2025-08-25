package chart_api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// WebSocketServer manages WebSocket connections for real-time chart data
type WebSocketServer struct {
	clients       map[string]*Client
	subscriptions map[string]*Subscription
	upgrader      websocket.Upgrader
	mutex         sync.RWMutex
	logger        *zap.Logger
	ctx           context.Context
	cancel        context.CancelFunc
}

// NewWebSocketServer creates a new WebSocket server
func NewWebSocketServer(logger *zap.Logger) *WebSocketServer {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &WebSocketServer{
		clients:       make(map[string]*Client),
		subscriptions: make(map[string]*Subscription),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				// Allow all origins for now - in production, validate origins
				return true
			},
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
		logger: logger,
		ctx:    ctx,
		cancel: cancel,
	}
}

// HandleConnection handles new WebSocket connections
func (ws *WebSocketServer) HandleConnection(w http.ResponseWriter, r *http.Request) {
	conn, err := ws.upgrader.Upgrade(w, r, nil)
	if err != nil {
		ws.logger.Error("Failed to upgrade connection", zap.Error(err))
		return
	}

	clientID := generateClientID()
	client := &Client{
		ID:            clientID,
		Connection:    conn,
		Subscriptions: make(map[string]bool),
		LastPing:      time.Now(),
		IsAlive:       true,
	}

	ws.mutex.Lock()
	ws.clients[clientID] = client
	ws.mutex.Unlock()

	ws.logger.Info("New WebSocket client connected", zap.String("client_id", clientID))

	// Send welcome message
	welcome := WebSocketMessage{
		Type:      "welcome",
		Data:      map[string]interface{}{
			"client_id": clientID,
			"server_time": time.Now().Unix() * 1000,
		},
		Timestamp: time.Now().Unix() * 1000,
	}
	ws.sendToClient(client, welcome)

	// Start goroutines for this client
	go ws.readPump(client)
	go ws.writePump(client)
}

// readPump handles incoming messages from client
func (ws *WebSocketServer) readPump(client *Client) {
	defer func() {
		ws.removeClient(client)
		if conn, ok := client.Connection.(*websocket.Conn); ok {
			conn.Close()
		}
	}()

	conn := client.Connection.(*websocket.Conn)
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	
	conn.SetPongHandler(func(string) error {
		client.LastPing = time.Now()
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, messageBytes, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				ws.logger.Error("WebSocket error", zap.Error(err))
			}
			break
		}

		var request SubscriptionRequest
		if err := json.Unmarshal(messageBytes, &request); err != nil {
			ws.logger.Error("Failed to parse message", zap.Error(err))
			ws.sendError(client, "Invalid message format")
			continue
		}

		ws.handleSubscriptionRequest(client, request)
	}
}

// writePump handles outgoing messages to client
func (ws *WebSocketServer) writePump(client *Client) {
	ticker := time.NewTicker(54 * time.Second) // Ping every 54 seconds
	defer func() {
		ticker.Stop()
		if conn, ok := client.Connection.(*websocket.Conn); ok {
			conn.Close()
		}
	}()

	conn := client.Connection.(*websocket.Conn)
	
	for {
		select {
		case <-ws.ctx.Done():
			return
		case <-ticker.C:
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// handleSubscriptionRequest processes subscription/unsubscription requests
func (ws *WebSocketServer) handleSubscriptionRequest(client *Client, request SubscriptionRequest) {
	switch request.Type {
	case "subscribe":
		ws.subscribe(client, request)
	case "unsubscribe":
		ws.unsubscribe(client, request)
	default:
		ws.sendError(client, "Unknown request type: "+request.Type)
	}
}

// subscribe adds client to a channel subscription
func (ws *WebSocketServer) subscribe(client *Client, request SubscriptionRequest) {
	if !ws.isValidSubscription(request) {
		ws.sendError(client, "Invalid subscription parameters")
		return
	}

	channelKey := ws.getChannelKey(request)
	
	ws.mutex.Lock()
	
	// Get or create subscription
	subscription, exists := ws.subscriptions[channelKey]
	if !exists {
		subscription = &Subscription{
			Channel:      request.Channel,
			BaseTokenID:  request.BaseTokenID,
			QuoteTokenID: request.QuoteTokenID,
			Timeframe:    request.Timeframe,
			Clients:      make(map[string]*Client),
		}
		ws.subscriptions[channelKey] = subscription
	}
	
	// Add client to subscription
	subscription.Clients[client.ID] = client
	client.Subscriptions[channelKey] = true
	
	ws.mutex.Unlock()

	ws.logger.Info("Client subscribed to channel",
		zap.String("client_id", client.ID),
		zap.String("channel", channelKey))

	// Send confirmation
	response := WebSocketMessage{
		Type:    "subscription_success",
		Channel: channelKey,
		Data: map[string]interface{}{
			"status": "subscribed",
			"channel": channelKey,
		},
		Timestamp: time.Now().Unix() * 1000,
	}
	ws.sendToClient(client, response)
}

// unsubscribe removes client from a channel subscription
func (ws *WebSocketServer) unsubscribe(client *Client, request SubscriptionRequest) {
	channelKey := ws.getChannelKey(request)
	
	ws.mutex.Lock()
	
	if subscription, exists := ws.subscriptions[channelKey]; exists {
		delete(subscription.Clients, client.ID)
		delete(client.Subscriptions, channelKey)
		
		// Remove empty subscriptions
		if len(subscription.Clients) == 0 {
			delete(ws.subscriptions, channelKey)
		}
	}
	
	ws.mutex.Unlock()

	ws.logger.Info("Client unsubscribed from channel",
		zap.String("client_id", client.ID),
		zap.String("channel", channelKey))

	// Send confirmation
	response := WebSocketMessage{
		Type:    "unsubscription_success",
		Channel: channelKey,
		Data: map[string]interface{}{
			"status": "unsubscribed",
			"channel": channelKey,
		},
		Timestamp: time.Now().Unix() * 1000,
	}
	ws.sendToClient(client, response)
}

// BroadcastPriceUpdate broadcasts live price updates to subscribed clients
func (ws *WebSocketServer) BroadcastPriceUpdate(price LivePrice) {
	channelKey := fmt.Sprintf("price:%d:%d", price.BaseTokenID, price.QuoteTokenID)
	
	ws.mutex.RLock()
	subscription, exists := ws.subscriptions[channelKey]
	ws.mutex.RUnlock()
	
	if !exists || len(subscription.Clients) == 0 {
		return
	}

	message := WebSocketMessage{
		Type:    "price_update",
		Channel: channelKey,
		Data:    price,
		Timestamp: time.Now().Unix() * 1000,
	}

	ws.broadcastToSubscription(subscription, message)
}

// BroadcastCandleUpdate broadcasts live candle updates to subscribed clients
func (ws *WebSocketServer) BroadcastCandleUpdate(candle LiveCandle) {
	channelKey := fmt.Sprintf("candles:%s:%d:%d", candle.Timeframe, candle.BaseTokenID, candle.QuoteTokenID)
	
	ws.mutex.RLock()
	subscription, exists := ws.subscriptions[channelKey]
	ws.mutex.RUnlock()
	
	if !exists || len(subscription.Clients) == 0 {
		return
	}

	message := WebSocketMessage{
		Type:    "candle_update",
		Channel: channelKey,
		Data:    candle,
		Timestamp: time.Now().Unix() * 1000,
	}

	ws.broadcastToSubscription(subscription, message)
}

// broadcastToSubscription sends message to all clients in a subscription
func (ws *WebSocketServer) broadcastToSubscription(subscription *Subscription, message WebSocketMessage) {
	for clientID, client := range subscription.Clients {
		if !client.IsAlive {
			continue
		}
		
		if err := ws.sendToClient(client, message); err != nil {
			ws.logger.Error("Failed to send message to client",
				zap.String("client_id", clientID),
				zap.Error(err))
			// Mark client as dead - cleanup will handle it
			client.IsAlive = false
		}
	}
}

// sendToClient sends a message to a specific client
func (ws *WebSocketServer) sendToClient(client *Client, message WebSocketMessage) error {
	conn, ok := client.Connection.(*websocket.Conn)
	if !ok {
		return fmt.Errorf("invalid connection type")
	}

	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return conn.WriteJSON(message)
}

// sendError sends an error message to a client
func (ws *WebSocketServer) sendError(client *Client, errorMsg string) {
	message := WebSocketMessage{
		Type:      "error",
		Error:     errorMsg,
		Timestamp: time.Now().Unix() * 1000,
	}
	ws.sendToClient(client, message)
}

// removeClient removes a client and cleans up subscriptions
func (ws *WebSocketServer) removeClient(client *Client) {
	ws.mutex.Lock()
	defer ws.mutex.Unlock()

	// Remove from all subscriptions
	for channelKey := range client.Subscriptions {
		if subscription, exists := ws.subscriptions[channelKey]; exists {
			delete(subscription.Clients, client.ID)
			
			// Remove empty subscriptions
			if len(subscription.Clients) == 0 {
				delete(ws.subscriptions, channelKey)
			}
		}
	}

	// Remove client
	delete(ws.clients, client.ID)

	ws.logger.Info("Client disconnected", zap.String("client_id", client.ID))
}

// isValidSubscription validates subscription parameters
func (ws *WebSocketServer) isValidSubscription(request SubscriptionRequest) bool {
	if request.Channel != "price" && request.Channel != "candles" {
		return false
	}
	
	if request.BaseTokenID == 0 || request.QuoteTokenID == 0 {
		return false
	}
	
	if request.Channel == "candles" {
		// Validate timeframe
		for _, tf := range SupportedTimeframes {
			if tf == request.Timeframe {
				return true
			}
		}
		return false
	}
	
	return true
}

// getChannelKey generates a unique key for subscription channels
func (ws *WebSocketServer) getChannelKey(request SubscriptionRequest) string {
	if request.Channel == "price" {
		return fmt.Sprintf("price:%d:%d", request.BaseTokenID, request.QuoteTokenID)
	}
	return fmt.Sprintf("candles:%s:%d:%d", request.Timeframe, request.BaseTokenID, request.QuoteTokenID)
}

// GetStats returns statistics about WebSocket connections
func (ws *WebSocketServer) GetStats() map[string]interface{} {
	ws.mutex.RLock()
	defer ws.mutex.RUnlock()

	return map[string]interface{}{
		"total_clients":      len(ws.clients),
		"total_subscriptions": len(ws.subscriptions),
		"active_clients":     ws.countActiveClients(),
	}
}

// countActiveClients counts clients that are still alive
func (ws *WebSocketServer) countActiveClients() int {
	count := 0
	for _, client := range ws.clients {
		if client.IsAlive {
			count++
		}
	}
	return count
}

// Shutdown gracefully shuts down the WebSocket server
func (ws *WebSocketServer) Shutdown() {
	ws.logger.Info("Shutting down WebSocket server")
	
	ws.cancel()
	
	ws.mutex.Lock()
	defer ws.mutex.Unlock()
	
	// Close all client connections
	for _, client := range ws.clients {
		if conn, ok := client.Connection.(*websocket.Conn); ok {
			conn.Close()
		}
	}
	
	ws.logger.Info("WebSocket server shutdown complete")
}

// generateClientID generates a unique client ID
func generateClientID() string {
	return fmt.Sprintf("client_%d", time.Now().UnixNano())
}