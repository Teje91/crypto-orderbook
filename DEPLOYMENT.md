# Deployment Guide - Make Your App Live on the Internet!

## The Challenge: You Have TWO Parts to Deploy

```
┌──────────────────────────────────────────────────────────┐
│                                                          │
│  ❌ PROBLEM: Vercel is for FRONTEND ONLY                │
│                                                          │
│  Your app has:                                           │
│  1. Frontend (React) ✅ Can deploy to Vercel            │
│  2. Backend (Go) ❌ Vercel doesn't support Go backend   │
│                                                          │
└──────────────────────────────────────────────────────────┘
```

---

## Solution: Hybrid Deployment

Deploy each part to its best platform:

- **Frontend (React)** → Vercel (FREE, perfect for React)
- **Backend (Go)** → Fly.io or Railway (FREE tier, perfect for Go)

---

## Option 1: Recommended Setup (Frontend: Vercel + Backend: Railway)

### Part A: Deploy Backend to Railway (FREE)

Railway gives you:
- FREE $5/month credit (enough for hobby projects)
- Automatic HTTPS
- Public URL for your backend
- Easy deployments

#### Step 1: Prepare Backend for Deployment

Create `railway.toml` in your project root:

```toml
[build]
builder = "nixpacks"

[deploy]
startCommand = "go run ./cmd/main.go"
restartPolicyType = "on-failure"
```

#### Step 2: Create `Procfile` (tells Railway how to run your app)

```
web: go run ./cmd/main.go
```

#### Step 3: Sign Up & Deploy

1. Go to https://railway.app/
2. Sign in with GitHub
3. Click "New Project" → "Deploy from GitHub repo"
4. Select your `crypto-orderbook` repository
5. Railway will:
   - Detect it's a Go project
   - Build it automatically
   - Give you a public URL like: `https://your-app.up.railway.app`

#### Step 4: Get Your Backend URL

After deployment, Railway shows you a URL like:
```
https://crypto-orderbook-production.up.railway.app
```

**Save this!** You'll need it for the frontend.

---

### Part B: Deploy Frontend to Vercel

#### Step 1: Update Frontend to Use Production Backend

Edit `frontend/src/hooks/useWebSocket.ts`:

```typescript
// OLD (local development)
const WS_URL = 'ws://localhost:8086/ws'

// NEW (production) - Use Railway URL
const WS_URL = import.meta.env.PROD
  ? 'wss://crypto-orderbook-production.up.railway.app/ws'  // 👈 Your Railway URL here!
  : 'ws://localhost:8086/ws'
```

**Note:** Change `ws://` to `wss://` (secure WebSocket) for production!

#### Step 2: Deploy to Vercel

1. Go to https://vercel.com
2. Sign in with GitHub
3. Click "Add New Project"
4. Select your `crypto-orderbook` repository
5. Configure:
   - **Framework Preset:** Vite
   - **Root Directory:** `frontend`
   - **Build Command:** `npm run build`
   - **Output Directory:** `dist`
6. Click "Deploy"

#### Step 3: Your App is LIVE! 🎉

Vercel gives you a URL like:
```
https://crypto-orderbook.vercel.app
```

Open it in your browser - you should see your app running live!

---

## Option 2: All-in-One (Both on Fly.io)

Fly.io can host BOTH frontend and backend together.

### Step 1: Install Fly CLI

```bash
# macOS
brew install flyctl

# Login
flyctl auth login
```

### Step 2: Create `fly.toml` Configuration

```toml
app = "crypto-orderbook"

[build]
  dockerfile = "Dockerfile"

[[services]]
  http_checks = []
  internal_port = 8086
  processes = ["app"]
  protocol = "tcp"

  [[services.ports]]
    port = 80
    handlers = ["http"]

  [[services.ports]]
    port = 443
    handlers = ["tls", "http"]
```

### Step 3: Create `Dockerfile`

```dockerfile
# Build backend
FROM golang:1.22-alpine AS backend-build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o server ./cmd/main.go

# Build frontend
FROM node:20-alpine AS frontend-build
WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm install
COPY frontend/ ./
RUN npm run build

# Final image
FROM alpine:latest
WORKDIR /app
COPY --from=backend-build /app/server .
COPY --from=frontend-build /app/frontend/dist ./static
EXPOSE 8086
CMD ["./server"]
```

### Step 4: Deploy

```bash
fly launch
fly deploy
```

Your app will be live at: `https://crypto-orderbook.fly.dev`

---

## Option 3: Budget-Friendly (Both on DigitalOcean Droplet)

Cost: $6/month for basic droplet

### Step 1: Create Droplet
1. Go to DigitalOcean
2. Create new Droplet (Ubuntu)
3. SSH into it

### Step 2: Install Dependencies

```bash
# Install Go
wget https://go.dev/dl/go1.22.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.22.0.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin

# Install Node.js
curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash -
sudo apt-get install -y nodejs
```

### Step 3: Clone and Build

```bash
git clone https://github.com/yourusername/crypto-orderbook
cd crypto-orderbook

# Build backend
go build -o server ./cmd/main.go

# Build frontend
cd frontend
npm install
npm run build
```

### Step 4: Use Nginx as Reverse Proxy

Install Nginx:
```bash
sudo apt install nginx
```

Configure Nginx (`/etc/nginx/sites-available/crypto-orderbook`):

```nginx
server {
    listen 80;
    server_name your-domain.com;

    # Serve frontend
    location / {
        root /home/ubuntu/crypto-orderbook/frontend/dist;
        try_files $uri $uri/ /index.html;
    }

    # Proxy WebSocket to backend
    location /ws {
        proxy_pass http://localhost:8086;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
    }
}
```

