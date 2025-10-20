package orderbook

import (
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"orderbook/internal/exchange"
	"orderbook/internal/types"

	"github.com/shopspring/decimal"
)

// OrderBook manages the real-time order book state
type OrderBook struct {
	mu           sync.RWMutex
	bids         map[string]types.PriceLevel
	asks         map[string]types.PriceLevel
	lastUpdateID int64
	eventBuffer  []*exchange.DepthUpdate
	initialized  bool
	stats        types.Stats
	currentTick  types.TickLevel
	// Cached best bid/ask for performance
	bestBid   decimal.Decimal
	bestAsk   decimal.Decimal
	bidLevels int
	askLevels int
}

// New creates a new OrderBook instance
func New() *OrderBook {
	return &OrderBook{
		bids:        make(map[string]types.PriceLevel),
		asks:        make(map[string]types.PriceLevel),
		eventBuffer: make([]*exchange.DepthUpdate, 0),
		currentTick: types.Tick1, // Default to 1.0 tick size
		bestBid:     decimal.Zero,
		bestAsk:     decimal.Zero,
		stats: types.Stats{
			ConnectionTime: time.Now(),
		},
	}
}

// LoadSnapshot initializes the orderbook with a snapshot from the exchange
func (ob *OrderBook) LoadSnapshot(snapshot *exchange.Snapshot) error {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	ob.lastUpdateID = snapshot.LastUpdateID
	ob.bids = make(map[string]types.PriceLevel)
	ob.asks = make(map[string]types.PriceLevel)
	ob.bestBid = decimal.Zero
	ob.bestAsk = decimal.NewFromFloat(999999999)

	for _, bid := range snapshot.Bids {
		price, err := decimal.NewFromString(bid.Price)
		if err != nil {
			return fmt.Errorf("invalid bid price %s: %w", bid.Price, err)
		}
		qty, err := decimal.NewFromString(bid.Quantity)
		if err != nil {
			return fmt.Errorf("invalid bid quantity %s: %w", bid.Quantity, err)
		}
		if !qty.IsZero() {
			ob.bids[bid.Price] = types.PriceLevel{Price: price, Quantity: qty}
			// Update best bid
			if price.GreaterThan(ob.bestBid) {
				ob.bestBid = price
			}
		}
	}

	for _, ask := range snapshot.Asks {
		price, err := decimal.NewFromString(ask.Price)
		if err != nil {
			return fmt.Errorf("invalid ask price %s: %w", ask.Price, err)
		}
		qty, err := decimal.NewFromString(ask.Quantity)
		if err != nil {
			return fmt.Errorf("invalid ask quantity %s: %w", ask.Quantity, err)
		}
		if !qty.IsZero() {
			ob.asks[ask.Price] = types.PriceLevel{Price: price, Quantity: qty}
			// Update best ask
			if price.LessThan(ob.bestAsk) {
				ob.bestAsk = price
			}
		}
	}

	ob.updateStats()
	return nil
}

// HandleDepthUpdate processes a depth update from the WebSocket stream
func (ob *OrderBook) HandleDepthUpdate(update *exchange.DepthUpdate) {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	if !ob.initialized {
		ob.eventBuffer = append(ob.eventBuffer, update)
		return
	}

	expectedPrevID := ob.lastUpdateID
	if update.PrevUpdateID != expectedPrevID {
		if update.FirstUpdateID <= expectedPrevID+1 && update.FinalUpdateID > expectedPrevID {
			//log.Printf("Accepting overlapping event: U=%d, u=%d, expected_pu=%d, got_pu=%d", update.FirstUpdateID, update.FinalUpdateID, expectedPrevID, update.PrevUpdateID)
			ob.applyUpdate(update)
			return
		}

		//log.Printf("Sequence gap: expected pu=%d, got pu=%d. Buffering event...", expectedPrevID, update.PrevUpdateID)
		ob.eventBuffer = append(ob.eventBuffer, update)
		return
	}

	ob.applyUpdate(update)
}

