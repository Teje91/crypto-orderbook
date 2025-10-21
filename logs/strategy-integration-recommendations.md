# Orderbook Aggregator - Hedge Fund Strategy Integration

**Date**: 2025-10-20
**Fund Strategy**: Long-short market neutral with funding rate optimization
**AUM**: $22M

---

## Fund Strategy Overview

### Core Strategy
- **Long**: Quality assets (spot/perps) with negative or low positive funding
- **Short**: High-volatility assets (memecoins) with positive funding
- **Beta-adjusted** positions for market neutrality
- **Execution**: TWAP orders due to large position sizes that impact orderbooks

### Key Challenges
1. $22M positions take up significant orderbook depth
2. Need to minimize market impact during execution
3. Require optimal funding rate identification across venues
4. Wick catching strategies need real-time liquidity monitoring

---

## Current App Value Proposition

### 1. TWAP Execution Optimization
**What the app provides:**
- Total quantity metrics across 9 exchanges
- Depth analysis at 0.5%, 2%, and 10% levels
- Real-time liquidity comparison

**How to use it:**
- Before executing large orders, check "Total QTY" across exchanges
- Identify venues with sufficient depth to absorb position size
- Route orders to exchanges with deepest books to minimize slippage

**Example:**
```
Need to buy 500 BTC:
- Binance: 2,177 BTC total bids → Route 60% here
- Bybit: 428 BTC total bids → Route 20% here
- Coinbase: 150 BTC total bids → Avoid (would move market)
```

### 2. Wick Catching Strategy
**What the app provides:**
- Bid/Ask delta (Δ) showing order flow imbalance
- Depth metrics showing thin liquidity zones
- Real-time updates every 200ms

**How to use it:**
- Large negative Δ → Sell pressure, potential downward wick
- Thin asks at 0.5% depth → Potential for upward wick on low volume
- Monitor depth changes to identify support/resistance

**Example:**
```
Wick Setup Detection:
binance  Δ: -145 BTC (asks >> bids)
  DEPTH 0.5% Asks: 25 BTC (thin!)
→ High probability of downward wick
→ Set limit buy orders at support levels
```

### 3. Market Impact Pre-Trade Analysis
**What the app provides:**
- Spread visibility (execution cost)
- Cumulative depth at each price level
- Cross-exchange comparison

**How to use it:**
- Check total liquidity > position size before entry
- Identify best execution venue for size
- Estimate slippage based on depth at 2%/10%

---

## Priority Feature Recommendations

### **PRIORITY 1: Funding Rate Integration** 🔥

**Implementation:**
Add funding rate display for perpetual futures exchanges:

```
Exchange Stats Table:
Exchange    | Mid Price | Spread  | Funding Rate | Next Funding | Total Liquidity
binancef    | 110,000   | 0.10    | -0.01%      | 2h 15m       | 6,809 BTC
bybitf      | 110,001   | 0.10    | +0.02%      | 2h 15m       | 428 BTC
asterdexf   | 109,999   | 0.10    | -0.005%     | 1h 45m       | 242 BTC
```

**Color coding:**
- Green: Negative funding (get paid to long)
- Red: Positive funding (pay to long / get paid to short)
- Yellow: Near zero funding

**Data sources:**
- Binance: `/fapi/v1/premiumIndex`
- Bybit: `/v5/market/tickers`
- OKX: `/api/v5/public/funding-rate`

**Why critical:**
- Core to strategy: Find longs with negative funding, shorts with positive
- Enables cross-exchange funding arbitrage identification
- Time entries around funding intervals for optimal rates

**Estimated effort:** 2-3 hours

---

### **PRIORITY 2: Slippage Calculator**

**Implementation:**
Add input field for position size, calculate expected execution:

```
Position Size Calculator:
┌─────────────────────────────────────────┐
│ Buy 100 BTC                             │
└─────────────────────────────────────────┘

Execution Analysis:
Exchange   | Avg Fill Price | Slippage | % of Order
-----------|----------------|----------|------------
Binance    | $110,088      | 0.08%    | 60%
Bybit      | $110,165      | 0.15%    | 25%
OKX        | $110,142      | 0.13%    | 15%
-----------|----------------|----------|------------
AGGREGATE  | $110,112      | 0.10%    | 100%

Market Impact: $11,200 (0.10% of $11M notional)
```

**Calculation logic:**
- Walk through orderbook levels
- Calculate weighted average fill price
- Compare to mid price for slippage %

**Why valuable:**
- $22M positions need pre-trade impact analysis
- Can save 5-10 bps on execution ($11k-$22k per trade)
- Informs TWAP order sizing

