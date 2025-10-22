package hyperliquid

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"orderbook/internal/exchange"
)

// FuturesExchange implements the Exchange interface for Hyperliquid
type FuturesExchange struct {
	symbol       string
	wsURL        string
	restURL      string
	wsConn       *websocket.Conn
	wsConnMu     sync.Mutex // Protects wsConn for thread-safe operations
	updateChan   chan *exchange.DepthUpdate
	done         chan struct{}
	ctx          context.Context
	cancel       context.CancelFunc
	health       atomic.Value // stores exchange.HealthStatus
	reconnecting atomic.Bool  // Prevents concurrent reconnection attempts
}

// Config holds configuration for Hyperliquid exchange
type Config struct {
	Symbol string
}

// NewFuturesExchange creates a new Hyperliquid exchange instance
func NewFuturesExchange(config Config) *FuturesExchange {
	ctx, cancel := context.WithCancel(context.Background())

	// Convert XXXUSDT to XXX for Hyperliquid (e.g., BTCUSDT -> BTC)
	symbol := strings.TrimSuffix(config.Symbol, "USDT")

	ex := &FuturesExchange{
		symbol:     symbol,
		wsURL:      "wss://api.hyperliquid.xyz/ws",
		restURL:    "https://api.hyperliquid.xyz/info",
		updateChan: make(chan *exchange.DepthUpdate, 5000),
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
func (e *FuturesExchange) GetName() exchange.ExchangeName {
	return exchange.Hyperliquidf
}

// GetSymbol returns the trading symbol
func (e *FuturesExchange) GetSymbol() string {
	return e.symbol
}

// Connect establishes WebSocket connection to Hyperliquid
func (e *FuturesExchange) Connect(ctx context.Context) error {
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(ctx, e.wsURL, nil)
	if err != nil {
		e.incrementErrorCount()
		return fmt.Errorf("websocket connection failed: %w", err)
	}

	e.wsConnMu.Lock()
	e.wsConn = conn
	e.wsConnMu.Unlock()

	e.updateConnectionStatus(true)
	log.Printf("[%s] WebSocket connected successfully", e.GetName())

	// Subscribe to L2 book updates
	subscription := SubscriptionMessage{
		Method: "subscribe",
		Subscription: map[string]interface{}{
			"type": "l2Book",
			"coin": e.symbol,
		},
	}

	if err := conn.WriteJSON(subscription); err != nil {
		e.incrementErrorCount()
		return fmt.Errorf("failed to send subscription: %w", err)
	}

	go e.readMessages()
	go e.pingLoop()

	return nil
}

// Close closes the WebSocket connection
func (e *FuturesExchange) Close() error {
	if e.cancel != nil {
		e.cancel()
	}

	select {
	case <-e.done:
	default:
		close(e.done)
	}

	e.wsConnMu.Lock()
	conn := e.wsConn
	e.wsConnMu.Unlock()

	if conn != nil {
		err := conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		if err != nil {
			log.Printf("[%s] Error sending close message: %v", e.GetName(), err)
		}

		select {
		case <-time.After(time.Second):
		}

		e.updateConnectionStatus(false)
		return conn.Close()
	}
	return nil
}

// pingLoop monitors connection health by checking message timestamps
func (e *FuturesExchange) pingLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-e.ctx.Done():
			return
		case <-e.done:
			return
		case <-ticker.C:
			health := e.Health()
			if !health.LastPing.IsZero() && time.Since(health.LastPing) > 60*time.Second {
				log.Printf("[%s] No messages received for 60s, connection may be stale", e.GetName())
				go e.reconnect()
				return
			}
		}
	}
}

// reconnect attempts to reconnect the WebSocket connection with exponential backoff
func (e *FuturesExchange) reconnect() {
	// Prevent multiple simultaneous reconnection attempts
	if !e.reconnecting.CompareAndSwap(false, true) {
		log.Printf("[%s] Reconnection already in progress, skipping", e.GetName())
		return
	}
	defer e.reconnecting.Store(false)

	log.Printf("[%s] Starting reconnection process", e.GetName())

	// Close existing connection
	e.wsConnMu.Lock()
	if e.wsConn != nil {
		e.wsConn.Close()
		e.wsConn = nil
	}
	e.wsConnMu.Unlock()

	maxAttempts := 10
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		select {
		case <-e.ctx.Done():
			log.Printf("[%s] Context cancelled during reconnection", e.GetName())
			return
		case <-e.done:
			log.Printf("[%s] Shutdown signal during reconnection", e.GetName())
			return
		default:
		}

		log.Printf("[%s] Reconnection attempt %d/%d", e.GetName(), attempt, maxAttempts)

		// Exponential backoff: 5s, 10s, 15s, ..., up to 30s
		if attempt > 1 {
			backoff := time.Duration(attempt-1) * 5 * time.Second
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
			log.Printf("[%s] Waiting %v before reconnection attempt", e.GetName(), backoff)
			time.Sleep(backoff)
		}

		// Attempt to reconnect
		if err := e.Connect(e.ctx); err != nil {
			log.Printf("[%s] Reconnection attempt %d failed: %v", e.GetName(), attempt, err)
			continue
		}

		log.Printf("[%s] Reconnection successful", e.GetName())
		return
	}

	log.Printf("[%s] Failed to reconnect after %d attempts", e.GetName(), maxAttempts)
}

