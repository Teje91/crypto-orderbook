# Crypto Orderbook - Complete Architecture Explanation

## The Big Picture (Layman's Terms)

```
┌─────────────────┐
│  EXCHANGES      │  (Binance, Bybit, etc.)
│  (Data Sources) │
└────────┬────────┘
         │ WebSocket Streams (Real-time price updates)
         ↓
┌─────────────────┐
│   GO BACKEND    │  (Your Kitchen - Processes data)
│   Port 8086     │
│                 │
│  • Connects to 9 exchanges
│  • Maintains orderbooks
│  • Calculates statistics
│  • Broadcasts via WebSocket
└────────┬────────┘
         │ WebSocket (ws://localhost:8086/ws)
         ↓
┌─────────────────┐
│  REACT FRONTEND │  (Your Display - Shows pretty charts)
│   Port 5173     │
│                 │
│  • Receives data
│  • Renders tables/charts
│  • Updates in real-time
└────────┬────────┘
         ↓
┌─────────────────┐
│  YOUR BROWSER   │  (What you see!)
│  localhost:5173 │
└─────────────────┘
```

---

## Part 1: Backend Flow (Go)

### Step 1: Application Starts (`cmd/main.go:22-81`)

```go
// You run: go run ./cmd/main.go
func main() {
    // Parse command-line arguments
    symbol := "BTCUSDT"  // What crypto pair to monitor

    // Configure which exchanges to connect to
    cfg := config.NewMultiExchange([...]
        {Name: "binance", Symbol: symbol},
        {Name: "bybit", Symbol: symbol},
        // ... 7 more exchanges
    )

    // Start everything!
    runMultiExchange(cfg)
}
```

### Step 2: For Each Exchange, Run in Parallel (`cmd/main.go:126-217`)

```go
// Create 9 goroutines (like threads) - one per exchange
for _, exConfig := range cfg.Exchanges {
    go func(exCfg config.ExchangeConfig) {
        // 1. Create an orderbook (empty book to fill)
        ob := orderbook.New()

        // 2. Connect to exchange's WebSocket
        ex.Connect(ctx)

        // 3. Get initial snapshot (current state)
        snapshot := ex.GetSnapshot(ctx)
        ob.LoadSnapshot(snapshot)

        // 4. Listen for updates forever
        for update := range ex.Updates() {
            ob.HandleDepthUpdate(update)
        }
    }(exConfig)
}
```

**What's a Goroutine?**
Think of it like hiring 9 employees, each watching one exchange simultaneously. They all work at the same time (concurrently).

### Step 3: WebSocket Server Broadcasts Data (`internal/websocket/server.go`)

```go
// Server runs on port 8086
// Every time orderbook updates, send to all connected browsers
func (s *Server) broadcastOrderbook() {
    for {
        // For each exchange
        for name, orderbook := range s.orderbooks {
            // Get current state
            bids, asks := orderbook.GetLevels(tickSize)
            stats := orderbook.GetStats()

            // Send to all connected clients (your browser)
            s.broadcast(websocketMessage{
                Type: "orderbook",
                Exchange: name,
                Bids: bids,
                Asks: asks,
            })
        }
    }
}
```

---

## Part 2: Frontend Flow (React)

### Step 1: App Loads (`frontend/src/main.tsx`)

```tsx
// Browser opens localhost:5173
// React mounts the App component
ReactDOM.createRoot(document.getElementById('root')!).render(
  <App />
)
```

### Step 2: Connect to Backend (`frontend/src/hooks/useWebSocket.ts`)

```typescript
export function useWebSocket(url: string) {
  // Create WebSocket connection to backend
  const ws = new WebSocket('ws://localhost:8086/ws')

  // When we receive a message
  ws.onmessage = (event) => {
    const data = JSON.parse(event.data)

    if (data.type === 'orderbook') {
      // Update orderbook state
      setOrderbooks(prev => ({
        ...prev,
        [data.exchange]: data
      }))
    }

    if (data.type === 'stats') {
      // Update statistics
      setStats(prev => ({
        ...prev,
        [data.exchange]: data
      }))
    }
  }
}
```

### Step 3: Display Data (`frontend/src/App.tsx`)

```tsx
function App() {
  // Get data from WebSocket
  const { orderbooks, stats } = useWebSocket('ws://localhost:8086/ws')

  return (
    <div>
      {/* Show stats table */}
      <StatsTable data={stats} />

      {/* Show orderbook cards */}
      {Object.entries(orderbooks).map(([exchange, book]) => (
        <OrderbookCard
          exchange={exchange}
          bids={book.bids}
          asks={book.asks}
        />
      ))}

      {/* Show charts */}
      <LiquidityChart data={stats} />
    </div>
  )
}
```