**Estimated effort:** 3-4 hours

---

### **PRIORITY 3: Liquidity Alerts**

**Implementation:**
Configurable threshold alerts via Telegram/Discord/Email:

```
Alert Conditions:
✓ Total BTC liquidity < 500 BTC (whale detection)
✓ Bid/Ask delta > 200 BTC (wick opportunity)
✓ Funding rate < -0.02% (long opportunity)
✓ Spread > 0.5% (avoid trading)
```

**Alert examples:**
```
🚨 LOW LIQUIDITY ALERT
Total BTC bid depth: 347 BTC (↓ 45% from average)
Time: 2025-10-20 03:15 UTC
Venue: All exchanges

🎯 WICK OPPORTUNITY
BTC Bid/Ask Δ: -247 BTC
Depth 0.5% asks: 18 BTC (extremely thin)
Estimated wick potential: 2.3%
```

**Why valuable:**
- Wick catching requires catching extreme liquidity events
- Can't monitor orderbook 24/7
- Automated opportunity identification

**Estimated effort:** 2-3 hours

---

## Advanced Feature Roadmap

### 4. Historical Depth Patterns
**Goal:** Identify when liquidity typically dries up

**Features:**
- Hourly/daily depth averages
- Liquidity heatmap by time of day
- Pattern recognition (weekend thinning, Asian session depth)

**Use case:** Time large executions for maximum liquidity windows

**Estimated effort:** 1-2 days

---

### 5. Market Impact Simulator
**Goal:** Simulate TWAP execution scenarios

**Features:**
```
TWAP Simulator:
Order: Buy 500 BTC over 4 hours
Execution: Every 5 minutes (48 orders × 10.42 BTC)

Expected Results:
- Average fill: $110,250
- Total slippage: 0.12% ($660k)
- Market impact: 0.08%
- Recommended split: 60% Binance, 25% Bybit, 15% OKX

Risk Analysis:
- Liquidity risk: LOW (depth >> order size)
- Execution risk: MEDIUM (4hr window allows recovery)
```

**Why valuable:**
- Optimize TWAP parameters (duration, interval, venue split)
- Pre-trade analysis for risk management
- Backtesting execution strategies

**Estimated effort:** 3-5 days

---

### 6. Cross-Exchange Funding Arbitrage Scanner
**Goal:** Identify delta-neutral funding opportunities

**Features:**
```
🔥 FUNDING ARBITRAGE OPPORTUNITY

Setup:
Long:  100 BTC @ Binance Perps (-0.02% funding)
Short: 100 BTC @ Bybit Perps (+0.05% funding)

Returns:
- Funding collected: 0.07% every 8 hours
- Daily return: 0.21% ($46,200/day on $22M notional)
- APY: 76.6% (if rates persist)

Risks:
- Funding rate change
- Exchange basis risk
- Liquidation risk (monitor margin)

Required Actions:
1. Post $2.2M margin on each exchange (10x leverage)
2. Monitor basis every hour
3. Rebalance if spread > 0.5%
```

**Why valuable:**
- Core to market-neutral strategy
- Automates opportunity scanning
- Risk-free return enhancement

**Estimated effort:** 2-3 days

---

## Specific Use Cases

### Use Case 1: Entering a $5M Long Position

**Step-by-step workflow:**

1. **Check funding rates** across all perp exchanges
   - Target: Negative funding (get paid to hold)
   - If all positive, consider spot instead

2. **Analyze total liquidity**
   ```
   Total bid liquidity at 2% depth:
   - Binance: $5.2M ✓
   - Bybit: $1.8M
   - OKX: $900k
   Total available: $7.9M (> $5M target ✓)
   ```

3. **Calculate market impact**
   - Run slippage calculator for $5M buy
   - Expected slippage: 0.08% = $4,000 cost

4. **Monitor bid/ask delta**
   - Wait for positive delta (bids strengthening)
   - Avoid entering during sell pressure

5. **Execute TWAP**
   - Split: 65% Binance, 25% Bybit, 10% OKX
   - Duration: 2 hours
   - Order size: $41,667 every 5 minutes

---

### Use Case 2: Wick Catching Setup

**Monitoring dashboard:**

```
WICK WATCH - BTC
Updated: Every 200ms

Current Conditions:
Bid/Ask Δ: -189 BTC (⚠️ Strong sell pressure)
Depth 0.5% asks: 23 BTC (⚠️ Thin)
Recent volume: 450 BTC (1 hour)

Wick Probability: HIGH (8/10)
Expected wick: 1.5% - 3.0% down

Action Plan:
1. Set limit buys at:
   - $107,500 (25% of position)
   - $106,800 (50% of position)
   - $106,000 (25% of position)

2. Risk limits:
   - Max position: $2M
   - Stop loss: $105,000 (-3%)
   - Take profit: $108,500 (+2%)
```

