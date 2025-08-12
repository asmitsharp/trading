package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/shopspring/decimal"
)

type VWAPComparison struct {
	Symbol       string
	BaseToken    string
	QuoteToken   string
	OurVWAP      decimal.Decimal
	OurVolume    decimal.Decimal
	OurExchanges int
	CMCPrice     decimal.Decimal
	CMCVolume    decimal.Decimal
	PriceDiff    decimal.Decimal
	PriceDiffPct decimal.Decimal
}

func main() {
	// Load environment
	if err := godotenv.Load(); err != nil {
		log.Printf("No .env file found: %v", err)
	}

	// Connect to databases
	postgresDB := connectPostgres()
	defer postgresDB.Close()
	
	clickhouseDB := connectClickHouse()
	defer clickhouseDB.Close()

	// Get top trading pairs from our VWAP data
	pairs := getTopVWAPPairs(clickhouseDB, postgresDB)
	
	// Compare with CoinMarketCap
	comparisons := compareWithCMC(pairs, postgresDB)
	
	// Print results
	printResults(comparisons)
}

func connectPostgres() *sql.DB {
	pgDSN := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		getEnv("POSTGRES_HOST", "localhost"),
		getEnv("POSTGRES_PORT", "5432"),
		getEnv("POSTGRES_USER", "crypto_user"),
		getEnv("POSTGRES_PASSWORD", "crypto_password"),
		getEnv("POSTGRES_DB", "crypto_platform"))
	
	db, err := sql.Open("postgres", pgDSN)
	if err != nil {
		log.Fatalf("Failed to connect to PostgreSQL: %v", err)
	}
	
	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to ping PostgreSQL: %v", err)
	}
	
	return db
}

func connectClickHouse() clickhouse.Conn {
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{fmt.Sprintf("%s:%s",
			getEnv("CLICKHOUSE_HOST", "localhost"),
			getEnv("CLICKHOUSE_PORT", "9001"))},
		Auth: clickhouse.Auth{
			Database: getEnv("CLICKHOUSE_DATABASE", "crypto_platform"),
			Username: getEnv("CLICKHOUSE_USER", "default"),
			Password: getEnv("CLICKHOUSE_PASSWORD", "clickhouse123"),
		},
	})
	if err != nil {
		log.Fatalf("Failed to connect to ClickHouse: %v", err)
	}
	
	if err := conn.Ping(context.Background()); err != nil {
		log.Fatalf("Failed to ping ClickHouse: %v", err)
	}
	
	return conn
}

type VWAPData struct {
	BaseTokenID    uint32
	QuoteTokenID   uint32
	VWAPPrice      decimal.Decimal
	TotalVolume    decimal.Decimal
	ExchangeCount  uint8
}