### Step 5: Run Backend with PM2

```bash
npm install -g pm2
pm2 start ./server --name crypto-orderbook
pm2 startup
pm2 save
```

---

## Comparison Table

| Platform | Frontend | Backend | Cost | Ease | Best For |
|----------|----------|---------|------|------|----------|
| **Vercel + Railway** | ✅ | ✅ | FREE | 🟢 Easy | Beginners |
| **Fly.io** | ✅ | ✅ | FREE* | 🟡 Medium | All-in-one |
| **DigitalOcean** | ✅ | ✅ | $6/mo | 🔴 Hard | Learning Linux |
| **Heroku** | ✅ | ✅ | $7/mo | 🟢 Easy | Outdated |

*Fly.io FREE tier: 3 VMs, 3GB storage

---

## Important: Environment Variables

### Backend Environment Variables

If you add API keys or secrets later, set them in Railway/Fly.io dashboard:

```bash
# Example
BINANCE_API_KEY=your-key-here
BINANCE_API_SECRET=your-secret-here
```

### Frontend Environment Variables

In Vercel, add:

```bash
VITE_WS_URL=wss://your-backend.up.railway.app/ws
```

Then use in code:
```typescript
const WS_URL = import.meta.env.VITE_WS_URL || 'ws://localhost:8086/ws'
```

---

## Troubleshooting

### Problem: Frontend can't connect to backend

**Solution:** Check CORS settings in backend

Add to `internal/websocket/server.go`:

```go
func (s *Server) Start() error {
    r := mux.NewRouter()

    // Add CORS middleware
    r.Use(func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            w.Header().Set("Access-Control-Allow-Origin", "*")
            w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
            next.ServeHTTP(w, r)
        })
    })

    r.HandleFunc("/ws", s.handleWebSocket)
    // ...
}
```

### Problem: WebSocket connection fails

**Error:** `WebSocket connection failed: Error during WebSocket handshake`

**Solutions:**
1. Make sure backend URL uses `wss://` (not `ws://`)
2. Check firewall allows WebSocket connections
3. Verify backend is actually running

### Problem: "This site can't be reached"

**Solutions:**
1. Wait 2-3 minutes after deployment (servers need time to start)
2. Check deployment logs in Railway/Fly.io dashboard
3. Verify backend is running: Visit `https://your-backend.com/health` (if you add a health endpoint)

---

## Next Steps After Deployment

### 1. Add Custom Domain (Optional)

**Vercel:**
1. Buy domain from Namecheap/GoDaddy
2. In Vercel dashboard → Settings → Domains
3. Add your domain
4. Update DNS records (Vercel provides instructions)

**Railway:**
- Railway provides free `*.up.railway.app` subdomain
- Custom domains need Pro plan ($20/mo)

### 2. Set Up Monitoring

Use free services:
- **UptimeRobot:** Check if your site is online
- **LogTail:** View application logs
- **Sentry:** Track errors

### 3. Add Analytics

```bash
npm install @vercel/analytics
```

```tsx
// frontend/src/main.tsx
import { Analytics } from '@vercel/analytics/react'

<App />
<Analytics />
```

### 4. Optimize Performance

**Backend:**
- Add Redis cache for orderbook data
- Use connection pooling
- Add rate limiting

**Frontend:**
- Enable gzip compression (Vercel does this automatically)
- Code splitting for faster loading
- Lazy load components

---

## Cost Breakdown (Monthly)

### Option 1: FREE Setup
- Vercel: FREE
- Railway: FREE ($5 credit)
- **Total: $0** ✨

### Option 2: Hobbyist Setup
- Vercel: FREE
- Railway Pro: $20
- Custom domain: $12/year
- **Total: ~$21/month**

### Option 3: Production Setup
- Vercel Pro: $20
- Railway Pro: $20
- DigitalOcean Spaces (storage): $5
- **Total: ~$45/month**

---

## Quick Start: Deploy in 5 Minutes

```bash
# 1. Push to GitHub
git add .
git commit -m "Ready for deployment"
git push origin main

# 2. Deploy Backend (Railway)
# → Go to railway.app, connect GitHub, select repo

# 3. Deploy Frontend (Vercel)
# → Go to vercel.com, connect GitHub, select repo
# → Set root directory to "frontend"

# 4. Update frontend WS_URL with Railway URL

# Done! 🚀
```

---

## Resources

- **Railway Docs:** https://docs.railway.app/
- **Vercel Docs:** https://vercel.com/docs
- **Fly.io Docs:** https://fly.io/docs/
- **Go Deployment Guide:** https://go.dev/doc/tutorial/web-service-gin#deploy

---

## FAQ

**Q: Can I use Vercel for the backend too?**
A: No, Vercel serverless functions have 10-second timeout. Your WebSocket needs to stay open indefinitely.

**Q: Why not use AWS?**
A: AWS is powerful but complex (steep learning curve). Railway/Fly.io are much simpler for beginners.

**Q: How much traffic can the free tier handle?**
A: Railway FREE tier: ~100 concurrent WebSocket connections. Enough for testing/hobby projects.

**Q: Can I use a database?**
A: Your app doesn't need one yet (data is in memory). If you want persistence, add PostgreSQL:
- Railway: FREE 512MB PostgreSQL
- Supabase: FREE 500MB PostgreSQL

**Q: How do I update my deployed app?**
A: Just push to GitHub! Both Vercel and Railway auto-deploy on `git push`.