// ProcessBufferedEvents processes any buffered events after snapshot load
func (ob *OrderBook) ProcessBufferedEvents() {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	// For exchanges without sequence IDs (like Coinbase), just apply all buffered events
	// and mark as initialized
	if ob.lastUpdateID == 0 {
		log.Printf("Exchange doesn't use update IDs, applying all %d buffered events", len(ob.eventBuffer))
		for _, event := range ob.eventBuffer {
			ob.applyUpdate(event)
		}
		ob.eventBuffer = nil
		ob.initialized = true
		return
	}

	validEvents := make([]*exchange.DepthUpdate, 0)

	for _, event := range ob.eventBuffer {
		if event.FinalUpdateID <= ob.lastUpdateID {
			log.Printf("Discarding old buffered event: u=%d <= lastUpdateId=%d",
				event.FinalUpdateID, ob.lastUpdateID)
			continue
		}

		if event.FirstUpdateID <= ob.lastUpdateID+1 && event.FinalUpdateID > ob.lastUpdateID {
			validEvents = append(validEvents, event)
			log.Printf("Found valid buffered event: U=%d, u=%d, lastUpdateId=%d",
				event.FirstUpdateID, event.FinalUpdateID, ob.lastUpdateID)
		}
	}

	if len(validEvents) == 0 {
		log.Printf("No valid events found in buffer, dropping all and starting fresh")
		ob.eventBuffer = nil
		ob.initialized = true
		return
	}

	sort.Slice(validEvents, func(i, j int) bool {
		return validEvents[i].FirstUpdateID < validEvents[j].FirstUpdateID
	})

	ob.eventBuffer = nil

	for _, event := range validEvents {
		if event.FirstUpdateID <= ob.lastUpdateID+1 {
			ob.applyUpdate(event)
		}
	}

	ob.initialized = true
	log.Printf("Orderbook initialized with %d valid events", len(validEvents))
}

// CheckAndReinitialize checks if the orderbook needs reinitialization
func (ob *OrderBook) CheckAndReinitialize(getSnapshot func() (*exchange.Snapshot, error)) {
	ob.mu.RLock()
	shouldReinit := len(ob.eventBuffer) > 100
	bufferLen := len(ob.eventBuffer)
	initialized := ob.initialized
	ob.mu.RUnlock()

	if shouldReinit {
		log.Printf("Reinitializing due to buffer accumulation: %d events", bufferLen)
		ob.mu.Lock()
		ob.initialized = false
		ob.mu.Unlock()

		snapshot, err := getSnapshot()
		if err != nil {
			log.Printf("Failed to reinitialize: %v", err)
			return
		}

		if err := ob.LoadSnapshot(snapshot); err != nil {
			log.Printf("Failed to load snapshot during reinitialize: %v", err)
			return
		}

		ob.ProcessBufferedEvents()
	} else if initialized && bufferLen > 0 && bufferLen%10 == 0 {
		log.Printf("Buffer status: %d events pending", bufferLen)
	}
}

// SetTickLevel changes the current tick level for price aggregation
func (ob *OrderBook) SetTickLevel(tick types.TickLevel) {
	ob.mu.Lock()
	defer ob.mu.Unlock()
	ob.currentTick = tick
}

// GetTickLevel returns the current tick level
func (ob *OrderBook) GetTickLevel() types.TickLevel {
	ob.mu.RLock()
	defer ob.mu.RUnlock()
	return ob.currentTick
}

// GetBids returns a copy of the current bid levels
func (ob *OrderBook) GetBids() map[string]types.PriceLevel {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	bids := make(map[string]types.PriceLevel)
	for k, v := range ob.bids {
		bids[k] = v
	}
	return bids
}

// GetAsks returns a copy of the current ask levels
func (ob *OrderBook) GetAsks() map[string]types.PriceLevel {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	asks := make(map[string]types.PriceLevel)
	for k, v := range ob.asks {
		asks[k] = v
	}
	return asks
}

// GetStats returns a copy of the current statistics
func (ob *OrderBook) GetStats() types.Stats {
	ob.mu.RLock()
	defer ob.mu.RUnlock()
	return ob.stats
}

// IsInitialized returns whether the orderbook is initialized
func (ob *OrderBook) IsInitialized() bool {
	ob.mu.RLock()
	defer ob.mu.RUnlock()
	return ob.initialized
}

// GetBufferLength returns the current buffer length
func (ob *OrderBook) GetBufferLength() int {
	ob.mu.RLock()
	defer ob.mu.RUnlock()
	return len(ob.eventBuffer)
}

