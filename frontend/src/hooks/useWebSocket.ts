import { useEffect, useRef, useState } from 'react';
import type {
  WebSocketMessage,
  OrderbookData,
  StatsData,
} from '@/types';

export function useWebSocket(url: string) {
  const [orderbooks, setOrderbooks] = useState<OrderbookData>({});
  const [stats, setStats] = useState<StatsData>({});
  const [isConnected, setIsConnected] = useState(false);
  const [currentSymbol, setCurrentSymbol] = useState('BTCUSDT');
  const [isSwitchingSymbol, setIsSwitchingSymbol] = useState(false);
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimeoutRef = useRef<number | undefined>(undefined);
  const connectionTimeoutRef = useRef<number | undefined>(undefined);
  const reconnectAttempts = useRef(0);

  useEffect(() => {
    async function connect() {
      // Call health check endpoint first to wake up Railway's proxy
      try {
        const healthUrl = url.replace('wss://', 'https://').replace('ws://', 'http://').replace('/ws', '/health');
        await fetch(healthUrl, { mode: 'cors' });
        console.log('Health check successful');
      } catch (error) {
        console.warn('Health check failed, attempting connection anyway:', error);
      }

      const ws = new WebSocket(url);
      wsRef.current = ws;

      ws.onopen = () => {
        setIsConnected(true);
        console.log('WebSocket connected');

        // Set a timeout to detect stalled connections
        // If no data is received within 30 seconds, the connection is likely stalled at Railway's proxy
        connectionTimeoutRef.current = window.setTimeout(() => {
          console.warn('WebSocket stalled (no data received), reconnecting...');
          ws.close();
        }, 30000);
      };

      ws.onmessage = (event) => {
        // Clear the stalled connection timeout - we're receiving data!
        if (connectionTimeoutRef.current) {
          clearTimeout(connectionTimeoutRef.current);
          connectionTimeoutRef.current = undefined;
        }

        // Reset reconnect attempts on successful data receipt
        reconnectAttempts.current = 0;

        const message: WebSocketMessage = JSON.parse(event.data);

        if (message.type === 'orderbook') {
          setOrderbooks((prev) => ({
            ...prev,
            [message.exchange]: {
              bids: message.bids,
              asks: message.asks,
            },
          }));
        } else if (message.type === 'stats') {
          setStats((prev) => ({
            ...prev,
            [message.exchange]: {
              bestBid: message.bestBid,
              bestAsk: message.bestAsk,
              midPrice: message.midPrice,
              spread: message.spread,
              bidLiquidity05Pct: message.bidLiquidity05Pct,
              askLiquidity05Pct: message.askLiquidity05Pct,
              deltaLiquidity05Pct: message.deltaLiquidity05Pct,
              bidLiquidity2Pct: message.bidLiquidity2Pct,
              askLiquidity2Pct: message.askLiquidity2Pct,
              deltaLiquidity2Pct: message.deltaLiquidity2Pct,
              bidLiquidity10Pct: message.bidLiquidity10Pct,
              askLiquidity10Pct: message.askLiquidity10Pct,
              deltaLiquidity10Pct: message.deltaLiquidity10Pct,
              totalBidsQty: message.totalBidsQty,
              totalAsksQty: message.totalAsksQty,
              totalDelta: message.totalDelta,
            },
          }));
        }
      };

      ws.onerror = (error) => {
        console.error('WebSocket error:', error);
      };

      ws.onclose = () => {
        setIsConnected(false);

        // Exponential backoff: 3s, 6s, 12s, 24s, 30s (max)
        reconnectAttempts.current++;
        const delay = Math.min(3000 * Math.pow(2, reconnectAttempts.current - 1), 30000);

        console.log(`WebSocket disconnected, reconnecting in ${delay/1000}s... (attempt ${reconnectAttempts.current})`);
        reconnectTimeoutRef.current = window.setTimeout(connect, delay);
      };
    }

    connect();

    // Reconnect when tab becomes visible again (e.g., after unlocking computer)
    const handleVisibilityChange = () => {
      if (document.visibilityState === 'visible') {
        console.log('Tab visible, forcing WebSocket reconnection...');

        // Clear any pending reconnect timeout
        if (reconnectTimeoutRef.current) {
          clearTimeout(reconnectTimeoutRef.current);
        }

        // Force close existing connection (even if it thinks it's OPEN)
        // After sleep/wake, connections are often in a "zombie" state
        if (wsRef.current) {
          wsRef.current.close();
        }

        // Force immediate reconnection with fresh connection
        setTimeout(connect, 100);
      }
    };

    document.addEventListener('visibilitychange', handleVisibilityChange);

    return () => {
      document.removeEventListener('visibilitychange', handleVisibilityChange);
      if (reconnectTimeoutRef.current) {
        clearTimeout(reconnectTimeoutRef.current);
      }
      if (connectionTimeoutRef.current) {
        clearTimeout(connectionTimeoutRef.current);
      }
      if (wsRef.current) {
        wsRef.current.close();
      }
    };
  }, [url]);

  const setTickLevel = (tick: number) => {
    if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify({ type: 'set_tick', tick }));
    }
  };

  const setSymbol = (symbol: string) => {
    if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN) {
      setIsSwitchingSymbol(true);
      setOrderbooks({});
      setStats({});
      setCurrentSymbol(symbol);
      wsRef.current.send(JSON.stringify({ type: 'change_symbol', symbol }));

      setTimeout(() => {
        setIsSwitchingSymbol(false);
      }, 3000);
    }
  };

  return { orderbooks, stats, isConnected, currentSymbol, isSwitchingSymbol, setTickLevel, setSymbol };
}
