package kraken

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"orderbook/internal/exchange"

	"github.com/gorilla/websocket"
)

// SpotExchange implements the Exchange interface for Kraken Spot
type SpotExchange struct {
	symbol           string
	wsURL            string
	wsConn           *websocket.Conn
	wsConnMu         sync.Mutex // Protects wsConn for concurrent writes
	updateChan       chan *exchange.DepthUpdate
	done             chan struct{}
	ctx              context.Context
	cancel           context.CancelFunc
	health           atomic.Value
	snapshotReceived bool
	snapshot         *exchange.Snapshot
	snapshotMu       sync.Mutex
	reconnecting     atomic.Bool // Flag to prevent multiple reconnection attempts
}

// NewSpotExchange creates a new Kraken Spot exchange instance
func NewSpotExchange(config Config) *SpotExchange {
	ctx, cancel := context.WithCancel(context.Background())

	wsURL := "wss://ws.kraken.com/v2"

	// Convert symbol to Kraken format (e.g., BTCUSDT -> BTC/USD)
	krakenSymbol := convertToKrakenSymbol(config.Symbol)

	ex := &SpotExchange{
		symbol:     krakenSymbol,
		wsURL:      wsURL,
		updateChan: make(chan *exchange.DepthUpdate, 1000),
		done:       make(chan struct{}),
		ctx:        ctx,
		cancel:     cancel,
	}

	ex.health.Store(exchange.HealthStatus{
		Connected:    false,
		LastPing:     time.Time{},
		MessageCount: 0,
		ErrorCount:   0,
	})

	return ex
}

// GetName returns the exchange name
func (e *SpotExchange) GetName() exchange.ExchangeName {
	return exchange.Kraken
}

// GetSymbol returns the trading symbol
func (e *SpotExchange) GetSymbol() string {
	return e.symbol
}

// Connect establishes WebSocket connection to Kraken
func (e *SpotExchange) Connect(ctx context.Context) error {
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(ctx, e.wsURL, nil)
	if err != nil {
		e.incrementErrorCount()
		return fmt.Errorf("websocket connection failed: %w", err)
	}

	e.wsConn = conn
	e.updateConnectionStatus(true)
	log.Printf("[%s] WebSocket connected successfully", e.GetName())

	subscribeMsg := SubscribeRequest{
		Method: "subscribe",
		Params: SubscribeParams{
			Channel:  "book",
			Symbol:   []string{e.symbol},
			Depth:    1000,
			Snapshot: true,
		},
	}

	if err := conn.WriteJSON(subscribeMsg); err != nil {
		e.incrementErrorCount()
		conn.Close()
		return fmt.Errorf("failed to subscribe: %w", err)
	}

	log.Printf("[%s] Subscribed to book channel for %s", e.GetName(), e.symbol)

	go e.readMessages()
	go e.pingLoop()

	return nil
}

// Close closes the WebSocket connection
func (e *SpotExchange) Close() error {
	if e.cancel != nil {
		e.cancel()
	}

	// Close the done channel to signal all goroutines to stop
	select {
	case <-e.done:
	default:
		close(e.done)
	}

	if e.wsConn != nil {
		err := e.wsConn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		if err != nil {
			log.Printf("[%s] Error sending close message: %v", e.GetName(), err)
		}

		select {
		case <-time.After(time.Second):
		}

		e.updateConnectionStatus(false)
		e.wsConn.Close()
	}

	// Close update channel after all goroutines have stopped
	close(e.updateChan)

	return nil
}

