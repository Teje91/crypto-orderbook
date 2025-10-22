package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"time"

	"orderbook/internal/config"
	"orderbook/internal/exchange"
	"orderbook/internal/factory"
	"orderbook/internal/orderbook"
	"orderbook/internal/websocket"

	"github.com/shopspring/decimal"
)

func main() {
	// Parse command line flags
	var symbol = flag.String("symbol", "BTCUSDT", "Trading symbol to monitor")
	var logInterval = flag.Duration("log-interval", 10*time.Second, "Interval for logging orderbook stats")
	flag.Parse()

	// Load configuration with multiple exchanges
	cfg := config.NewMultiExchange([]config.ExchangeConfig{
		{
			Name:   exchange.Binancef,
			Symbol: *symbol,
		},
		{
			Name:   exchange.Binance,
			Symbol: *symbol,
		},
		{
			Name:   exchange.Bybitf,
			Symbol: *symbol,
		},
		{
			Name:   exchange.Bybit,
			Symbol: *symbol,
		},
		{
			Name:   exchange.Kraken,
			Symbol: *symbol,
		},
		/*{
			Name:   exchange.OKX,
			Symbol: *symbol,
		},*/
		{
			Name:   exchange.Coinbase,
			Symbol: *symbol,
		},
		{
			Name:   exchange.Asterdexf,
			Symbol: *symbol,
		},
		{
			Name:   exchange.BingX,
			Symbol: *symbol,
		},
		/*{
			Name:   exchange.BingXf,
			Symbol: *symbol,
		},*/
	})

	// Set up signal handling
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	log.Printf("Starting multi-exchange orderbook monitor for %s", *symbol)
	log.Printf("Exchanges: %v", getExchangeNames(cfg.Exchanges))
	log.Printf("Log interval: %v", *logInterval)

	runMultiExchange(cfg, *logInterval, interrupt)
}

func getExchangeNames(exchanges []config.ExchangeConfig) []string {
	names := make([]string, len(exchanges))
	for i, ex := range exchanges {
		names[i] = string(ex.Name)
	}
	return names
}

type orderbookWithName struct {
	name string
	ob   *orderbook.OrderBook
}

const (
	colorReset   = "\033[0m"
	colorYellow  = "\033[33m"
	colorGreen   = "\033[32m"
	colorRed     = "\033[31m"
	colorMagenta = "\033[35m"
	colorBold    = "\033[1m"
)

func runMultiExchange(cfg config.Config, logInterval time.Duration, interrupt chan os.Signal) {
	if len(cfg.Exchanges) == 0 {
		log.Fatal("No exchanges configured")
	}

	ctx := context.Background()
	var wg sync.WaitGroup
	orderbooks := make([]*orderbookWithName, 0, len(cfg.Exchanges))
	orderbooksMap := make(map[string]*orderbook.OrderBook)
	var obMutex sync.Mutex
	done := make(chan struct{})

	// Get port from environment variable (Railway) or default to 8086
	port := os.Getenv("PORT")
	if port == "" {
		port = "8086"
	}

	// Start WebSocket server
	wsServer := websocket.NewServer(orderbooksMap, port)
	go func() {
		if err := wsServer.Start(); err != nil {
			log.Fatalf("WebSocket server error: %v", err)
		}
	}()

	// Create an orderbook for each exchange
	for _, exConfig := range cfg.Exchanges {
		wg.Add(1)
		go func(exCfg config.ExchangeConfig) {
			defer wg.Done()

			log.Printf("[%s] Starting connection...", exCfg.Name)

			// Create exchange-specific orderbook
			ob := orderbook.New()

			// Create exchange instance
			ex, err := factory.NewExchange(factory.ExchangeConfig{
				Name:   exCfg.Name,
				Symbol: exCfg.Symbol,
			})
			if err != nil {
				log.Printf("[%s] Failed to create exchange: %v", exCfg.Name, err)
				return
			}

			// Connect
			if err := ex.Connect(ctx); err != nil {
				log.Printf("[%s] Failed to connect: %v", exCfg.Name, err)
				return
			}
			defer ex.Close()

			// Get snapshot
			snapshot, err := ex.GetSnapshot(ctx)
			if err != nil {
				log.Printf("[%s] Failed to get snapshot: %v", exCfg.Name, err)
				return
			}

			if err := ob.LoadSnapshot(snapshot); err != nil {
				log.Printf("[%s] Failed to load snapshot: %v", exCfg.Name, err)
				return
			}

			// Process updates in background
			updatesDone := make(chan struct{})
			go func() {
				defer close(updatesDone)
				for update := range ex.Updates() {
					ob.HandleDepthUpdate(update)
				}
			}()

			// Reinitialization check
			go func() {
				ticker := time.NewTicker(cfg.App.ReinitCheckInterval)
				defer ticker.Stop()

				for {
					select {
					case <-ticker.C:
						ob.CheckAndReinitialize(func() (*exchange.Snapshot, error) {
							return ex.GetSnapshot(ctx)
						})
					case <-updatesDone:
						return
					case <-done:
						return
					case <-interrupt:
						return
					}
				}
			}()

			ob.ProcessBufferedEvents()
			log.Printf("[%s] Orderbook initialized", exCfg.Name)

			// Add orderbook to shared collections
			obMutex.Lock()
			orderbooks = append(orderbooks, &orderbookWithName{
				name: string(exCfg.Name),
				ob:   ob,
			})
			orderbooksMap[string(exCfg.Name)] = ob
			obMutex.Unlock()

			// Wait for shutdown
			select {
			case <-updatesDone:
				log.Printf("[%s] Connection closed", exCfg.Name)
			case <-done:
				log.Printf("[%s] Shutting down...", exCfg.Name)
			case <-interrupt:
				log.Printf("[%s] Shutting down...", exCfg.Name)
			}
		}(exConfig)
	}

	// Centralized logging ticker
	go func() {
		ticker := time.NewTicker(logInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				obMutex.Lock()
				printCombinedStats(orderbooks)
				obMutex.Unlock()
			case <-done:
				return
			case <-interrupt:
				return
			}
		}
	}()

	// Wait for interrupt
	<-interrupt
	close(done)

	wg.Wait()
	log.Println("All exchanges closed. Goodbye!")
}

