package binance

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"orderbook/internal/exchange"
)

// FuturesExchange implements the Exchange interface for Binance Futures
type FuturesExchange struct {
	symbol     string
	wsURL      string
	restURL    string
	wsConn     *websocket.Conn
	updateChan chan *exchange.DepthUpdate
	done       chan struct{}
	ctx        context.Context
	cancel     context.CancelFunc
	health     atomic.Value // stores exchange.HealthStatus
}

// Config holds configuration for Binance Futures exchange
type Config struct {
	Symbol string
}

// NewFuturesExchange creates a new Binance Futures exchange instance
func NewFuturesExchange(config Config) *FuturesExchange {
	ctx, cancel := context.WithCancel(context.Background())

	symbol := strings.ToLower(config.Symbol)
	wsURL := fmt.Sprintf("wss://fstream.binance.com/stream?streams=%s@depth", symbol)
	restURL := fmt.Sprintf("https://fapi.binance.com/fapi/v1/depth?symbol=%s&limit=1000", strings.ToUpper(config.Symbol))

	ex := &FuturesExchange{
		symbol:     config.Symbol,
		wsURL:      wsURL,
		restURL:    restURL,
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
func (e *FuturesExchange) GetName() exchange.ExchangeName {
	return exchange.Binancef
}

// GetSymbol returns the trading symbol
func (e *FuturesExchange) GetSymbol() string {
	return e.symbol
}

// Connect establishes WebSocket connection to Binance Futures
func (e *FuturesExchange) Connect(ctx context.Context) error {
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

	go e.readMessages()

	return nil
}

// Close closes the WebSocket connection
func (e *FuturesExchange) Close() error {
	if e.cancel != nil {
		e.cancel()
	}

	if e.wsConn != nil {
		select {
		case <-e.done:
		default:
			close(e.done)
		}

		err := e.wsConn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		if err != nil {
			log.Printf("[%s] Error sending close message: %v", e.GetName(), err)
		}

		select {
		case <-time.After(time.Second):
		}

		e.updateConnectionStatus(false)
		return e.wsConn.Close()
	}
	return nil
}

// GetSnapshot fetches the initial orderbook snapshot via REST API
func (e *FuturesExchange) GetSnapshot(ctx context.Context) (*exchange.Snapshot, error) {
	log.Printf("[%s] Fetching orderbook snapshot...", e.GetName())

	req, err := http.NewRequestWithContext(ctx, "GET", e.restURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		e.incrementErrorCount()
		return nil, fmt.Errorf("failed to get snapshot: %w", err)
	}
	defer resp.Body.Close()

	var binanceSnapshot SnapshotResponse
	if err := json.NewDecoder(resp.Body).Decode(&binanceSnapshot); err != nil {
		e.incrementErrorCount()
		return nil, fmt.Errorf("failed to decode snapshot: %w", err)
	}

	snapshot := e.convertSnapshot(&binanceSnapshot)
	log.Printf("[%s] Snapshot received: %d bids, %d asks, lastUpdateId=%d",
		e.GetName(), len(snapshot.Bids), len(snapshot.Asks), snapshot.LastUpdateID)
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
			var msg WSMessage
			if err := e.wsConn.ReadJSON(&msg); err != nil {
				e.incrementErrorCount()
				log.Printf("[%s] WebSocket read error: %v", e.GetName(), err)
				return
			}

			e.incrementMessageCount()
			e.updateLastPing()

			canonicalUpdate := e.convertDepthUpdate(&msg.Data)

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

// convertSnapshot converts Binance snapshot to canonical format
func (e *FuturesExchange) convertSnapshot(snapshot *SnapshotResponse) *exchange.Snapshot {
	bids := make([]exchange.PriceLevel, len(snapshot.Bids))
	for i, bid := range snapshot.Bids {
		bids[i] = exchange.PriceLevel{
			Price:    bid[0],
			Quantity: bid[1],
		}
	}

	asks := make([]exchange.PriceLevel, len(snapshot.Asks))
	for i, ask := range snapshot.Asks {
		asks[i] = exchange.PriceLevel{
			Price:    ask[0],
			Quantity: ask[1],
		}
	}

	return &exchange.Snapshot{
		Exchange:     e.GetName(),
		Symbol:       e.symbol,
		LastUpdateID: snapshot.LastUpdateID,
		Bids:         bids,
		Asks:         asks,
		Timestamp:    time.Now(),
	}
}

// convertDepthUpdate converts Binance depth update to canonical format
func (e *FuturesExchange) convertDepthUpdate(update *DepthUpdate) *exchange.DepthUpdate {
	bids := make([]exchange.PriceLevel, len(update.Bids))
	for i, bid := range update.Bids {
		bids[i] = exchange.PriceLevel{
			Price:    bid[0],
			Quantity: bid[1],
		}
	}

	asks := make([]exchange.PriceLevel, len(update.Asks))
	for i, ask := range update.Asks {
		asks[i] = exchange.PriceLevel{
			Price:    ask[0],
			Quantity: ask[1],
		}
	}

	return &exchange.DepthUpdate{
		Exchange:      e.GetName(),
		Symbol:        update.Symbol,
		EventTime:     time.UnixMilli(update.EventTime),
		FirstUpdateID: update.FirstUpdateID,
		FinalUpdateID: update.FinalUpdateID,
		PrevUpdateID:  update.PrevUpdateID,
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