// GetSnapshot fetches the initial orderbook snapshot via WebSocket
func (e *SpotExchange) GetSnapshot(ctx context.Context) (*exchange.Snapshot, error) {
	log.Printf("[%s] Waiting for orderbook snapshot from WebSocket...", e.GetName())

	timeout := time.NewTimer(10 * time.Second)
	defer timeout.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timeout.C:
			return nil, fmt.Errorf("timeout waiting for snapshot")
		default:
			e.snapshotMu.Lock()
			snap := e.snapshot
			e.snapshotMu.Unlock()

			if snap != nil {
				return snap, nil
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// Updates returns a channel that receives depth updates
func (e *SpotExchange) Updates() <-chan *exchange.DepthUpdate {
	return e.updateChan
}

// IsConnected checks if the WebSocket connection is active
func (e *SpotExchange) IsConnected() bool {
	return e.wsConn != nil
}

// Health returns connection health information
func (e *SpotExchange) Health() exchange.HealthStatus {
	if status, ok := e.health.Load().(exchange.HealthStatus); ok {
		return status
	}
	return exchange.HealthStatus{}
}

// readMessages continuously reads WebSocket messages
func (e *SpotExchange) readMessages() {
	// Note: Don't close updateChan here since we may reconnect and reuse it
	defer e.updateConnectionStatus(false)

	for {
		select {
		case <-e.ctx.Done():
			log.Printf("[%s] Context cancelled, stopping message reading", e.GetName())
			return
		case <-e.done:
			return
		default:
			e.wsConnMu.Lock()
			conn := e.wsConn
			e.wsConnMu.Unlock()

			if conn == nil {
				log.Printf("[%s] Connection is nil, triggering reconnection", e.GetName())
				go e.reconnect()
				return
			}

			_, message, err := conn.ReadMessage()
			if err != nil {
				e.incrementErrorCount()
				log.Printf("[%s] WebSocket read error: %v", e.GetName(), err)
				// Trigger reconnection instead of just returning
				go e.reconnect()
				return
			}

			// Try to parse as subscription response first
			var subResp SubscribeResponse
			if err := json.Unmarshal(message, &subResp); err == nil && subResp.Method == "subscribe" {
				if !subResp.Success {
					log.Printf("[%s] Subscription failed: %s", e.GetName(), subResp.Error)
				}
				continue
			}

			// Try to parse as pong response
			var pongResp PongResponse
			if err := json.Unmarshal(message, &pongResp); err == nil && pongResp.Method == "pong" {
				// Pong received, connection is alive
				e.updateLastPing()
				continue
			}

			// Try to parse as heartbeat message
			var heartbeat HeartbeatMessage
			if err := json.Unmarshal(message, &heartbeat); err == nil && heartbeat.Channel == "heartbeat" {
				// Heartbeat received, connection is alive
				e.updateLastPing()
				continue
			}

			// Parse as data message
			var msg WSMessage
			if err := json.Unmarshal(message, &msg); err != nil {
				// Skip unknown message types silently
				continue
			}

			if msg.Channel != "book" || len(msg.Data) == 0 {
				continue
			}

			e.incrementMessageCount()
			e.updateLastPing()

			bookData := msg.Data[0]

			if msg.Type == "snapshot" && !e.snapshotReceived {
				e.storeSnapshot(&bookData)
				e.snapshotReceived = true
			}

			if msg.Type == "update" {
				canonicalUpdate := e.convertDepthUpdate(&bookData, msg.Type)

				select {
				case e.updateChan <- canonicalUpdate:
				case <-e.ctx.Done():
					return
				case <-e.done:
					return
				default:
					log.Printf("[%s] Warning: update channel full, skipping update", e.GetName())
				}
			}
		}
	}
}

// storeSnapshot converts and stores the initial snapshot
func (e *SpotExchange) storeSnapshot(data *BookData) {
	bids := make([]exchange.PriceLevel, len(data.Bids))
	for i, bid := range data.Bids {
		bids[i] = exchange.PriceLevel{
			Price:    fmt.Sprintf("%.10f", bid.Price),
			Quantity: fmt.Sprintf("%.10f", bid.Qty),
		}
	}

	asks := make([]exchange.PriceLevel, len(data.Asks))
	for i, ask := range data.Asks {
		asks[i] = exchange.PriceLevel{
			Price:    fmt.Sprintf("%.10f", ask.Price),
			Quantity: fmt.Sprintf("%.10f", ask.Qty),
		}
	}

	snapshot := &exchange.Snapshot{
		Exchange:     e.GetName(),
		Symbol:       data.Symbol,
		LastUpdateID: 0, // Kraken doesn't use update IDs, uses timestamps
		Bids:         bids,
		Asks:         asks,
		Timestamp:    time.Now(),
	}

	e.snapshotMu.Lock()
	e.snapshot = snapshot
	e.snapshotMu.Unlock()
}

// convertDepthUpdate converts Kraken depth update to canonical format
func (e *SpotExchange) convertDepthUpdate(data *BookData, msgType string) *exchange.DepthUpdate {
	bids := make([]exchange.PriceLevel, len(data.Bids))
	for i, bid := range data.Bids {
		bids[i] = exchange.PriceLevel{
			Price:    fmt.Sprintf("%.10f", bid.Price),
			Quantity: fmt.Sprintf("%.10f", bid.Qty),
		}
	}

	asks := make([]exchange.PriceLevel, len(data.Asks))
	for i, ask := range data.Asks {
		asks[i] = exchange.PriceLevel{
			Price:    fmt.Sprintf("%.10f", ask.Price),
			Quantity: fmt.Sprintf("%.10f", ask.Qty),
		}
	}

	var eventTime time.Time
	if data.Timestamp != "" {
		eventTime, _ = time.Parse(time.RFC3339Nano, data.Timestamp)
	} else {
		eventTime = time.Now()
	}

	return &exchange.DepthUpdate{
		Exchange:      e.GetName(),
		Symbol:        data.Symbol,
		EventTime:     eventTime,
		FirstUpdateID: 0,
		FinalUpdateID: 0,
		PrevUpdateID:  0,
		Bids:          bids,
		Asks:          asks,
	}
}

// pingLoop sends periodic ping messages to keep the connection alive
func (e *SpotExchange) pingLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	reqID := 1
	for {
		select {
		case <-e.ctx.Done():
			return
		case <-e.done:
			return
		case <-ticker.C:
			e.wsConnMu.Lock()
			conn := e.wsConn
			e.wsConnMu.Unlock()

			if conn == nil {
				log.Printf("[%s] Ping loop: connection is nil, stopping ping loop", e.GetName())
				return
			}

			pingMsg := PingRequest{
				Method: "ping",
				ReqID:  reqID,
			}
			reqID++

			e.wsConnMu.Lock()
			err := conn.WriteJSON(pingMsg)
			e.wsConnMu.Unlock()

			if err != nil {
				log.Printf("[%s] Failed to send ping: %v", e.GetName(), err)
				// Don't reconnect here, let readMessages handle it
				return
			}
		}
	}
}

// reconnect attempts to re-establish the WebSocket connection
func (e *SpotExchange) reconnect() {
	// Prevent multiple simultaneous reconnection attempts
	if !e.reconnecting.CompareAndSwap(false, true) {
		log.Printf("[%s] Reconnection already in progress, skipping", e.GetName())
		return
	}
	defer e.reconnecting.Store(false)

	log.Printf("[%s] Starting reconnection...", e.GetName())

	// Close existing connection if any
	e.wsConnMu.Lock()
	if e.wsConn != nil {
		e.wsConn.Close()
		e.wsConn = nil
	}
	e.wsConnMu.Unlock()

	// Wait before reconnecting (Kraken recommends at least 5 seconds)
	time.Sleep(5 * time.Second)

	// Attempt to reconnect
	maxAttempts := 10
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		select {
		case <-e.ctx.Done():
			log.Printf("[%s] Context cancelled during reconnection", e.GetName())
			return
		case <-e.done:
			log.Printf("[%s] Done signal received during reconnection", e.GetName())
			return
		default:
			log.Printf("[%s] Reconnection attempt %d/%d", e.GetName(), attempt, maxAttempts)

			// Create new context for this connection attempt
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			err := e.Connect(ctx)
			cancel()

			if err == nil {
				log.Printf("[%s] Reconnection successful!", e.GetName())

				// Reset snapshot flag to get a fresh snapshot
				e.snapshotMu.Lock()
				e.snapshotReceived = false
				e.snapshot = nil
				e.snapshotMu.Unlock()

				return
			}

			log.Printf("[%s] Reconnection attempt %d failed: %v", e.GetName(), attempt, err)

			// Exponential backoff with max of 30 seconds
			backoff := time.Duration(attempt) * 5 * time.Second
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
			time.Sleep(backoff)
		}
	}

	log.Printf("[%s] Failed to reconnect after %d attempts, giving up", e.GetName(), maxAttempts)
}