func getTopVWAPPairs(ch clickhouse.Conn, pg *sql.DB) []VWAPData {
	// First get token IDs for major tokens
	var usdtID, usdcID int
	err := pg.QueryRow("SELECT id FROM tokens WHERE symbol = 'USDT'").Scan(&usdtID)
	if err != nil {
		log.Printf("Error getting USDT ID: %v", err)
	}
	err = pg.QueryRow("SELECT id FROM tokens WHERE symbol = 'USDC'").Scan(&usdcID)
	if err != nil {
		log.Printf("Error getting USDC ID: %v", err)
	}
	
	// Get IDs of major tokens from PostgreSQL
	rows, err := pg.Query(`
		SELECT id FROM tokens 
		WHERE symbol IN ('BTC', 'ETH', 'BNB', 'SOL', 'XRP', 'DOGE', 
		                 'ADA', 'AVAX', 'SHIB', 'DOT', 'LINK', 'TRX', 
		                 'MATIC', 'UNI', 'LTC', 'PEPE', 'WIF', 'BONK', 'FLOKI')
	`)
	if err != nil {
		log.Printf("Error getting major token IDs: %v", err)
		return nil
	}
	defer rows.Close()
	
	var majorTokenIDs []string
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err == nil {
			majorTokenIDs = append(majorTokenIDs, fmt.Sprintf("%d", id))
		}
	}
	
	if len(majorTokenIDs) == 0 {
		log.Printf("No major tokens found")
		return nil
	}
	
	query := fmt.Sprintf(`
		SELECT 
			base_token_id,
			quote_token_id,
			vwap_price,
			total_volume,
			exchange_count
		FROM vwap_prices
		WHERE timestamp >= now() - INTERVAL 5 MINUTE
			AND quote_token_id IN (%d, %d)
			AND exchange_count >= 3
			AND base_token_id IN (%s)
			AND vwap_price > 1
		ORDER BY total_volume DESC
		LIMIT 20
	`, usdtID, usdcID, strings.Join(majorTokenIDs, ","))
	
	chRows, err := ch.Query(context.Background(), query)
	if err != nil {
		log.Printf("Error querying VWAP data: %v", err)
		return nil
	}
	defer chRows.Close()
	
	var pairs []VWAPData
	for chRows.Next() {
		var p VWAPData
		
		err := chRows.Scan(
			&p.BaseTokenID,
			&p.QuoteTokenID,
			&p.VWAPPrice,
			&p.TotalVolume,
			&p.ExchangeCount,
		)
		if err != nil {
			log.Printf("Error scanning row: %v", err)
			continue
		}
		
		pairs = append(pairs, p)
	}
	
	fmt.Printf("Found %d VWAP pairs to compare\n", len(pairs))
	return pairs
}

func compareWithCMC(pairs []VWAPData, pg *sql.DB) []VWAPComparison {
	var comparisons []VWAPComparison
	
	// Get token symbols
	tokenMap := getTokenSymbols(pg)
	
	for _, pair := range pairs {
		baseSymbol := tokenMap[pair.BaseTokenID]
		quoteSymbol := tokenMap[pair.QuoteTokenID]
		
		if baseSymbol == "" || quoteSymbol == "" {
			log.Printf("Missing symbols - Base: %s (%d), Quote: %s (%d)", 
				baseSymbol, pair.BaseTokenID, quoteSymbol, pair.QuoteTokenID)
			continue
		}
		
		// Only compare USD pairs
		if !strings.Contains(quoteSymbol, "USD") {
			log.Printf("Skipping non-USD pair: %s/%s", baseSymbol, quoteSymbol)
			continue
		}
		
		// Fetch from CoinGecko
		cmcPrice, cmcVolume := fetchMarketData(baseSymbol)
		
		if cmcPrice.IsZero() {
			log.Printf("Failed to get market price for %s", baseSymbol)
			continue
		}
		
		comp := VWAPComparison{
			Symbol:       fmt.Sprintf("%s/%s", baseSymbol, quoteSymbol),
			BaseToken:    baseSymbol,
			QuoteToken:   quoteSymbol,
			OurVWAP:      pair.VWAPPrice,
			OurVolume:    pair.TotalVolume,
			OurExchanges: int(pair.ExchangeCount),
			CMCPrice:     cmcPrice,
			CMCVolume:    cmcVolume,
		}
		
		// Calculate difference
		comp.PriceDiff = comp.OurVWAP.Sub(comp.CMCPrice)
		if comp.CMCPrice.IsPositive() {
			comp.PriceDiffPct = comp.PriceDiff.Div(comp.CMCPrice).Mul(decimal.NewFromInt(100))
		}
		
		comparisons = append(comparisons, comp)
		
		// Rate limiting for API calls
		time.Sleep(100 * time.Millisecond)
	}
	
	return comparisons
}