// applyUpdate applies a depth update to the orderbook (must be called with mutex locked)
func (ob *OrderBook) applyUpdate(update *exchange.DepthUpdate) {
	bestBidChanged := false
	bestAskChanged := false

	for _, bid := range update.Bids {
		price := bid.Price
		qty, _ := decimal.NewFromString(bid.Quantity)
		priceDecimal, _ := decimal.NewFromString(price)

		if qty.IsZero() {
			// Remove bid level
			if _, exists := ob.bids[price]; exists {
				delete(ob.bids, price)
				// Check if this was the best bid
				if priceDecimal.Equal(ob.bestBid) {
					bestBidChanged = true
				}
			}
		} else {
			// Add/update bid level
			ob.bids[price] = types.PriceLevel{Price: priceDecimal, Quantity: qty}
			// Check if this is a new best bid
			if priceDecimal.GreaterThan(ob.bestBid) {
				ob.bestBid = priceDecimal
			}
		}
	}

	for _, ask := range update.Asks {
		price := ask.Price
		qty, _ := decimal.NewFromString(ask.Quantity)
		priceDecimal, _ := decimal.NewFromString(price)

		if qty.IsZero() {
			// Remove ask level
			if _, exists := ob.asks[price]; exists {
				delete(ob.asks, price)
				// Check if this was the best ask
				if priceDecimal.Equal(ob.bestAsk) {
					bestAskChanged = true
				}
			}
		} else {
			// Add/update ask level
			ob.asks[price] = types.PriceLevel{Price: priceDecimal, Quantity: qty}
			// Check if this is a new best ask
			if priceDecimal.LessThan(ob.bestAsk) {
				ob.bestAsk = priceDecimal
			}
		}
	}

	// Recalculate best prices only if needed
	if bestBidChanged {
		ob.recalculateBestBid()
	}
	if bestAskChanged {
		ob.recalculateBestAsk()
	}

	ob.lastUpdateID = update.FinalUpdateID
	ob.stats.EventsProcessed++
	ob.stats.LastEventTime = update.EventTime
	ob.updateCachedStats()
}

// updateStats recalculates orderbook statistics (must be called with mutex locked)
func (ob *OrderBook) updateStats() {
	ob.bidLevels = len(ob.bids)
	ob.askLevels = len(ob.asks)

	ob.bestBid = decimal.Zero
	ob.bestAsk = decimal.NewFromFloat(999999999)

	if len(ob.bids) > 0 {
		for _, level := range ob.bids {
			if level.Price.GreaterThan(ob.bestBid) {
				ob.bestBid = level.Price
			}
		}
	}

	if len(ob.asks) > 0 {
		for _, level := range ob.asks {
			if level.Price.LessThan(ob.bestAsk) {
				ob.bestAsk = level.Price
			}
		}
		if ob.bestAsk.Equal(decimal.NewFromFloat(999999999)) {
			ob.bestAsk = decimal.Zero
		}
	} else {
		ob.bestAsk = decimal.Zero
	}

	ob.updateCachedStats()
}

// updateCachedStats updates the stats structure with cached values (must be called with mutex locked)
func (ob *OrderBook) updateCachedStats() {
	ob.stats.BidLevels = ob.bidLevels
	ob.stats.AskLevels = ob.askLevels
	ob.stats.BufferedEvents = len(ob.eventBuffer)
	ob.stats.BestBid = ob.bestBid
	ob.stats.BestAsk = ob.bestAsk

	if !ob.bestBid.IsZero() && !ob.bestAsk.IsZero() && ob.bestAsk.GreaterThan(ob.bestBid) {
		ob.stats.Spread = ob.bestAsk.Sub(ob.bestBid)
	} else {
		ob.stats.Spread = decimal.Zero
	}

	// Calculate liquidity depth metrics
	ob.calculateLiquidityDepth()
}

