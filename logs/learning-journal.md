# Learning Journal: Crypto Orderbook Aggregator
**Your personal guide to understanding this codebase**

Last Updated: October 22, 2025

---

## Table of Contents
1. [Core Concepts](#core-concepts)
2. [Architecture Overview](#architecture-overview)
3. [Data Flow](#data-flow)
4. [Exchange Patterns](#exchange-patterns)
5. [Go Language Concepts](#go-language-concepts)
6. [Code Walkthrough](#code-walkthrough)
7. [Common Issues & Solutions](#common-issues--solutions)
8. [Questions & Answers](#questions--answers)

---

## Core Concepts

### What is an Orderbook? 📚

An orderbook is like a **restaurant menu with prices** where buyers and sellers meet:

- **Asks** = People wanting to SELL (sellers at a market saying "I'll sell you this for $100")
- **Bids** = People wanting to BUY (buyers saying "I'll buy that for $98")
- **Spread** = The difference between lowest ask and highest bid

**Example for Bitcoin:**
```
ASKS (people selling):
$108,200.00 → 0.5 BTC available
$108,190.00 → 1.2 BTC available
$108,180.00 → 0.8 BTC available ← Best ask (cheapest seller)

SPREAD = $30 difference

$108,150.00 → 2.1 BTC wanted ← Best bid (highest buyer)
$108,140.00 → 0.9 BTC wanted
$108,130.00 → 1.5 BTC wanted
BIDS (people buying):
```

**Key Terms:**
- **Price Level**: A specific price point with quantity available
- **Best Bid**: Highest price someone will pay (top of bid side)
- **Best Ask**: Lowest price someone will sell (top of ask side)
- **Mid Price**: Average of best bid and best ask
- **Depth**: How many price levels deep the orderbook goes
- **Liquidity**: Total amount available to trade at various prices

### What Does This App Do?

This application is an **Orderbook Aggregator** - it:
1. Connects to multiple exchanges (Binance, Kraken, Coinbase, etc.)
2. Collects their orderbooks in real-time
3. Combines them to find the **best prices across all exchanges**
4. Shows you where you can buy cheapest / sell highest

**Why is this useful?**
- Different exchanges have different prices
- You might get $108,150 on Binance but $108,180 on Kraken
- This app shows you the best deal across all exchanges

---

## Architecture Overview

### High-Level Structure

```
┌─────────────────────────────────────────────────────────┐
│                    FRONTEND (React)                      │
│              Displays orderbook in browser               │
└─────────────────────────────────────────────────────────┘
                            ▲
                            │ WebSocket
                            │
┌─────────────────────────────────────────────────────────┐
│              BACKEND (Go Application)                    │
│  ┌───────────────────────────────────────────────────┐  │
│  │         WebSocket Server                          │  │
│  │    Sends updates to frontend                      │  │
│  └───────────────────────────────────────────────────┘  │
│                            ▲                             │
│                            │                             │
│  ┌───────────────────────────────────────────────────┐  │
│  │           AGGREGATOR                              │  │
│  │   Combines orderbooks from all exchanges          │  │
│  │   Finds best bid/ask across all                   │  │
│  └───────────────────────────────────────────────────┘  │
│      ▲         ▲         ▲         ▲         ▲          │
│      │         │         │         │         │          │
│  ┌───────┐ ┌──────┐ ┌──────┐ ┌──────┐ ┌───────┐       │
│  │Binance│ │Kraken│ │Coinb.│ │ BingX│ │  OKX  │       │
│  │Adapter│ │Adaptr│ │Adaptr│ │Adaptr│ │Adapter│       │
│  └───────┘ └──────┘ └──────┘ └──────┘ └───────┘       │
└─────────────────────────────────────────────────────────┘
      ▲         ▲         ▲         ▲         ▲
      │         │         │         │         │
      │ WebSocket/REST API connections        │
      │         │         │         │         │
┌─────────────────────────────────────────────────────────┐
│        EXCHANGES (Binance, Kraken, etc.)                 │
│            External crypto exchanges                     │
└─────────────────────────────────────────────────────────┘
```

### Directory Structure

```
crypto-orderbook/
├── cmd/
│   └── main.go                 # Application entry point
├── internal/
│   ├── exchange/               # Exchange adapters
│   │   ├── interface.go        # Common interface all exchanges implement
│   │   ├── binance/           # Binance implementation
│   │   ├── kraken/            # Kraken implementation
│   │   ├── coinbase/          # Coinbase implementation
│   │   ├── bingx/             # BingX implementation
│   │   └── okx/               # OKX implementation
│   ├── aggregator/            # Combines multiple exchanges
│   │   └── aggregator.go
│   └── websocket/             # WebSocket server for frontend
│       └── server.go
├── web/                       # Frontend React app
└── logs/                      # Documentation and logs
```

---

## Data Flow

### The Complete Journey: Exchange → Your Screen

```
1. EXCHANGE (Kraken servers)
   └─ WebSocket sends: snapshot + continuous updates

2. EXCHANGE ADAPTER (internal/exchange/kraken/spot.go)
   ├─ Receives WebSocket messages
   ├─ Parses JSON
   ├─ Converts to standard format
   └─ Sends to channel → updateChan

3. AGGREGATOR (internal/aggregator/aggregator.go)
   ├─ Reads from ALL exchange channels
   ├─ Combines orderbooks
   ├─ Finds best bid/ask across exchanges
   └─ Sends to channel → outputChan

4. WEBSOCKET SERVER (internal/websocket/server.go)
   ├─ Reads from aggregator channel
   ├─ Converts to JSON
   └─ Sends to browser via WebSocket

5. FRONTEND (React app)
   ├─ Receives WebSocket messages
   ├─ Updates UI
   └─ Shows prices on screen
```

### Step 1: Initial Snapshot (The Starting Point)

Think of this like taking a **photograph** of the entire menu:

```
1. Your app starts up
2. Connects to exchange (Binance, Kraken, etc.)
3. Exchange says: "Here's the FULL orderbook right now"
   → 1000s of price levels
   → All current bids and asks
   → This is the SNAPSHOT
```

**Analogy**: Like walking into a store and getting a complete price list.

### Step 2: Real-Time Updates (Keeping Current)

After you have the snapshot, you get **incremental updates** via WebSocket:

```
WebSocket: "Hey, someone just bought 0.5 BTC at $108,180"
→ Remove that ask from your orderbook

WebSocket: "New buyer! Someone wants 3.0 BTC at $108,145"
→ Add that bid to your orderbook

WebSocket: "Seller canceled! Remove the $108,200 ask"
→ Remove that ask
```

**Analogy**: Like getting text message updates: "Item X sold out", "New item added", "Price changed"

---

## Exchange Patterns

Different exchanges provide data in different ways. This app handles 3 patterns:

### Pattern 1: Snapshot via WebSocket + WebSocket Updates
**Used by:** Kraken, Coinbase, BingX

```
1. Connect to WebSocket
2. WebSocket immediately sends: "Here's the full orderbook" (snapshot)
3. Then continuously sends: "Here's what changed" (updates)
```

**Code example** (BingX):
```go
// First message has action="all" (this is the snapshot)
if msg.Data.Action == "all" {
    e.handleSnapshot(&msg)  // Store the full orderbook
}

// Later messages have action="update" (these are changes)
if msg.Data.Action == "update" {
    e.handleUpdate(&msg)  // Apply the changes
}
```

**Analogy**: Like Netflix - connect once, get everything streaming.

**Pros:**
- Most efficient
- Single connection
- Real-time updates

**Cons:**
- More complex to implement
- Must handle connection drops

---

### Pattern 2: Snapshot via REST API + WebSocket Updates
**Used by:** Binance, Bybit, AsterDEX

```
1. First, make HTTP request: "GET /api/orderbook" → Get snapshot
2. Then connect WebSocket → Get continuous updates
3. Apply updates on top of the snapshot
```

**Why?** Some exchanges separate these for efficiency:
- REST for bulk data (snapshot)
- WebSocket for speed (updates)

**Code example** (Binance):
```go
// Step 1: Get snapshot via REST
func (e *SpotExchange) GetSnapshot(ctx context.Context) (*exchange.Snapshot, error) {
    resp, err := client.Do(req)  // HTTP GET request
    json.Unmarshal(body, &snapshot)
    return snapshot
}

// Step 2: Subscribe to WebSocket for updates
func (e *SpotExchange) Connect(ctx context.Context) error {
    conn.WriteJSON(subscribeMsg)  // Subscribe to depth stream
    go e.readMessages()           // Read updates forever
}
```

**Analogy**: Download a map file first (REST), then get live traffic updates (WebSocket).

**Pros:**
- Reliable snapshot
- Fast updates via WebSocket

**Cons:**
- Two connections to manage
- Must sync snapshot with first update

---

### Pattern 3: Polling (No WebSocket)
**Used by:** OKX (currently having issues)

```
1. Every 1 second: "GET /api/orderbook" → Get full snapshot
2. Treat each snapshot as an "update"
3. Repeat forever
```

**Code example** (OKX):
```go
func (e *SpotExchange) pollLoop() {
    ticker := time.NewTicker(1 * time.Second)  // Every 1 second

    for {
        case <-ticker.C:
            snapshot, err := e.GetSnapshot(ctx)  // HTTP request
            // Send snapshot as if it's an update
            e.updateChan <- convertToUpdate(snapshot)
    }
}
```

**Why?**
- OKX might not offer public WebSocket
- Or WebSocket implementation isn't done yet

**Analogy**: Like refreshing a webpage every second instead of having it auto-update.

**Pros:**
- Simple to implement
- No WebSocket connection to maintain

**Cons:**
- Slower (1 second delay)
- More bandwidth usage
- Misses rapid changes between polls
- Can hit rate limits

---

## Go Language Concepts

Since you're new to Go, here are key concepts used throughout this codebase:

### Goroutines (Concurrency)

```go
go e.readMessages()  // Run this in the background
go e.pingLoop()      // Run this in the background too
// Main program continues without waiting
```

**What are goroutines?**
- Lightweight threads managed by Go
- Allow multiple tasks to run simultaneously
- Use the `go` keyword to start one

**Analogy**: Like opening multiple browser tabs - they all run at the same time.

**In this app:**
- Each exchange runs in its own goroutine
- Aggregator runs in a goroutine
- WebSocket server runs in a goroutine
- They all communicate via channels

---

### Channels (Communication Between Goroutines)

```go
// Creating a channel (like a pipe)
updateChan := make(chan *exchange.DepthUpdate, 5000)

// Sending data into the channel (producer)
updateChan <- update

// Receiving data from the channel (consumer)
update := <-updateChan

// Closing a channel (no more data will be sent)
close(updateChan)
```

**What is a channel?**
Think of it like a **pipe** or **conveyor belt**:
- One goroutine **puts** data into the channel
- Another goroutine **takes** data out of the channel
- If the pipe is full, sender waits (blocks)
- If the pipe is empty, receiver waits (blocks)

**Buffer Size (the 5000 number):**
```go
make(chan Type, 5000)  // Buffered channel - holds 5000 items
```
- If exchanges send 100 updates/second
- Buffer = 5000 updates
- You have ~50 seconds before pipe fills up
- If full, sender must wait or skip

**Real example from code:**
```go
// Kraken sends updates via this channel
case e.updateChan <- update:  // Try to send
    // Success!
default:
    // Channel full! Log warning and skip
    log.Printf("Warning: update channel full, skipping update")
}
```

---

### Select Statement (Channel Multiplexing)

```go
select {
case update := <-updateChan:  // If update available
    processUpdate(update)
case <-done:                   // If done signal received
    return
case <-time.After(5 * time.Second):  // If 5 seconds pass
    log.Println("Timeout!")
}
```

**What does select do?**
- Waits on multiple channel operations
- Whichever happens first, that case runs
- Like `switch` but for channels

**Analogy**: Like waiting for multiple delivery drivers - whoever arrives first, you answer the door for them.

---

### Defer (Cleanup)

```go
func doSomething() {
    file, _ := os.Open("data.txt")
    defer file.Close()  // This runs when function exits

    // Do stuff with file...
    // Even if error occurs, file.Close() will run
}
```

**What is defer?**
- Schedules a function call to run when the current function returns
- Runs even if there's a panic/error
- Multiple defers run in reverse order (LIFO)

**Analogy**: Like setting a reminder to lock the door when you leave - no matter how you leave, it happens.

**Common use cases:**
- Closing connections
- Unlocking mutexes
- Closing files
- Cleaning up resources

---

### Interfaces (Polymorphism)

```go
// Define interface (contract)
type Exchange interface {
    Connect(ctx context.Context) error
    GetSnapshot(ctx context.Context) (*Snapshot, error)
    Updates() <-chan *DepthUpdate
    Close() error
}

// Any type that has these methods implements the interface
type BinanceExchange struct { ... }
func (e *BinanceExchange) Connect(...) error { ... }
func (e *BinanceExchange) GetSnapshot(...) (*Snapshot, error) { ... }
// etc.

// Now you can use it anywhere that accepts Exchange
func ProcessExchange(ex Exchange) {
    ex.Connect(ctx)
    // Works with Binance, Kraken, any exchange!
}
```

**Why interfaces?**
- Write code that works with ANY exchange
- Aggregator doesn't care if it's Binance or Kraken
- Each exchange implements the same interface differently

**In this app:**
All exchanges implement the `Exchange` interface defined in `internal/exchange/interface.go`

---

### Structs (Custom Data Types)

```go
type SpotExchange struct {
    symbol     string                      // Trading pair (e.g., "BTCUSDT")
    wsConn     *websocket.Conn            // WebSocket connection
    updateChan chan *exchange.DepthUpdate // Channel for updates
    health     atomic.Value                // Thread-safe health status
}

// Creating an instance
exchange := &SpotExchange{
    symbol:     "BTCUSDT",
    updateChan: make(chan *exchange.DepthUpdate, 5000),
}

// Accessing fields
fmt.Println(exchange.symbol)  // "BTCUSDT"
```

**What are structs?**
- Like objects in other languages
- Group related data together
- Can have methods attached to them

---

### Methods (Functions on Structs)

```go
// Method with receiver (e *SpotExchange)
func (e *SpotExchange) GetName() string {
    return "Binance"
}

// Calling the method
exchange := &SpotExchange{}
name := exchange.GetName()  // "Binance"
```

**Pointer receivers vs Value receivers:**
```go
func (e *SpotExchange) Modify() {
    // Can modify e, changes persist
    e.symbol = "ETHUSDT"
}

func (e SpotExchange) ReadOnly() {
    // Gets a copy of e, changes don't persist
}
```

**Rule of thumb:** Use pointer receivers (`*`) almost always.

---

### Atomic Operations (Thread-Safe Variables)

```go
type SpotExchange struct {
    health       atomic.Value  // Thread-safe
    reconnecting atomic.Bool   // Thread-safe
}

// Storing a value
e.health.Store(healthStatus)

// Loading a value
status := e.health.Load().(exchange.HealthStatus)

// Compare and swap (atomic operation)
if e.reconnecting.CompareAndSwap(false, true) {
    // Only one goroutine gets here
    doReconnect()
}
```

**Why atomic?**
Multiple goroutines might access the same variable. Without atomics:
```go
// UNSAFE with multiple goroutines
if !e.reconnecting {
    e.reconnecting = true  // Race condition!
    reconnect()
}
```

Two goroutines could both see `reconnecting = false` and both try to reconnect!

**Atomic version is safe:**
```go
if e.reconnecting.CompareAndSwap(false, true) {
    // Only ONE goroutine succeeds
    reconnect()
}
```

---

### Context (Cancellation & Timeouts)

```go
// Create context with cancel
ctx, cancel := context.WithCancel(context.Background())

// Later, cancel all operations
cancel()  // Everything using this context stops

// Context with timeout
ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
defer cancel()

// Check if context is cancelled
select {
case <-ctx.Done():
    return  // Context cancelled, exit
default:
    // Continue working
}
```

**What is context for?**
- Signal cancellation across goroutines
- Set timeouts
- Pass request-scoped values

**In this app:**
```go
// When app shuts down
cancel()  // Tells all exchanges to stop

// In exchange goroutines
select {
case <-e.ctx.Done():  // Context cancelled
    return  // Clean up and exit
}
```

---

## Code Walkthrough

### Starting Point: `cmd/main.go`

This is where the app begins. Let's trace what happens:

```go
func main() {
    // 1. Create exchange instances
    binanceEx := binance.NewSpotExchange(binance.Config{Symbol: "BTCUSDT"})
    krakenEx := kraken.NewSpotExchange(kraken.Config{Symbol: "BTCUSDT"})

    // 2. Create aggregator with all exchanges
    exchanges := []exchange.Exchange{binanceEx, krakenEx}
    agg := aggregator.New(exchanges)

    // 3. Start aggregator (begins collecting data)
    agg.Start()

    // 4. Start WebSocket server (for frontend)
    wsServer := websocket.NewServer(agg)
    wsServer.Start()

    // 5. Wait forever (until Ctrl+C)
    select {}
}
```

---

### Exchange Adapter: `internal/exchange/kraken/spot.go`

Let's walk through Kraken as an example:

#### 1. Creating the Exchange

```go
func NewSpotExchange(config Config) *SpotExchange {
    ctx, cancel := context.WithCancel(context.Background())

    ex := &SpotExchange{
        symbol:     config.Symbol,
        wsURL:      "wss://ws.kraken.com/v2",
        updateChan: make(chan *exchange.DepthUpdate, 5000),
        done:       make(chan struct{}),
        ctx:        ctx,
        cancel:     cancel,
    }

    return ex
}
```

#### 2. Connecting to Exchange

```go
func (e *SpotExchange) Connect(ctx context.Context) error {
    // Open WebSocket connection
    conn, _, err := dialer.DialContext(ctx, e.wsURL, nil)
    if err != nil {
        return fmt.Errorf("connection failed: %w", err)
    }
    e.wsConn = conn

    // Subscribe to orderbook
    subMsg := SubscriptionMessage{
        Method: "subscribe",
        Params: SubscriptionParams{
            Channel: "book",
            Symbol:  []string{e.symbol},
            Depth:   1000,
        },
    }

    if err := conn.WriteJSON(subMsg); err != nil {
        return fmt.Errorf("failed to subscribe: %w", err)
    }

    // Start background goroutines
    go e.readMessages()  // Read WebSocket messages forever
    go e.pingLoop()      // Send pings to keep connection alive

    return nil
}
```

#### 3. Reading Messages (The Main Loop)

```go
func (e *SpotExchange) readMessages() {
    defer e.updateConnectionStatus(false)  // Mark disconnected when this exits

    for {
        select {
        case <-e.ctx.Done():  // App shutting down
            return
        case <-e.done:        // Exchange closing
            return
        default:
            // Read next WebSocket message
            _, message, err := e.wsConn.ReadMessage()
            if err != nil {
                log.Printf("WebSocket error: %v", err)
                go e.reconnect()  // Try to reconnect
                return
            }

            // Parse JSON
            var msg WSMessage
            if err := json.Unmarshal(message, &msg); err != nil {
                continue  // Skip invalid messages
            }

            // Handle different message types
            if msg.Type == "snapshot" {
                e.handleSnapshot(&msg)
            } else if msg.Type == "update" {
                e.handleUpdate(&msg)
            }
        }
    }
}
```

#### 4. Handling Snapshot

```go
func (e *SpotExchange) handleSnapshot(msg *WSMessage) {
    // Convert from Kraken format to our standard format
    snapshot := &exchange.Snapshot{
        Exchange:     e.GetName(),
        Symbol:       e.symbol,
        Bids:         convertBids(msg.Data.Bids),
        Asks:         convertAsks(msg.Data.Asks),
        Timestamp:    time.Now(),
    }

    // Store it
    e.snapshotMu.Lock()
    e.snapshot = snapshot
    e.snapshotReceived = true
    e.snapshotMu.Unlock()

    log.Printf("[Kraken] Snapshot received: %d bids, %d asks",
        len(snapshot.Bids), len(snapshot.Asks))
}
```

#### 5. Handling Updates

```go
func (e *SpotExchange) handleUpdate(msg *WSMessage) {
    // Convert to standard format
    update := &exchange.DepthUpdate{
        Exchange:  e.GetName(),
        Symbol:    e.symbol,
        Bids:      convertBids(msg.Data.Bids),
        Asks:      convertAsks(msg.Data.Asks),
        EventTime: time.Now(),
    }

    // Send to aggregator via channel
    select {
    case e.updateChan <- update:
        // Successfully sent
    case <-e.ctx.Done():
        return
    default:
        // Channel full, skip this update
        log.Printf("[Kraken] Channel full, skipping update")
    }
}
```

#### 6. Heartbeat (Keeping Connection Alive)

```go
func (e *SpotExchange) pingLoop() {
    ticker := time.NewTicker(30 * time.Second)  // Every 30 seconds
    defer ticker.Stop()

    reqID := 1
    for {
        select {
        case <-e.ctx.Done():
            return
        case <-e.done:
            return
        case <-ticker.C:
            // Send ping
            pingMsg := PingRequest{
                Method: "ping",
                ReqID:  reqID,
            }
            reqID++

            if err := e.wsConn.WriteJSON(pingMsg); err != nil {
                log.Printf("[Kraken] Ping failed: %v", err)
                return
            }
        }
    }
}
```

#### 7. Reconnection (Recovering from Errors)

```go
func (e *SpotExchange) reconnect() {
    // Prevent multiple simultaneous reconnections
    if !e.reconnecting.CompareAndSwap(false, true) {
        return  // Already reconnecting
    }
    defer e.reconnecting.Store(false)

    log.Printf("[Kraken] Starting reconnection...")

    // Close old connection
    if e.wsConn != nil {
        e.wsConn.Close()
        e.wsConn = nil
    }

    // Wait before reconnecting
    time.Sleep(5 * time.Second)

    // Try up to 10 times with exponential backoff
    for attempt := 1; attempt <= 10; attempt++ {
        ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
        err := e.Connect(ctx)
        cancel()

        if err == nil {
            log.Printf("[Kraken] Reconnection successful!")
            return
        }

        log.Printf("[Kraken] Attempt %d failed: %v", attempt, err)

        // Exponential backoff: 5s, 10s, 15s, ..., max 30s
        backoff := time.Duration(attempt) * 5 * time.Second
        if backoff > 30*time.Second {
            backoff = 30 * time.Second
        }
        time.Sleep(backoff)
    }

    log.Printf("[Kraken] Reconnection failed after 10 attempts")
}
```

---

### Aggregator: `internal/aggregator/aggregator.go`

The aggregator combines orderbooks from all exchanges:

```go
func (a *Aggregator) Run() {
    for {
        select {
        // Receive update from ANY exchange
        case update := <-binanceUpdates:
            a.processUpdate(update)
        case update := <-krakenUpdates:
            a.processUpdate(update)
        case update := <-coinbaseUpdates:
            a.processUpdate(update)
        }
    }
}

func (a *Aggregator) processUpdate(update *exchange.DepthUpdate) {
    // Apply update to that exchange's orderbook
    orderbook := a.orderbooks[update.Exchange]
    orderbook.ApplyUpdate(update)

    // Find best prices across ALL exchanges
    bestBid := a.findBestBid()
    bestAsk := a.findBestAsk()

    // Send to frontend
    a.outputChan <- AggregatedOrderbook{
        BestBid: bestBid,
        BestAsk: bestAsk,
        Timestamp: time.Now(),
    }
}
```

---

## Common Issues & Solutions

### Issue 1: "Update channel full, skipping update"

**What it means:**
- Updates are arriving faster than they can be processed
- The channel buffer (5000 items) is full
- New updates are being dropped

**Why it happens:**
- Exchange sends 100+ updates per second
- Processing is slower than 100/sec
- Buffer fills up in ~50 seconds

**Solution:**
```go
// Increased buffer from 1000 to 5000
updateChan: make(chan *exchange.DepthUpdate, 5000)
```

**When to worry:**
- If you see this constantly, buffer is too small
- Occasional warnings are OK (burst of activity)

---

### Issue 2: "WebSocket close 1006" (Zombie Connection)

**What it means:**
- Connection closed unexpectedly
- No close frame was received
- Connection died without proper goodbye

**Why it happens:**
- Network hiccup
- Server restarted
- Connection timeout
- Firewall closed connection

**Solution:**
Implement heartbeat (ping/pong):
```go
func (e *SpotExchange) pingLoop() {
    ticker := time.NewTicker(30 * time.Second)
    for {
        case <-ticker.C:
            // Send ping
            e.wsConn.WriteJSON(PingMessage{})
    }
}
```

And automatic reconnection:
```go
func (e *SpotExchange) readMessages() {
    _, message, err := e.wsConn.ReadMessage()
    if err != nil {
        go e.reconnect()  // Automatically reconnect
        return
    }
}
```

---

### Issue 3: "Failed to decode snapshot: invalid character '<'"

**What it means:**
- Expected JSON but got HTML instead
- API is returning an error page

**Why it happens:**
- API endpoint is wrong
- IP address is blocked
- Rate limiting kicked in
- API requires authentication

**Solution:**
Add better error logging:
```go
if resp.StatusCode != http.StatusOK {
    bodyBytes, _ := io.ReadAll(resp.Body)
    return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(bodyBytes))
}
```

This will show you what the server is actually returning.

---

### Issue 4: Race Conditions (Concurrent Access)

**What it means:**
- Multiple goroutines accessing same variable
- Results are unpredictable
- Can cause crashes or wrong values

**Example of UNSAFE code:**
```go
// UNSAFE - race condition!
if !e.reconnecting {
    e.reconnecting = true
    reconnect()
}
```

**Solution: Use atomic operations:**
```go
// SAFE - atomic operation
if e.reconnecting.CompareAndSwap(false, true) {
    reconnect()
}
```

Or use mutexes:
```go
e.mu.Lock()
if !e.reconnecting {
    e.reconnecting = true
    reconnect()
}
e.mu.Unlock()
```

---

## Questions & Answers

### Q: Why do we need heartbeat/ping logic?

**A:** WebSocket connections can become "zombies":
- Connection appears open but is actually dead
- No errors are generated
- You stop receiving updates but don't know it!

Heartbeat detects this:
```
Every 30s: Send ping →
                      ← Receive pong

If no pong after 60s:
    "Connection is dead! Reconnect!"
```

---

### Q: What's the difference between WebSocket and REST?

**A:**

**REST API (HTTP Request):**
```
You: "Hey Binance, what's the orderbook?"
Binance: "Here it is: [big JSON response]"
[Connection closes]

You: "Hey Binance, what's the orderbook now?"
Binance: "Here it is: [big JSON response]"
[Connection closes]
```
- Request/response
- Must keep asking
- Higher latency
- More bandwidth

**WebSocket:**
```
You: "Hey Kraken, I want orderbook updates"
Kraken: "OK, staying connected..."
[Time passes]
Kraken: "Update: price changed"
Kraken: "Update: order filled"
[Connection stays open]
```
- Persistent connection
- Server pushes to you
- Real-time
- More efficient

---

### Q: What are Go channels and why use them?

**A:** Channels are pipes for communication between goroutines:

```go
// Create pipe
ch := make(chan int, 10)

// Goroutine 1: Producer
go func() {
    ch <- 42  // Put data in pipe
}()

// Goroutine 2: Consumer
go func() {
    value := <-ch  // Take data out of pipe
    fmt.Println(value)  // 42
}()
```

**Why use channels instead of shared variables?**
- Thread-safe by design
- No race conditions
- Go philosophy: "Share memory by communicating"

---

### Q: Why do some exchanges use REST + WebSocket instead of just WebSocket?

**A:** Efficiency and reliability:

**Snapshot via REST:**
- Get bulk data all at once
- Reliable (can retry if fails)
- No need to maintain connection while fetching

**Updates via WebSocket:**
- Fast, real-time
- Low latency
- Efficient for small changes

**Analogy:**
- REST = Downloading a map (one-time, bulk)
- WebSocket = Live traffic updates (continuous, small)

---

### Q: What happens if an exchange disconnects?

**A:** The app handles this automatically:

1. **Detection:**
   - ReadMessage() returns error
   - Or ping timeout (no pong received)

2. **Reconnection:**
   ```go
   go e.reconnect()  // Start reconnection in background
   ```

3. **Exponential backoff:**
   - Try 1: Wait 5s
   - Try 2: Wait 10s
   - Try 3: Wait 15s
   - ...
   - Try 10: Wait 30s (max)

4. **Success:**
   - Resubscribe to orderbook
   - Get new snapshot
   - Resume sending updates

5. **Frontend impact:**
   - That exchange's data becomes stale
   - Other exchanges continue working
   - When reconnected, fresh data appears

---

### Q: Why is OKX using polling instead of WebSocket?

**A:** Currently OKX implementation uses polling because:

1. **Simpler to implement initially**
   - Just make HTTP requests
   - No WebSocket connection management

2. **Public WebSocket might not be available**
   - Or might require authentication
   - Or has different API

3. **Temporary solution**
   - Can be upgraded to WebSocket later
   - Works for now but less efficient

**Current issue:**
OKX is returning HTML instead of JSON, likely:
- IP blocking (Render datacenter IPs)
- Rate limiting (polling every 1 second)
- Requires authentication

---

### Q: What does "thread-safe" mean?

**A:** Safe for multiple goroutines to use simultaneously:

**NOT thread-safe:**
```go
// Multiple goroutines doing this = disaster
count := 0
count++  // Read, increment, write
```

**Thread-safe with mutex:**
```go
var mu sync.Mutex
count := 0

// Goroutine 1
mu.Lock()
count++
mu.Unlock()

// Goroutine 2
mu.Lock()
count++
mu.Unlock()
```

**Thread-safe with atomic:**
```go
var count atomic.Int64
count.Add(1)  // Atomic operation
```

---

## Tips for Exploring This Codebase

### 1. Start with the Flow
Follow the data:
1. `cmd/main.go` - Entry point
2. `internal/exchange/binance/spot.go` - Pick one exchange
3. Follow `Connect()` → `readMessages()` → `handleUpdate()`
4. See where `updateChan` goes
5. Find it in aggregator

### 2. Use Your IDE
- **Go to Definition** (Cmd+Click on Mac) - Jump to where something is defined
- **Find Usages** - See where a function/variable is used
- **Type Hierarchy** - See what implements an interface

### 3. Add Logging
When confused, add logs:
```go
log.Printf("[DEBUG] Received update: %+v", update)
```

### 4. Run Locally
```bash
go run ./cmd/main.go
```
See the logs in real-time, understand the flow.

### 5. Read Exchange Documentation
- Binance API docs: https://binance-docs.github.io/apidocs/spot/en/
- Kraken API docs: https://docs.kraken.com/websockets-v2/
- Understand what messages exchanges send

### 6. Experiment
- Comment out exchanges to simplify
- Change log levels to see more/less
- Modify buffer sizes and see what happens

---

## Next Topics to Explore

When you're ready, dive into these:

1. **Error Handling Patterns**
   - How errors propagate
   - When to retry vs fail
   - Logging best practices

2. **Testing**
   - How to test WebSocket connections
   - Mocking exchanges
   - Integration tests

3. **Performance**
   - Profiling goroutines
   - Memory usage
   - CPU usage
   - Optimizing hot paths

4. **Deployment**
   - Docker containers
   - Environment variables
   - Health checks
   - Monitoring

---

## Your Notes Section

> Add your own notes, questions, and discoveries here as you explore!

### Date: [Add date]
**What I learned today:**

**Questions I still have:**

**Experiments I tried:**

---

## Resources

### Go Learning
- [Tour of Go](https://go.dev/tour/) - Official interactive tutorial
- [Go by Example](https://gobyexample.com/) - Code examples
- [Effective Go](https://go.dev/doc/effective_go) - Best practices

### WebSocket & APIs
- [WebSocket MDN Docs](https://developer.mozilla.org/en-US/docs/Web/API/WebSockets_API)
- [REST API Tutorial](https://restfulapi.net/)

### This Project
- GitHub: [Link to your repo]
- Render Dashboard: [Your Render URL]
- Frontend: [Your frontend URL]

---

**Remember:** It's okay to not understand everything at once. Take it one piece at a time, experiment, and ask questions!
