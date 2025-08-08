package main

import (
	"database/sql"
	"log"
	"os"
	"strings"

	_ "github.com/lib/pq"
)

type TokenMapping struct {
	TokenID          int
	Symbol           string
	ExchangeVariants []string // Different representations across exchanges
}

type ExchangeConfig struct {
	ID      string
	Symbols map[string][]string // token symbol -> exchange-specific symbols
}

func main() {
	// Database connection
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://crypto_user:crypto_password@localhost:5432/crypto_platform?sslmode=disable"
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	defer db.Close()

	// Test connection
	if err := db.Ping(); err != nil {
		log.Fatal("Failed to ping database:", err)
	}

	log.Println("Connected to database")

	// Get all tokens from database
	tokens, err := getTokens(db)
	if err != nil {
		log.Fatal("Failed to get tokens:", err)
	}

	log.Printf("Found %d tokens in database", len(tokens))

	// Define exchange configurations
	exchanges := []ExchangeConfig{
		{
			ID: "binance",
			Symbols: map[string][]string{
				"BTC":  {"BTC"},
				"ETH":  {"ETH"},
				"USDT": {"USDT"},
				"USDC": {"USDC"},
				"BNB":  {"BNB"},
				"SOL":  {"SOL"},
				"XRP":  {"XRP"},
				"ADA":  {"ADA"},
				"DOGE": {"DOGE"},
				"AVAX": {"AVAX"},
			},
		},
		{
			ID: "kraken",
			Symbols: map[string][]string{
				"BTC":  {"XBT", "BTC"},
				"ETH":  {"ETH"},
				"USDT": {"USDT"},
				"USDC": {"USDC"},
				"SOL":  {"SOL"},
				"XRP":  {"XRP"},
				"ADA":  {"ADA"},
				"DOGE": {"DOGE"},
				"AVAX": {"AVAX"},
			},
		},
		{
			ID: "okx",
			Symbols: map[string][]string{
				"BTC":  {"BTC"},
				"ETH":  {"ETH"},
				"USDT": {"USDT"},
				"USDC": {"USDC"},
				"SOL":  {"SOL"},
				"XRP":  {"XRP"},
				"ADA":  {"ADA"},
				"DOGE": {"DOGE"},
				"AVAX": {"AVAX"},
			},
		},
		{
			ID: "coinbase",
			Symbols: map[string][]string{
				"BTC":  {"BTC"},
				"ETH":  {"ETH"},
				"USDT": {"USDT"},
				"USDC": {"USDC"},
				"SOL":  {"SOL"},
				"XRP":  {"XRP"},
				"ADA":  {"ADA"},
				"DOGE": {"DOGE"},
				"AVAX": {"AVAX"},
			},
		},
	}

	// Insert token exchange symbols
	if err := insertTokenExchangeSymbols(db, tokens, exchanges); err != nil {
		log.Fatal("Failed to insert token exchange symbols:", err)
	}

	// Insert common trading pairs
	if err := insertTradingPairs(db, tokens, exchanges); err != nil {
		log.Fatal("Failed to insert trading pairs:", err)
	}

	log.Println("Successfully populated token mappings and trading pairs")
}

func getTokens(db *sql.DB) (map[string]int, error) {
	query := `SELECT id, symbol FROM tokens WHERE is_active = true`
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tokens := make(map[string]int)
	for rows.Next() {
		var id int
		var symbol string
		if err := rows.Scan(&id, &symbol); err != nil {
			return nil, err
		}
		tokens[strings.ToUpper(symbol)] = id
	}

	return tokens, nil
}

func insertTokenExchangeSymbols(db *sql.DB, tokens map[string]int, exchanges []ExchangeConfig) error {
	query := `
		INSERT INTO token_exchange_symbols (token_id, exchange_id, exchange_symbol, normalized_symbol, is_active)
		VALUES ($1, $2, $3, $4, true)
		ON CONFLICT (exchange_id, exchange_symbol) 
		DO UPDATE SET token_id = $1, normalized_symbol = $4, updated_at = NOW()
	`

	stmt, err := db.Prepare(query)
	if err != nil {
		return err
	}
	defer stmt.Close()

	count := 0
	for _, exchange := range exchanges {
		for normalizedSymbol, exchangeSymbols := range exchange.Symbols {
			tokenID, ok := tokens[normalizedSymbol]
			if !ok {
				log.Printf("Token %s not found in database, skipping", normalizedSymbol)
				continue
			}

			for _, exchangeSymbol := range exchangeSymbols {
				_, err := stmt.Exec(tokenID, exchange.ID, exchangeSymbol, normalizedSymbol)
				if err != nil {
					log.Printf("Failed to insert mapping for %s/%s on %s: %v", 
						exchangeSymbol, normalizedSymbol, exchange.ID, err)
					continue
				}
				count++
			}
		}
	}

	log.Printf("Inserted %d token exchange symbol mappings", count)
	return nil
}

func insertTradingPairs(db *sql.DB, tokens map[string]int, exchanges []ExchangeConfig) error {
	// Common quote currencies
	quoteCurrencies := []string{"USDT", "USDC", "USD", "BTC", "ETH", "BNB"}
	
	// Common base currencies to pair
	baseCurrencies := []string{"BTC", "ETH", "SOL", "XRP", "ADA", "DOGE", "AVAX", "BNB"}

	query := `
		INSERT INTO trading_pairs (base_token_id, quote_token_id, exchange_id, exchange_pair_symbol, is_active)
		VALUES ($1, $2, $3, $4, true)
		ON CONFLICT (exchange_id, exchange_pair_symbol)
		DO UPDATE SET base_token_id = $1, quote_token_id = $2, updated_at = NOW()
	`

	stmt, err := db.Prepare(query)
	if err != nil {
		return err
	}
	defer stmt.Close()

	count := 0
	for _, exchange := range exchanges {
		for _, base := range baseCurrencies {
			baseID, ok := tokens[base]
			if !ok {
				continue
			}

			for _, quote := range quoteCurrencies {
				if base == quote {
					continue // Skip same currency pairs
				}

				quoteID, ok := tokens[quote]
				if !ok {
					continue
				}

				// Generate pair symbol based on exchange format
				var pairSymbol string
				switch exchange.ID {
				case "binance":
					pairSymbol = base + quote
				case "kraken":
					// Kraken uses XBT for BTC
					baseSymbol := base
					if base == "BTC" {
						baseSymbol = "XBT"
					}
					pairSymbol = baseSymbol + quote
				case "okx", "coinbase":
					pairSymbol = base + "-" + quote
				default:
					pairSymbol = base + quote
				}

				_, err := stmt.Exec(baseID, quoteID, exchange.ID, pairSymbol)
				if err != nil {
					log.Printf("Failed to insert pair %s on %s: %v", pairSymbol, exchange.ID, err)
					continue
				}
				count++
			}
		}
	}

	log.Printf("Inserted %d trading pairs", count)
	return nil
}