**Alert triggers:**
- Bid/ask delta < -150 BTC
- Depth 0.5% asks < 30 BTC
- Volume spike > 300 BTC in 15 minutes

---

### Use Case 3: Beta-Neutral Rebalancing

**Scenario:** Market moved, need to rebalance $22M book

**Current positions:**
- Long: $12M BTC (beta: 1.0)
- Short: $10M DOGE (beta: 1.8)
- Current beta: -0.18 (out of neutral range)

**Target:** Beta = 0.0 (market neutral)

**Rebalancing workflow:**

1. **Calculate required trades**
   - Need to add $2M short exposure
   - Options: Short more DOGE or add BTC short

2. **Check orderbook depth**
   ```
   BTC Short (sell):
   - Binance bids at 2%: $5.2M ✓

   DOGE Short (sell):
   - Binance asks at 2%: $8.3M ✓
   ```

3. **Choose execution path**
   - Route 1: Short $2M BTC → Lower slippage, better depth
   - Route 2: Short $2M more DOGE → Higher slippage
   - Decision: Route 1 (BTC short)

4. **Execute rebalance**
   - Sell 18.18 BTC across Binance/Bybit
   - TWAP over 30 minutes
   - Monitor funding rates for ongoing cost

---

## Quick Win Implementation Plan

### Week 1: Funding Rates
**Effort:** 2-3 hours
**Value:** High (core to strategy)

**Tasks:**
- [ ] Add funding rate API calls for Binance, Bybit, OKX futures
- [ ] Display funding rate in stats table
- [ ] Color code: Green (negative), Red (positive)
- [ ] Add "next funding" countdown timer
- [ ] Deploy to production

**API Endpoints:**
```go
// Binance Futures
GET /fapi/v1/premiumIndex?symbol=BTCUSDT

// Bybit
GET /v5/market/tickers?category=linear&symbol=BTCUSDT

// OKX
GET /api/v5/public/funding-rate?instId=BTC-USDT-SWAP
```

---

### Week 2: Slippage Calculator
**Effort:** 3-4 hours
**Value:** High (execution optimization)

**Tasks:**
- [ ] Add position size input field to UI
- [ ] Implement orderbook walk-through algorithm
- [ ] Calculate weighted average fill price
- [ ] Display per-exchange breakdown
- [ ] Show aggregate execution stats
- [ ] Deploy to production

**Algorithm:**
```go
func CalculateSlippage(orderbook []PriceLevel, size float64, side string) {
    remaining := size
    totalCost := 0.0

    for _, level := range orderbook {
        if remaining <= 0 {
            break
        }

        fillQty := min(remaining, level.Quantity)
        totalCost += fillQty * level.Price
        remaining -= fillQty
    }

    avgPrice := totalCost / size
    slippage := (avgPrice - midPrice) / midPrice

    return avgPrice, slippage
}
```

---

### Week 3: Alert System
**Effort:** 2-3 hours
**Value:** Medium (automation)

**Tasks:**
- [ ] Set up Telegram bot or Discord webhook
- [ ] Define alert thresholds (configurable)
- [ ] Implement alert trigger logic
- [ ] Add alert history/log
- [ ] Test with mock conditions
- [ ] Deploy to production

**Alert Types:**
1. Low liquidity (< threshold)
2. High bid/ask delta (wick opportunity)
3. Extreme funding rates (> ±0.03%)
4. Spread widening (> 0.5%)

---

## Expected ROI Analysis

### Execution Cost Savings
**Current cost** (estimated):
- Average slippage: 0.15% on $22M monthly volume
- Monthly cost: $33,000

**With slippage calculator:**
- Optimized slippage: 0.08% (47% reduction)
- Monthly cost: $17,600
- **Monthly savings: $15,400**
- **Annual savings: $184,800**

---

### Funding Rate Optimization
**Current approach** (manual monitoring):
- Miss ~30% of optimal funding opportunities
- Average funding differential: 0.05%/day missed

**With funding rate scanner:**
- Catch 90% of opportunities
- Additional funding revenue: 0.03%/day on $11M (half of book in perps)
- **Daily additional revenue: $3,300**
- **Annual additional revenue: $1,204,500**

---

### Wick Catching Enhancement
**Current approach:**
- Manual monitoring, limited coverage
- ~5 successful wick catches/month
- Average profit: $15k per catch

