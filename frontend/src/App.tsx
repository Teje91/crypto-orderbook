import { useMemo, useState } from 'react';
import { useLocalStorage } from '@uidotdev/usehooks';
import { useWebSocket } from './hooks/useWebSocket';
import { useTheme } from './hooks/useTheme';
import { useChartData } from './hooks/useChartData';
import { useAggregatedOrderbook } from './hooks/useAggregatedOrderbook';
import { StatsTable } from './components/StatsTable';
import { OrderbookCard } from './components/OrderbookCard';
import { LiquidityChart } from './components/LiquidityChart';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from './components/ui/select';
import { Button } from './components/ui/button';
import { Tooltip, TooltipContent, TooltipTrigger } from './components/ui/tooltip';
import { ToggleGroup, ToggleGroupItem } from './components/ui/toggle-group';
import { Moon, Sun, Layers } from 'lucide-react';
import { TICK_LEVELS, CHART_CONFIG } from './constants';
import { filterExchangesByMarket, sortExchangesByGroup } from './utils/calculations';
import type { MarketFilter } from './types';

function App() {
  const { isDark, toggleTheme } = useTheme();
  const [marketFilter, setMarketFilter] = useLocalStorage<MarketFilter>('marketFilter', 'all');
  const [showAggregate, setShowAggregate] = useState(false);

  // Use production WebSocket URL in production, localhost in development
  const wsUrl = import.meta.env.PROD
    ? 'wss://crypto-orderbook-production.up.railway.app/ws'
    : 'ws://localhost:8086/ws';

  const { orderbooks, stats, isConnected, setTickLevel } = useWebSocket(wsUrl);
  const { chartData05Pct, chartData2Pct, chartData10Pct, chartDataTotal } = useChartData(stats, marketFilter);

  // Filter and sort orderbooks based on market filter
  const filteredOrderbooks = useMemo(() => {
    const filtered = Object.entries(orderbooks).filter(([exchange]) =>
      filterExchangesByMarket(exchange, marketFilter)
    );
    return sortExchangesByGroup(filtered);
  }, [orderbooks, marketFilter]);

  // Get aggregated orderbook for filtered exchanges
  const filteredOrderbooksData = useMemo(() => {
    return Object.fromEntries(filteredOrderbooks);
  }, [filteredOrderbooks]);

  const aggregated = useAggregatedOrderbook(filteredOrderbooksData);

  return (
    <div className="min-h-screen bg-background text-foreground">
      <div className="max-w-[1400px] mx-auto px-4 md:px-6 lg:px-8 py-6">
        <div className="mb-6 flex items-center justify-between gap-3">
          <div className="flex items-center gap-3">
            <h1 className="text-2xl font-semibold tracking-tight">Crypto Orderbook</h1>
            <div className="hidden sm:inline-flex items-center gap-2 rounded-full border border-border bg-muted/30 px-2 py-1">
              <span
                className={`size-2 rounded-full ${isConnected ? 'bg-green-500' : 'bg-destructive'
                  }`}
              />
              <span className="text-xs text-muted-foreground">
                {isConnected ? 'Connected' : 'Disconnected'}
              </span>
            </div>
          </div>

          <div className="flex items-center gap-2">
            <ToggleGroup
              type="single"
              value={marketFilter}
              onValueChange={(value) => value && setMarketFilter(value as MarketFilter)}
              variant="outline"
            >
              <ToggleGroupItem value="all" aria-label="Show all markets" className="px-2.5">
                All
              </ToggleGroupItem>
              <ToggleGroupItem value="spot" aria-label="Show spot markets" className="px-2.5">
                Spot
              </ToggleGroupItem>
              <ToggleGroupItem value="perps" aria-label="Show perpetual futures" className="px-2.5">
                Perps
              </ToggleGroupItem>
            </ToggleGroup>

            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="outline"
                  size="icon"
                  aria-label="Toggle theme"
                  onClick={toggleTheme}
                >
                  {isDark ? <Sun className="size-4" /> : <Moon className="size-4" />}
                </Button>
              </TooltipTrigger>
              <TooltipContent>Toggle theme</TooltipContent>
            </Tooltip>
          </div>
        </div>

        <div className="space-y-6">
          <section>
            <div className="mb-3 flex items-center justify-between">
              <h2 className="text-sm font-semibold tracking-wide text-muted-foreground uppercase">
                Exchange Statistics
              </h2>
            </div>
            <StatsTable stats={stats} filter={marketFilter} />
          </section>

          <section>
            <div className="mb-3 flex items-center justify-between">
              <h2 className="text-sm font-semibold tracking-wide text-muted-foreground uppercase">
                Order Books
              </h2>
              <div className="flex items-center gap-2">
                <span className="hidden md:block text-xs text-muted-foreground">Tick</span>
                <Select defaultValue="1" onValueChange={(value) => setTickLevel(parseFloat(value))}>
                  <SelectTrigger size="sm" className="w-[84px]">
                    <SelectValue placeholder="Tick" />
                  </SelectTrigger>
                  <SelectContent>
                    {TICK_LEVELS.map((tick) => (
                      <SelectItem key={tick.value} value={tick.value}>
                        {tick.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      variant={showAggregate ? 'default' : 'outline'}
                      size="sm"
                      onClick={() => setShowAggregate(!showAggregate)}
                      className="px-2 gap-1.5"
                    >
                      <Layers className="size-3.5" />
                      <span className="text-xs">Aggregate</span>
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>
                    {showAggregate ? 'Show individual exchanges' : 'Show aggregated orderbook'}
                  </TooltipContent>
                </Tooltip>
              </div>
            </div>
            <div className="flex gap-4 overflow-x-auto pb-2">
              {showAggregate ? (
                <OrderbookCard
                  exchange="Aggregated"
                  bids={aggregated.bids}
                  asks={aggregated.asks}
                  stats={aggregated.stats}
                />
              ) : (
                filteredOrderbooks.map(([exchange, data]) => (
                  <OrderbookCard
                    key={exchange}
                    exchange={exchange}
                    bids={data.bids}
                    asks={data.asks}
                    stats={stats[exchange]}
                  />
                ))
              )}
            </div>
          </section>

          {chartData05Pct.length > 0 && (
            <section>
              <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
                <LiquidityChart
                  title="Liquidity at 0.5% Depth"
                  data={chartData05Pct}
                  config={CHART_CONFIG}
                />
                <LiquidityChart
                  title="Liquidity at 2% Depth"
                  data={chartData2Pct}
                  config={CHART_CONFIG}
                />
                <LiquidityChart
                  title="Liquidity at 10% Depth"
                  data={chartData10Pct}
                  config={CHART_CONFIG}
                />
                <LiquidityChart
                  title="Total Liquidity"
                  data={chartDataTotal}
                  config={CHART_CONFIG}
                />
              </div>
            </section>
          )}
        </div>
      </div>
    </div>
  );
}

export default App;