**React's Magic:** Every time `orderbooks` or `stats` changes, React automatically re-renders the UI! You don't manually update the display.

---

## Part 3: How Real-Time Works (WebSocket)

### Traditional Way (HTTP):
```
Browser: "Hey backend, what's the price?" (request)
Backend: "It's $111,000" (response)
[2 seconds pass]
Browser: "Hey backend, what's the price now?" (request)
Backend: "It's $111,005" (response)
```

**Problem:** Browser has to keep asking. Slow and wasteful!

### WebSocket Way:
```
Browser: "Hey backend, keep me updated!" (connect once)
Backend: "$111,000" (push update)
Backend: "$111,005" (push update)
Backend: "$111,010" (push update)
Backend: "$111,003" (push update)
```

**Benefit:** Backend pushes updates instantly. No asking needed!

---

## Key Technologies Explained

### Go (Backend Language)
- **Fast:** Compiled language (turns into machine code)
- **Concurrent:** Can handle thousands of connections easily
- **Simple:** Less code than Java/C++

### React (Frontend Library)
- **Component-Based:** Build UI like LEGO blocks
- **Reactive:** UI updates automatically when data changes
- **Popular:** Huge ecosystem, lots of help online

### WebSocket Protocol
- **Bidirectional:** Both sides can send messages
- **Persistent:** Connection stays open
- **Low Latency:** Updates arrive in milliseconds

### TypeScript (Frontend Language)
- **JavaScript + Types:** Catches errors before running
- **Better IDE Support:** Autocomplete, hints
- **Safer Code:** Can't accidentally use wrong data types

---

## File Structure (What Each Folder Does)

```
crypto-orderbook/
├── cmd/
│   └── main.go                    # 🚀 START HERE - Application entry point
│
├── internal/                      # Backend logic (private code)
│   ├── exchange/                  # 📡 Exchange connectors
│   │   ├── binance/
│   │   ├── bybit/
│   │   └── types.go              # Interface all exchanges implement
│   │
│   ├── orderbook/                 # 📖 Orderbook engine
│   │   └── orderbook.go          # Maintains bid/ask levels
│   │
│   ├── websocket/                 # 🔌 WebSocket server
│   │   └── server.go             # Broadcasts to frontend
│   │
│   └── types/                     # 📦 Data structures
│       └── types.go              # PriceLevel, Stats, etc.
│
├── frontend/                      # React application
│   ├── src/
│   │   ├── main.tsx              # 🚀 START HERE - Frontend entry
│   │   ├── App.tsx               # Main component
│   │   ├── components/           # UI components
│   │   │   ├── StatsTable.tsx
│   │   │   ├── OrderbookCard.tsx
│   │   │   └── LiquidityChart.tsx
│   │   └── hooks/                # React hooks (reusable logic)
│   │       └── useWebSocket.ts   # WebSocket connection
│   │
│   └── package.json              # Dependencies list
│
└── go.mod                         # Go dependencies list
```

---

## Common Questions

### Q: Why are there two servers running?
**A:**
- **Backend (Port 8086):** Go server that collects data from exchanges
- **Frontend (Port 5173):** Vite dev server that serves your React app

They're separate but communicate via WebSocket!

### Q: What happens if I close my browser?
**A:**
- Backend keeps running and collecting data
- When you reopen browser, frontend reconnects via WebSocket

### Q: Can multiple people use this at once?
**A:**
- YES! Backend can handle many browser connections simultaneously
- Each browser gets its own WebSocket connection

### Q: Where is the data stored?
**A:**
- **Nowhere!** It's all in memory (RAM)
- Data disappears when you stop the backend
- That's why it starts fresh each time

### Q: Why Go for backend instead of Node.js?
**A:**
- Go handles concurrent connections better
- Lower memory usage
- Faster execution
- Built-in concurrency (goroutines)

---

## Next Steps: Deployment (Making it Live)

Currently running on `localhost` (only you can see it).
To make it public, you need to deploy both parts:

### Option 1: Simple Deployment
- **Frontend:** Deploy to Vercel (FREE, easy)
- **Backend:** Deploy to Fly.io or Railway (FREE tier available)

### Option 2: All-in-One
- Deploy both to a VPS (Digital Ocean, Linode)
- Use Docker to package everything

We'll cover deployment in detail next!

---

## Glossary

- **Orderbook:** List of all buy (bid) and sell (ask) orders
- **Bid:** Buy order - "I want to buy at this price"
- **Ask:** Sell order - "I want to sell at this price"
- **Spread:** Difference between best bid and best ask
- **Liquidity:** How much can be bought/sold without moving price
- **WebSocket:** Two-way real-time communication channel
- **Goroutine:** Lightweight thread in Go
- **Hook:** Reusable React logic (starts with `use`)
- **Component:** Reusable UI building block in React