// calculateLiquidityDepth calculates liquidity at various depth percentages (must be called with mutex locked)
func (ob *OrderBook) calculateLiquidityDepth() {
	if ob.bestBid.IsZero() || ob.bestAsk.IsZero() {
		ob.stats.BidLiquidity05Pct = decimal.Zero
		ob.stats.AskLiquidity05Pct = decimal.Zero
		ob.stats.BidLiquidity2Pct = decimal.Zero
		ob.stats.AskLiquidity2Pct = decimal.Zero
		ob.stats.BidLiquidity10Pct = decimal.Zero
		ob.stats.AskLiquidity10Pct = decimal.Zero
		ob.stats.DeltaLiquidity05Pct = decimal.Zero
		ob.stats.DeltaLiquidity2Pct = decimal.Zero
		ob.stats.DeltaLiquidity10Pct = decimal.Zero
		ob.stats.TotalBidsQty = decimal.Zero
		ob.stats.TotalAsksQty = decimal.Zero
		return
	}

	// Calculate mid price
	midPrice := ob.bestBid.Add(ob.bestAsk).Div(decimal.NewFromInt(2))

	// Calculate price thresholds
	threshold05Pct := midPrice.Mul(decimal.NewFromFloat(0.005))
	threshold2Pct := midPrice.Mul(decimal.NewFromFloat(0.02))
	threshold10Pct := midPrice.Mul(decimal.NewFromFloat(0.10))

	// Calculate bid side liquidity
	bidLiq05 := decimal.Zero
	bidLiq2 := decimal.Zero
	bidLiq10 := decimal.Zero
	totalBidsQty := decimal.Zero
	minBid05Pct := midPrice.Sub(threshold05Pct)
	minBid2Pct := midPrice.Sub(threshold2Pct)
	minBid10Pct := midPrice.Sub(threshold10Pct)

	for _, level := range ob.bids {
		totalBidsQty = totalBidsQty.Add(level.Quantity)
		if level.Price.GreaterThanOrEqual(minBid05Pct) {
			bidLiq05 = bidLiq05.Add(level.Quantity)
		}
		if level.Price.GreaterThanOrEqual(minBid2Pct) {
			bidLiq2 = bidLiq2.Add(level.Quantity)
		}
		if level.Price.GreaterThanOrEqual(minBid10Pct) {
			bidLiq10 = bidLiq10.Add(level.Quantity)
		}
	}

	// Calculate ask side liquidity
	askLiq05 := decimal.Zero
	askLiq2 := decimal.Zero
	askLiq10 := decimal.Zero
	totalAsksQty := decimal.Zero
	maxAsk05Pct := midPrice.Add(threshold05Pct)
	maxAsk2Pct := midPrice.Add(threshold2Pct)
	maxAsk10Pct := midPrice.Add(threshold10Pct)

	for _, level := range ob.asks {
		totalAsksQty = totalAsksQty.Add(level.Quantity)
		if level.Price.LessThanOrEqual(maxAsk05Pct) {
			askLiq05 = askLiq05.Add(level.Quantity)
		}
		if level.Price.LessThanOrEqual(maxAsk2Pct) {
			askLiq2 = askLiq2.Add(level.Quantity)
		}
		if level.Price.LessThanOrEqual(maxAsk10Pct) {
			askLiq10 = askLiq10.Add(level.Quantity)
		}
	}

	// Update stats
	ob.stats.BidLiquidity05Pct = bidLiq05
	ob.stats.AskLiquidity05Pct = askLiq05
	ob.stats.BidLiquidity2Pct = bidLiq2
	ob.stats.AskLiquidity2Pct = askLiq2
	ob.stats.BidLiquidity10Pct = bidLiq10
	ob.stats.AskLiquidity10Pct = askLiq10
	ob.stats.TotalBidsQty = totalBidsQty
	ob.stats.TotalAsksQty = totalAsksQty

	// Calculate deltas (positive = more bid liquidity = bullish pressure)
	ob.stats.DeltaLiquidity05Pct = bidLiq05.Sub(askLiq05)
	ob.stats.DeltaLiquidity2Pct = bidLiq2.Sub(askLiq2)
	ob.stats.DeltaLiquidity10Pct = bidLiq10.Sub(askLiq10)
	ob.stats.TotalDelta = totalBidsQty.Sub(totalAsksQty)
}

// recalculateBestBid recalculates the best bid when the current best is removed
func (ob *OrderBook) recalculateBestBid() {
	ob.bestBid = decimal.Zero
	for _, level := range ob.bids {
		if level.Price.GreaterThan(ob.bestBid) {
			ob.bestBid = level.Price
		}
	}
}

// recalculateBestAsk recalculates the best ask when the current best is removed
func (ob *OrderBook) recalculateBestAsk() {
	ob.bestAsk = decimal.NewFromFloat(999999999)
	for _, level := range ob.asks {
		if level.Price.LessThan(ob.bestAsk) {
			ob.bestAsk = level.Price
		}
	}
	if ob.bestAsk.Equal(decimal.NewFromFloat(999999999)) {
		ob.bestAsk = decimal.Zero
	}
}