func printCombinedStats(orderbooks []*orderbookWithName) {
	if len(orderbooks) == 0 {
		return
	}

	fmt.Println()

	for i, obn := range orderbooks {
		if !obn.ob.IsInitialized() {
			continue
		}

		stats := obn.ob.GetStats()
		midPrice := stats.BestBid.Add(stats.BestAsk).Div(decimal.NewFromInt(2))

		// print exchange name
		fmt.Printf("%s%s%s", colorBold, obn.name, colorReset)
		// Print exchange header
		fmt.Printf("  Mid: %s%10s%s │ Spread: %s%8s%s | BB: %s%10s%s │ BA: %s%10s%s\n",
			colorYellow, midPrice.StringFixed(2), colorReset,
			colorMagenta, stats.Spread.StringFixed(4), colorReset,
			colorGreen, stats.BestBid.StringFixed(2), colorReset,
			colorRed, stats.BestAsk.StringFixed(2), colorReset)

		// Print depth metrics
		fmt.Printf("  DEPTH 0.5%% Bids: %s%9s%s │ Asks: %s%9s%s │ Δ: %s%10s%s\n",
			colorGreen, stats.BidLiquidity05Pct.StringFixed(2), colorReset,
			colorRed, stats.AskLiquidity05Pct.StringFixed(2), colorReset,
			getDeltaColor(stats.DeltaLiquidity05Pct), stats.DeltaLiquidity05Pct.StringFixed(2), colorReset)

		fmt.Printf("  DEPTH 2%%:  Bids: %s%9s%s │ Asks: %s%9s%s │ Δ: %s%10s%s\n",
			colorGreen, stats.BidLiquidity2Pct.StringFixed(2), colorReset,
			colorRed, stats.AskLiquidity2Pct.StringFixed(2), colorReset,
			getDeltaColor(stats.DeltaLiquidity2Pct), stats.DeltaLiquidity2Pct.StringFixed(2), colorReset)

		fmt.Printf("  DEPTH 10%%  Bids: %s%9s%s │ Asks: %s%9s%s │ Δ: %s%10s%s\n",
			colorGreen, stats.BidLiquidity10Pct.StringFixed(2), colorReset,
			colorRed, stats.AskLiquidity10Pct.StringFixed(2), colorReset,
			getDeltaColor(stats.DeltaLiquidity10Pct), stats.DeltaLiquidity10Pct.StringFixed(2), colorReset)

		fmt.Printf("  TOTAL QTY: Bids: %s%9s%s │ Asks: %s%9s%s\n",
			colorGreen, stats.TotalBidsQty.StringFixed(2), colorReset,
			colorRed, stats.TotalAsksQty.StringFixed(2), colorReset)

		// Print separator between exchanges (but not after the last one)
		if i < len(orderbooks)-1 {
			fmt.Println()
		}
	}
}

func getDeltaColor(delta decimal.Decimal) string {
	if delta.GreaterThan(decimal.Zero) {
		return colorGreen
	} else if delta.LessThan(decimal.Zero) {
		return colorRed
	}
	return colorYellow
}
