package main

import (
	"database/sql"
	"log"
	"os"
	"strings"

	_ "github.com/lib/pq"
)

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
	tokens, err := getAllTokens(db)
	if err != nil {
		log.Fatal("Failed to get tokens:", err)
	}

	log.Printf("Found %d tokens in database", len(tokens))

	// Populate token exchange symbols for ALL tokens
	if err := populateAllTokenMappings(db, tokens); err != nil {
		log.Fatal("Failed to populate token mappings:", err)
	}

	// Populate trading pairs for common combinations
	if err := populateAllTradingPairs(db, tokens); err != nil {
		log.Fatal("Failed to populate trading pairs:", err)
	}

	log.Println("Successfully populated all token mappings and trading pairs")
}

func getAllTokens(db *sql.DB) (map[string]int, error) {
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

func populateAllTokenMappings(db *sql.DB, tokens map[string]int) error {
	exchanges := []string{"binance", "kraken", "okx", "coinbase"}
	
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
	for symbol, tokenID := range tokens {
		// For each token, create mappings for all exchanges
		for _, exchange := range exchanges {
			// Standard mapping
			exchangeSymbol := symbol
			
			// Special cases for Kraken
			if exchange == "kraken" && symbol == "BTC" {
				// Add both BTC and XBT for Kraken
				_, err := stmt.Exec(tokenID, exchange, "XBT", symbol)
				if err != nil {
					log.Printf("Failed to insert XBT mapping for Kraken: %v", err)
				} else {
					count++
				}
			}
			
			// Insert standard mapping
			_, err := stmt.Exec(tokenID, exchange, exchangeSymbol, symbol)
			if err != nil {
				log.Printf("Failed to insert mapping for %s on %s: %v", symbol, exchange, err)
			} else {
				count++
			}
		}
	}

	log.Printf("Inserted %d token exchange symbol mappings", count)
	return nil
}

func populateAllTradingPairs(db *sql.DB, tokens map[string]int) error {
	// Most common quote currencies in order of preference
	majorQuotes := []string{"USDT", "USDC", "USD", "BUSD", "DAI", "TUSD", "USDP", "FDUSD"}
	cryptoQuotes := []string{"BTC", "ETH", "BNB"}
	fiatQuotes := []string{"EUR", "GBP", "JPY", "AUD", "CAD", "CHF", "CNY", "KRW"}
	
	allQuotes := append(append(majorQuotes, cryptoQuotes...), fiatQuotes...)

	exchanges := []struct {
		id        string
		separator string
	}{
		{"binance", ""},    // BTCUSDT
		{"kraken", ""},     // XBTUSDT
		{"okx", "-"},       // BTC-USDT
		{"coinbase", "-"},  // BTC-USD
	}

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
	processedPairs := make(map[string]bool)

	// For each base token
	for baseSymbol, baseID := range tokens {
		// Try pairing with each quote currency
		for _, quoteSymbol := range allQuotes {
			if baseSymbol == quoteSymbol {
				continue // Skip same currency pairs
			}

			quoteID, ok := tokens[quoteSymbol]
			if !ok {
				continue // Quote currency not in our token list
			}

			// For each exchange
			for _, exchange := range exchanges {
				// Generate pair symbol based on exchange format
				var pairSymbol string
				baseExchangeSymbol := baseSymbol
				
				// Special handling for Kraken BTC
				if exchange.id == "kraken" && baseSymbol == "BTC" {
					baseExchangeSymbol = "XBT"
				}

				if exchange.separator != "" {
					pairSymbol = baseExchangeSymbol + exchange.separator + quoteSymbol
				} else {
					pairSymbol = baseExchangeSymbol + quoteSymbol
				}

				// Create unique key to avoid duplicates
				key := exchange.id + ":" + pairSymbol
				if processedPairs[key] {
					continue
				}
				processedPairs[key] = true

				_, err := stmt.Exec(baseID, quoteID, exchange.id, pairSymbol)
				if err != nil {
					// Only log errors for major pairs
					if contains([]string{"BTC", "ETH", "BNB", "SOL", "XRP"}, baseSymbol) &&
					   contains([]string{"USDT", "USDC", "USD"}, quoteSymbol) {
						log.Printf("Failed to insert pair %s on %s: %v", pairSymbol, exchange.id, err)
					}
				} else {
					count++
				}
			}
		}
	}

	log.Printf("Inserted %d trading pairs", count)
	return nil
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}