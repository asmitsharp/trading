# 🚀 How to Run the Crypto Platform - Step by Step

## Quick Start (3 Steps)

```bash
# Step 1: Start databases
docker-compose up -d postgres clickhouse redis

# Step 2: Wait for services (10 seconds)
sleep 10

# Step 3: Run the simplified REST API server
go run cmd/main_rest.go
```

That's it! The API will be available at http://localhost:8080

## Complete Setup with Data

### 1. Start Docker Services

```bash
# Start PostgreSQL, ClickHouse, and Redis
docker-compose up -d postgres clickhouse redis

# Check if services are running
docker-compose ps
```

### 2. Setup Databases (First Time Only)

```bash
# Create ClickHouse database
docker exec crypto_clickhouse clickhouse-client --query "CREATE DATABASE IF NOT EXISTS crypto_platform"

# Run PostgreSQL migrations
docker exec -i crypto_postgres psql -U crypto_user -d crypto_platform < migrations/001_create_tokens_table.sql
docker exec -i crypto_postgres psql -U crypto_user -d crypto_platform < migrations/002_create_exchanges_table.sql
docker exec -i crypto_postgres psql -U crypto_user -d crypto_platform < migrations/003_create_exchange_symbols_table.sql
docker exec -i crypto_postgres psql -U crypto_user -d crypto_platform < migrations/004_create_trading_pairs_table.sql

# Run ClickHouse migrations
docker exec -i crypto_clickhouse clickhouse-client --database crypto_platform --multiquery < migrations/clickhouse_001_vwap_prices.sql
docker exec -i crypto_clickhouse clickhouse-client --database crypto_platform --multiquery < migrations/clickhouse_002_candlesticks.sql
docker exec -i crypto_clickhouse clickhouse-client --database crypto_platform --multiquery < migrations/clickhouse_003_exchange_health.sql

# Seed initial data (exchanges and tokens)
docker exec -i crypto_postgres psql -U crypto_user -d crypto_platform < migrations/seed_tokens.sql
docker exec -i crypto_postgres psql -U crypto_user -d crypto_platform < migrations/seed_exchanges.sql
```

### 3. Run the Application

```bash
# Option 1: Run directly with Go
go run cmd/main_rest.go

# Option 2: Build and run
go build -o crypto-platform cmd/main_rest.go
./crypto-platform

# Option 3: Use the run script
./run.sh
```

## Testing the API

### Check Health
```bash
curl http://localhost:8080/health
```

Expected response:
```json
{
  "status": "healthy",
  "services": {
    "postgres": true,
    "clickhouse": true
  },
  "timestamp": 1234567890
}
```

### List Exchanges
```bash
curl http://localhost:8080/api/v1/exchanges
```

### List Tokens
```bash
curl http://localhost:8080/api/v1/tokens
```

### Get All Tickers
```bash
curl http://localhost:8080/api/v1/tickers
```

## Environment Variables

Create a `.env` file or export these variables:

```bash
# PostgreSQL
export POSTGRES_HOST=localhost
export POSTGRES_PORT=5432
export POSTGRES_DATABASE=crypto_platform
export POSTGRES_USERNAME=crypto_user
export POSTGRES_PASSWORD=crypto_password

# ClickHouse
export CLICKHOUSE_HOST=localhost
export CLICKHOUSE_PORT=9000
export CLICKHOUSE_DATABASE=crypto_platform
export CLICKHOUSE_USERNAME=default
export CLICKHOUSE_PASSWORD=""

# Redis (optional)
export REDIS_URL=redis://localhost:6379/0

# Server
export SERVER_PORT=:8080
export SERVICE_MODE=all  # Options: all, api, poller
export POLL_INTERVAL=15s
```

## Service Modes

The application can run in different modes:

- **all** (default): Runs both API and polling service
- **api**: Only runs the REST API server
- **poller**: Only runs the exchange polling service

```bash
# Run only API
SERVICE_MODE=api go run cmd/main_rest.go

# Run only poller
SERVICE_MODE=poller go run cmd/main_rest.go

# Run both (default)
go run cmd/main_rest.go
```

## Troubleshooting

### Problem: "database crypto_platform does not exist"

```bash
# Create the database manually
docker exec crypto_postgres createdb -U crypto_user crypto_platform
```

### Problem: Port 8080 already in use

```bash
# Use a different port
SERVER_PORT=:8081 go run cmd/main_rest.go
```

### Problem: Cannot connect to PostgreSQL

```bash
# Check if PostgreSQL is running
docker-compose ps

# Check PostgreSQL logs
docker-compose logs postgres

# Test connection
docker exec crypto_postgres pg_isready -U crypto_user
```

### Problem: Cannot connect to ClickHouse

```bash
# Check if ClickHouse is running
curl http://localhost:8123/ping

# Check ClickHouse logs
docker-compose logs clickhouse
```

## Stopping Everything

```bash
# Stop the application
# Press Ctrl+C in the terminal where it's running

# Stop Docker services
docker-compose down

# Stop and remove all data
docker-compose down -v
```

## What's Working

✅ PostgreSQL connection and queries
✅ ClickHouse connection
✅ Health check endpoint
✅ Exchange listing
✅ Token listing
✅ Basic ticker endpoints
✅ Polling service structure
✅ VWAP calculation logic

## What's In Progress

🚧 Live price polling from exchanges
🚧 VWAP price storage in ClickHouse
🚧 Candlestick generation
🚧 WebSocket real-time updates
🚧 Complete API endpoints

## API Endpoints Available

| Endpoint | Method | Description | Status |
|----------|--------|-------------|--------|
| `/health` | GET | Health check | ✅ Working |
| `/api/v1/exchanges` | GET | List all exchanges | ✅ Working |
| `/api/v1/exchanges/:id` | GET | Get exchange details | ✅ Working |
| `/api/v1/tokens` | GET | List all tokens | ✅ Working |
| `/api/v1/tokens/:id` | GET | Get token details | ✅ Working |
| `/api/v1/tickers` | GET | Get all tickers | ✅ Working |
| `/api/v1/tickers/:symbol` | GET | Get specific ticker | 🚧 In Progress |
| `/api/v1/vwap/:symbol` | GET | Get VWAP price | 🚧 In Progress |

## Next Steps

1. The polling service will start collecting real-time data from exchanges
2. VWAP prices will be calculated and stored
3. Candlesticks will be generated from VWAP data
4. More API endpoints will become functional

The platform is designed to poll 30+ exchanges every 10-15 seconds and calculate accurate cross-exchange prices using Volume Weighted Average Price (VWAP).

---

**For any issues, check the logs:**
```bash
# Application logs (if running in terminal)
# Will be visible in the terminal

# Docker logs
docker-compose logs -f
```