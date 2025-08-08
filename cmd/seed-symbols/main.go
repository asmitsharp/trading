package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	_ "github.com/lib/pq"
)

// SymbolMapping represents a token symbol mapping for an exchange
type SymbolMapping struct {
	TokenSymbol      string
	ExchangeID       string
	ExchangeSymbol   string
	NormalizedSymbol string
}

// PairMapping represents a trading pair mapping
type PairMapping struct {
	BaseSymbol         string
	QuoteSymbol        string
	ExchangeID         string
	ExchangePairSymbol string
}

func main() {
	// Connect to PostgreSQL
	dbHost := os.Getenv("POSTGRES_HOST")
	if dbHost == "" {
		dbHost = "localhost"
	}

	// Load environment variables
	dbUser := os.Getenv("POSTGRES_USER")
	if dbUser == "" {
		dbUser = "crypto_user"
	}
	dbName := os.Getenv("POSTGRES_DB")
	if dbName == "" {
		dbName = "crypto_platform"
	}
	dbPassword := os.Getenv("POSTGRES_PASSWORD")
	if dbPassword == "" {
		dbPassword = "crypto_password"
	}

	connStr := fmt.Sprintf("host=%s port=5432 user=%s password=%s dbname=%s sslmode=disable", dbHost, dbUser, dbPassword, dbName)
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	defer db.Close()

	// Test connection
	if err := db.Ping(); err != nil {
		log.Fatal("Failed to ping database:", err)
	}

	log.Println("Connected to PostgreSQL")

	// Seed symbol mappings
	if err := seedSymbolMappings(db); err != nil {
		log.Fatal("Failed to seed symbol mappings:", err)
	}

	// Seed trading pair mappings
	if err := seedTradingPairs(db); err != nil {
		log.Fatal("Failed to seed trading pairs:", err)
	}

	log.Println("Symbol mappings seeded successfully!")
}