// convertToKrakenSymbol converts various symbol formats to Kraken format
// Examples: BTCUSDT -> BTC/USD, ETHUSDT -> ETH/USD, BTC/USD -> BTC/USD
func convertToKrakenSymbol(symbol string) string {
	// If already has slash, assume it's correct
	if strings.Contains(symbol, "/") {
		return strings.ToUpper(symbol)
	}

	symbol = strings.ToUpper(symbol)

	// Common conversions for USDT pairs
	if strings.HasSuffix(symbol, "USDT") {
		base := strings.TrimSuffix(symbol, "USDT")
		return fmt.Sprintf("%s/USD", base)
	}

	// Common conversions for USD pairs (without T)
	if strings.HasSuffix(symbol, "USD") && !strings.HasSuffix(symbol, "USDT") {
		base := strings.TrimSuffix(symbol, "USD")
		return fmt.Sprintf("%s/USD", base)
	}

	// Common conversions for EUR pairs
	if strings.HasSuffix(symbol, "EUR") {
		base := strings.TrimSuffix(symbol, "EUR")
		return fmt.Sprintf("%s/EUR", base)
	}

	// Common conversions for GBP pairs
	if strings.HasSuffix(symbol, "GBP") {
		base := strings.TrimSuffix(symbol, "GBP")
		return fmt.Sprintf("%s/GBP", base)
	}

	// If we can't determine, return as-is and let Kraken reject it
	log.Printf("[Kraken] Warning: Could not convert symbol %s to Kraken format, using as-is", symbol)
	return symbol
}

// updateConnectionStatus updates the connection status in health
func (e *SpotExchange) updateConnectionStatus(connected bool) {
	status := e.Health()
	status.Connected = connected
	if !connected {
		now := time.Now()
		status.ReconnectTime = &now
	}
	e.health.Store(status)
}

// incrementMessageCount increments the message count in health
func (e *SpotExchange) incrementMessageCount() {
	status := e.Health()
	status.MessageCount++
	e.health.Store(status)
}

// incrementErrorCount increments the error count in health
func (e *SpotExchange) incrementErrorCount() {
	status := e.Health()
	status.ErrorCount++
	e.health.Store(status)
}

// updateLastPing updates the last ping time in health
func (e *SpotExchange) updateLastPing() {
	status := e.Health()
	status.LastPing = time.Now()
	e.health.Store(status)
}