// GetSnapshot fetches the initial orderbook snapshot via REST API
func (e *FuturesExchange) GetSnapshot(ctx context.Context) (*exchange.Snapshot, error) {
	log.Printf("[%s] Fetching orderbook snapshot...", e.GetName())

	requestBody := map[string]interface{}{
		"type": "l2Book",
		"coin": e.symbol,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", e.restURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		e.incrementErrorCount()
		return nil, fmt.Errorf("failed to get snapshot: %w", err)
	}
	defer resp.Body.Close()

	var hyperliquidSnapshot L2BookResponse
	if err := json.NewDecoder(resp.Body).Decode(&hyperliquidSnapshot); err != nil {
		e.incrementErrorCount()
		return nil, fmt.Errorf("failed to decode snapshot: %w", err)
	}

	snapshot := e.convertSnapshot(&hyperliquidSnapshot)
	return snapshot, nil
}

// Updates returns a channel that receives depth updates
func (e *FuturesExchange) Updates() <-chan *exchange.DepthUpdate {
	return e.updateChan
}

// IsConnected checks if the WebSocket connection is active
func (e *FuturesExchange) IsConnected() bool {
	return e.wsConn != nil
}

// Health returns connection health information
func (e *FuturesExchange) Health() exchange.HealthStatus {
	if status, ok := e.health.Load().(exchange.HealthStatus); ok {
		return status
	}
	return exchange.HealthStatus{}
}

// readMessages continuously reads WebSocket messages
func (e *FuturesExchange) readMessages() {
	defer close(e.updateChan)
	defer e.updateConnectionStatus(false)

	for {
		select {
		case <-e.ctx.Done():
			log.Printf("[%s] Context cancelled, stopping message reading", e.GetName())
			return
		case <-e.done:
			return
		default:
			// Thread-safe access to connection
			e.wsConnMu.Lock()
			conn := e.wsConn
			e.wsConnMu.Unlock()

			if conn == nil {
				log.Printf("[%s] Connection is nil, triggering reconnection", e.GetName())
				go e.reconnect()
				return
			}

			var msg WSMessage
			if err := conn.ReadJSON(&msg); err != nil {
				e.incrementErrorCount()
				log.Printf("[%s] WebSocket read error: %v", e.GetName(), err)
				go e.reconnect()
				return
			}

			e.incrementMessageCount()
			e.updateLastPing()

			// Handle subscription response
			if msg.Channel == "subscriptionResponse" {
				continue
			}

			// Handle L2 book updates
			if msg.Channel == "l2Book" {
				var bookData WsBook
				dataBytes, err := json.Marshal(msg.Data)
				if err != nil {
					log.Printf("[%s] Error marshalling book data: %v", e.GetName(), err)
					continue
				}

				if err := json.Unmarshal(dataBytes, &bookData); err != nil {
					log.Printf("[%s] Error unmarshalling book data: %v", e.GetName(), err)
					continue
				}

				canonicalUpdate := e.convertDepthUpdate(&bookData)

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

// convertSnapshot converts Hyperliquid snapshot to canonical format
func (e *FuturesExchange) convertSnapshot(snapshot *L2BookResponse) *exchange.Snapshot {
	bids := make([]exchange.PriceLevel, len(snapshot.Levels[0]))
	for i, bid := range snapshot.Levels[0] {
		bids[i] = exchange.PriceLevel{
			Price:    bid.Px,
			Quantity: bid.Sz,
		}
	}

	asks := make([]exchange.PriceLevel, len(snapshot.Levels[1]))
	for i, ask := range snapshot.Levels[1] {
		asks[i] = exchange.PriceLevel{
			Price:    ask.Px,
			Quantity: ask.Sz,
		}
	}

	return &exchange.Snapshot{
		Exchange:     e.GetName(),
		Symbol:       e.symbol,
		LastUpdateID: snapshot.Time, // Use timestamp as update ID
		Bids:         bids,
		Asks:         asks,
		Timestamp:    time.UnixMilli(snapshot.Time),
	}
}

// convertDepthUpdate converts Hyperliquid book update to canonical format
func (e *FuturesExchange) convertDepthUpdate(update *WsBook) *exchange.DepthUpdate {
	bids := make([]exchange.PriceLevel, len(update.Levels[0]))
	for i, bid := range update.Levels[0] {
		bids[i] = exchange.PriceLevel{
			Price:    bid.Px,
			Quantity: bid.Sz,
		}
	}

	asks := make([]exchange.PriceLevel, len(update.Levels[1]))
	for i, ask := range update.Levels[1] {
		asks[i] = exchange.PriceLevel{
			Price:    ask.Px,
			Quantity: ask.Sz,
		}
	}

	return &exchange.DepthUpdate{
		Exchange:      e.GetName(),
		Symbol:        update.Coin,
		EventTime:     time.UnixMilli(update.Time),
		FirstUpdateID: update.Time,
		FinalUpdateID: update.Time,
		PrevUpdateID:  update.Time - 1, // Approximation since Hyperliquid doesn't provide this
		Bids:          bids,
		Asks:          asks,
	}
}

// updateConnectionStatus updates the connection status in health
func (e *FuturesExchange) updateConnectionStatus(connected bool) {
	status := e.Health()
	status.Connected = connected
	if !connected {
		now := time.Now()
		status.ReconnectTime = &now
	}
	e.health.Store(status)
}

// incrementMessageCount increments the message count in health
func (e *FuturesExchange) incrementMessageCount() {
	status := e.Health()
	status.MessageCount++
	e.health.Store(status)
}

// incrementErrorCount increments the error count in health
func (e *FuturesExchange) incrementErrorCount() {
	status := e.Health()
	status.ErrorCount++
	e.health.Store(status)
}

// updateLastPing updates the last ping time in health
func (e *FuturesExchange) updateLastPing() {
	status := e.Health()
	status.LastPing = time.Now()
	e.health.Store(status)
}