**With automated alerts:**
- Estimated 15 catches/month (3x improvement)
- **Additional monthly profit: $150,000**
- **Annual additional profit: $1,800,000**

---

## Total Expected Annual Value

| Benefit Category | Annual Value |
|-----------------|--------------|
| Execution cost savings | $184,800 |
| Funding optimization | $1,204,500 |
| Wick catching enhancement | $1,800,000 |
| **TOTAL** | **$3,189,300** |

**ROI on development:**
- Development time: ~2-3 weeks
- Development cost: ~$15,000 (if outsourced) or $0 (if internal)
- **ROI: 21,262% in year 1**

---

## Technical Implementation Notes

### Backend Changes Needed

**1. Add funding rate fetching:**
```go
// internal/exchange/funding/service.go
type FundingService struct {
    binanceClient *binance.Client
    bybitClient   *bybit.Client
    okxClient     *okx.Client
}

func (s *FundingService) GetFundingRates() map[string]FundingRate {
    // Fetch from all exchanges
    // Cache for 1 minute
    // Return map[exchange]rate
}
```

**2. Add to WebSocket message:**
```go
type StatsMessage struct {
    // ... existing fields
    FundingRate     string `json:"fundingRate,omitempty"`
    NextFundingTime int64  `json:"nextFundingTime,omitempty"`
}
```

**3. Slippage calculation:**
```go
func (ob *OrderBook) CalculateSlippage(size decimal.Decimal, side string) SlippageResult {
    // Walk orderbook
    // Calculate VWAP
    // Return result
}
```

### Frontend Changes Needed

**1. Display funding rates:**
```tsx
// components/StatsTable.tsx
<TableCell>
  <div className={fundingRate < 0 ? 'text-green-500' : 'text-red-500'}>
    {fundingRate.toFixed(4)}%
  </div>
  <div className="text-xs text-muted-foreground">
    Next: {nextFundingTime}
  </div>
</TableCell>
```

**2. Add slippage calculator:**
```tsx
// components/SlippageCalculator.tsx
<Card>
  <Input
    type="number"
    placeholder="Position size (BTC)"
    onChange={calculateSlippage}
  />
  <Table>
    {/* Show per-exchange results */}
  </Table>
</Card>
```

---

## Risk Considerations

### Technical Risks
- **API rate limits**: Funding rate APIs have limits, need caching
- **WebSocket bandwidth**: Adding more data increases message size
- **Calculation latency**: Slippage calc on large books could be slow

**Mitigations:**
- Cache funding rates for 60 seconds (they update every 8 hours anyway)
- Only send funding rates every 10 seconds, not every 200ms
- Pre-calculate slippage for common sizes (10, 50, 100, 500 BTC)

### Strategy Risks
- **Funding rate volatility**: Rates can flip quickly during volatility
- **Liquidity drying up**: Wick setups can fail if whales enter
- **Exchange basis risk**: Cross-exchange arb has counterparty risk

**Mitigations:**
- Alert on rapid funding rate changes (> 0.01% in 5 minutes)
- Set max position sizes based on 50% of available depth
- Monitor exchange spreads, pause if > 0.5%

---

## Next Steps

**Immediate (This Week):**
1. ✅ Document strategy integration approach (this file)
2. Implement funding rate integration
3. Test on production with real data
4. Validate funding rate accuracy vs exchange UI

**Short-term (Next 2 Weeks):**
1. Build slippage calculator
2. Set up alert infrastructure
3. Create monitoring dashboard for alerts
4. Backtest wick catching algorithm on historical data

**Medium-term (Next Month):**
1. Build historical depth database
2. Implement TWAP simulator
3. Create funding arbitrage scanner
4. Develop mobile-responsive alerts (SMS/push)

**Long-term (Next Quarter):**
1. Machine learning for wick prediction
2. Automated execution integration (via APIs)
3. Multi-asset support (ETH, SOL, etc.)
4. Portfolio-level risk analytics

---

## Conclusion

The orderbook aggregator is already valuable for TWAP execution optimization and market impact analysis. By adding **funding rates**, **slippage calculation**, and **alerts**, it becomes a complete execution and opportunity identification platform for the fund's strategy.

**Expected annual value: $3.2M+ with 2-3 weeks of development.**

The three priority features (funding, slippage, alerts) directly address the fund's core needs:
- ✅ Finding optimal funding rate positions (long negative, short positive)
- ✅ Minimizing execution cost on $22M positions
- ✅ Automating wick catching opportunity identification

**Recommended action: Start with funding rate integration this week.**