func fetchMarketData(symbol string) (decimal.Decimal, decimal.Decimal) {
	geckoID := getCoinGeckoID(symbol)
	url := fmt.Sprintf("https://api.coingecko.com/api/v3/simple/price?ids=%s&vs_currencies=usd&include_24hr_vol=true", geckoID)
	
	resp, err := http.Get(url)
	if err != nil {
		log.Printf("Error fetching price for %s: %v", symbol, err)
		return decimal.Zero, decimal.Zero
	}
	defer resp.Body.Close()
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return decimal.Zero, decimal.Zero
	}
	
	var result map[string]map[string]float64
	if err := json.Unmarshal(body, &result); err != nil {
		return decimal.Zero, decimal.Zero
	}
	
	if data, ok := result[geckoID]; ok {
		price := decimal.NewFromFloat(data["usd"])
		volume := decimal.NewFromFloat(data["usd_24h_vol"])
		return price, volume
	}
	
	return decimal.Zero, decimal.Zero
}

func getCoinGeckoID(symbol string) string {
	geckoMap := map[string]string{
		"BTC": "bitcoin",
		"ETH": "ethereum",
		"BNB": "binancecoin",
		"SOL": "solana",
		"XRP": "ripple",
		"DOGE": "dogecoin",
		"ADA": "cardano",
		"AVAX": "avalanche-2",
		"SHIB": "shiba-inu",
		"DOT": "polkadot",
		"LINK": "chainlink",
		"TRX": "tron",
		"MATIC": "matic-network",
		"UNI": "uniswap",
		"LTC": "litecoin",
		"PEPE": "pepe",
		"WIF": "dogwifhat",
		"BONK": "bonk",
		"FLOKI": "floki",
	}
	
	if id, ok := geckoMap[symbol]; ok {
		return id
	}
	return strings.ToLower(symbol)
}

func getTokenSymbols(db *sql.DB) map[uint32]string {
	query := "SELECT id, symbol FROM tokens"
	rows, err := db.Query(query)
	if err != nil {
		log.Printf("Error fetching tokens: %v", err)
		return nil
	}
	defer rows.Close()
	
	tokenMap := make(map[uint32]string)
	for rows.Next() {
		var id uint32
		var symbol string
		if err := rows.Scan(&id, &symbol); err == nil {
			tokenMap[id] = symbol
		}
	}
	
	return tokenMap
}

func printResults(comparisons []VWAPComparison) {
	fmt.Println("\n=== VWAP Price Comparison with CoinGecko Market Data ===\n")
	fmt.Printf("%-12s | %-12s | %-12s | %-12s | %-8s | %-10s | %s\n",
		"Symbol", "Our VWAP", "Market Price", "Difference", "Diff %", "Exchanges", "Our Volume (M)")
	fmt.Println(strings.Repeat("-", 100))
	
	var totalDiff decimal.Decimal
	count := 0
	
	for _, comp := range comparisons {
		volumeInM := comp.OurVolume.Div(decimal.NewFromInt(1000000))
		
		fmt.Printf("%-12s | $%-11s | $%-11s | $%-11s | %-7s%% | %-10d | $%s\n",
			comp.Symbol,
			comp.OurVWAP.StringFixed(2),
			comp.CMCPrice.StringFixed(2),
			comp.PriceDiff.StringFixed(2),
			comp.PriceDiffPct.StringFixed(2),
			comp.OurExchanges,
			volumeInM.StringFixed(2))
		
		totalDiff = totalDiff.Add(comp.PriceDiffPct.Abs())
		count++
	}
	
	if count > 0 {
		avgDiff := totalDiff.Div(decimal.NewFromInt(int64(count)))
		fmt.Printf("\n\nAverage Price Difference: %s%%\n", avgDiff.StringFixed(2))
		
		if avgDiff.LessThan(decimal.NewFromInt(2)) {
			fmt.Println("✅ Excellent accuracy! VWAP calculations are very close to market prices.")
		} else if avgDiff.LessThan(decimal.NewFromInt(5)) {
			fmt.Println("⚠️  Good accuracy. Some minor discrepancies detected.")
		} else {
			fmt.Println("❌ Significant discrepancies detected. Review volume normalization and outlier detection.")
		}
	} else {
		fmt.Println("\nNo comparable pairs found. Checking data availability...")
	}
	
	fmt.Printf("\nComparison performed at: %s\n", time.Now().Format("2006-01-02 15:04:05"))
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}