func seedSymbolMappings(db *sql.DB) error {
	// Common token mappings across exchanges
	mappings := []SymbolMapping{
		// Bitcoin variations
		{"BTC", "binance", "BTC", "BTC"},
		{"BTC", "coinbase", "BTC", "BTC"},
		{"BTC", "kraken", "XBT", "BTC"},
		{"BTC", "kraken", "XXBT", "BTC"},
		{"BTC", "bitfinex", "BTC", "BTC"},

		// Ethereum
		{"ETH", "binance", "ETH", "ETH"},
		{"ETH", "coinbase", "ETH", "ETH"},
		{"ETH", "kraken", "ETH", "ETH"},
		{"ETH", "kraken", "XETH", "ETH"},

		// Stablecoins
		{"USDT", "binance", "USDT", "USDT"},
		{"USDT", "coinbase", "USDT", "USDT"},
		{"USDT", "kraken", "USDT", "USDT"},
		{"USDC", "binance", "USDC", "USDC"},
		{"USDC", "coinbase", "USDC", "USDC"},
		{"USDC", "kraken", "USDC", "USDC"},

		// USD representations
		{"USD", "coinbase", "USD", "USD"},
		{"USD", "kraken", "USD", "USD"},
		{"USD", "kraken", "ZUSD", "USD"},
		{"USD", "bitstamp", "USD", "USD"},
		{"USD", "gemini", "USD", "USD"},

		// Other major tokens
		{"BNB", "binance", "BNB", "BNB"},
		{"SOL", "binance", "SOL", "SOL"},
		{"SOL", "coinbase", "SOL", "SOL"},
		{"ADA", "binance", "ADA", "ADA"},
		{"ADA", "coinbase", "ADA", "ADA"},
		{"ADA", "kraken", "ADA", "ADA"},
		{"DOT", "binance", "DOT", "DOT"},
		{"DOT", "coinbase", "DOT", "DOT"},
		{"DOT", "kraken", "DOT", "DOT"},
		{"MATIC", "binance", "MATIC", "MATIC"},
		{"MATIC", "coinbase", "MATIC", "MATIC"},
		{"MATIC", "kraken", "MATIC", "MATIC"},
		{"AVAX", "binance", "AVAX", "AVAX"},
		{"AVAX", "coinbase", "AVAX", "AVAX"},
		{"LINK", "binance", "LINK", "LINK"},
		{"LINK", "coinbase", "LINK", "LINK"},
		{"LINK", "kraken", "LINK", "LINK"},
		{"UNI", "binance", "UNI", "UNI"},
		{"UNI", "coinbase", "UNI", "UNI"},
		{"ATOM", "binance", "ATOM", "ATOM"},
		{"ATOM", "coinbase", "ATOM", "ATOM"},
		{"XRP", "binance", "XRP", "XRP"},
		{"XRP", "coinbase", "XRP", "XRP"},
		{"XRP", "kraken", "XRP", "XRP"},
		{"XRP", "kraken", "XXRP", "XRP"},
		{"LTC", "binance", "LTC", "LTC"},
		{"LTC", "coinbase", "LTC", "LTC"},
		{"LTC", "kraken", "LTC", "LTC"},
		{"LTC", "kraken", "XLTC", "LTC"},
		{"DOGE", "binance", "DOGE", "DOGE"},
		{"DOGE", "coinbase", "DOGE", "DOGE"},
		{"DOGE", "kraken", "DOGE", "DOGE"},
		{"DOGE", "kraken", "XDOGE", "DOGE"},
	}

	// First, get token IDs
	tokenIDs := make(map[string]int)
	rows, err := db.Query("SELECT id, symbol FROM tokens WHERE is_active = true")
	if err != nil {
		return fmt.Errorf("failed to query tokens: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id int
		var symbol string
		if err := rows.Scan(&id, &symbol); err != nil {
			continue
		}
		tokenIDs[symbol] = id
	}

	// Insert mappings
	stmt, err := db.Prepare(`
		INSERT INTO token_exchange_symbols (token_id, exchange_id, exchange_symbol, normalized_symbol)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (exchange_id, exchange_symbol) DO UPDATE
		SET token_id = $1, normalized_symbol = $4, updated_at = NOW()
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	inserted := 0
	for _, mapping := range mappings {
		tokenID, ok := tokenIDs[mapping.TokenSymbol]
		if !ok {
			log.Printf("Token %s not found in database, skipping", mapping.TokenSymbol)
			continue
		}

		_, err := stmt.Exec(tokenID, mapping.ExchangeID, mapping.ExchangeSymbol, mapping.NormalizedSymbol)
		if err != nil {
			log.Printf("Failed to insert mapping for %s on %s: %v", mapping.TokenSymbol, mapping.ExchangeID, err)
			continue
		}
		inserted++
	}

	log.Printf("Inserted %d symbol mappings", inserted)
	return nil
}

func seedTradingPairs(db *sql.DB) error {
	// Common trading pairs
	pairs := []PairMapping{
		// BTC pairs
		{"BTC", "USDT", "binance", "BTCUSDT"},
		{"BTC", "USDC", "binance", "BTCUSDC"},
		{"BTC", "USD", "coinbase", "BTC-USD"},
		{"BTC", "USDT", "coinbase", "BTC-USDT"},
		{"BTC", "USD", "kraken", "XXBTZUSD"},
		{"BTC", "USDT", "kraken", "XBTUSDT"},
		{"BTC", "EUR", "kraken", "XXBTZEUR"},
		{"BTC", "USDT", "okx", "BTC-USDT"},
		{"BTC", "USDC", "okx", "BTC-USDC"},

		// ETH pairs
		{"ETH", "USDT", "binance", "ETHUSDT"},
		{"ETH", "USDC", "binance", "ETHUSDC"},
		{"ETH", "BTC", "binance", "ETHBTC"},
		{"ETH", "USD", "coinbase", "ETH-USD"},
		{"ETH", "USDT", "coinbase", "ETH-USDT"},
		{"ETH", "BTC", "coinbase", "ETH-BTC"},
		{"ETH", "USD", "kraken", "ETHUSD"},
		{"ETH", "USDT", "kraken", "ETHUSDT"},
		{"ETH", "BTC", "kraken", "ETHXBT"},

		// Other major pairs
		{"SOL", "USDT", "binance", "SOLUSDT"},
		{"SOL", "USD", "coinbase", "SOL-USD"},
		{"ADA", "USDT", "binance", "ADAUSDT"},
		{"ADA", "USD", "coinbase", "ADA-USD"},
		{"DOT", "USDT", "binance", "DOTUSDT"},
		{"DOT", "USD", "coinbase", "DOT-USD"},
		{"MATIC", "USDT", "binance", "MATICUSDT"},
		{"MATIC", "USD", "coinbase", "MATIC-USD"},
		{"AVAX", "USDT", "binance", "AVAXUSDT"},
		{"AVAX", "USD", "coinbase", "AVAX-USD"},
		{"LINK", "USDT", "binance", "LINKUSDT"},
		{"LINK", "USD", "coinbase", "LINK-USD"},
		{"UNI", "USDT", "binance", "UNIUSDT"},
		{"UNI", "USD", "coinbase", "UNI-USD"},
		{"ATOM", "USDT", "binance", "ATOMUSDT"},
		{"ATOM", "USD", "coinbase", "ATOM-USD"},
		{"XRP", "USDT", "binance", "XRPUSDT"},
		{"XRP", "USD", "coinbase", "XRP-USD"},
		{"LTC", "USDT", "binance", "LTCUSDT"},
		{"LTC", "USD", "coinbase", "LTC-USD"},
		{"DOGE", "USDT", "binance", "DOGEUSDT"},
		{"DOGE", "USD", "coinbase", "DOGE-USD"},
	}

	// Get token IDs
	tokenIDs := make(map[string]int)
	rows, err := db.Query("SELECT id, symbol FROM tokens WHERE is_active = true")
	if err != nil {
		return fmt.Errorf("failed to query tokens: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id int
		var symbol string
		if err := rows.Scan(&id, &symbol); err != nil {
			continue
		}
		tokenIDs[symbol] = id
	}

	// Insert trading pairs
	stmt, err := db.Prepare(`
		INSERT INTO trading_pairs (base_token_id, quote_token_id, exchange_id, exchange_pair_symbol)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (exchange_id, exchange_pair_symbol) DO UPDATE
		SET base_token_id = $1, quote_token_id = $2, updated_at = NOW()
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	inserted := 0
	for _, pair := range pairs {
		baseID, baseOk := tokenIDs[pair.BaseSymbol]
		quoteID, quoteOk := tokenIDs[pair.QuoteSymbol]

		if !baseOk || !quoteOk {
			log.Printf("Tokens not found for pair %s/%s, skipping", pair.BaseSymbol, pair.QuoteSymbol)
			continue
		}

		_, err := stmt.Exec(baseID, quoteID, pair.ExchangeID, pair.ExchangePairSymbol)
		if err != nil {
			log.Printf("Failed to insert pair %s on %s: %v", pair.ExchangePairSymbol, pair.ExchangeID, err)
			continue
		}
		inserted++
	}

	log.Printf("Inserted %d trading pairs", inserted)
	return nil
}
