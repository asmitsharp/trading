package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"go.uber.org/zap"

	"github.com/ashmitsharp/trading/internal/exchanges"
	"github.com/ashmitsharp/trading/internal/symbol"
)

type MissingToken struct {
	Symbol      string
	ExchangeID  string
	PairSymbol  string
	BaseSymbol  string
	QuoteSymbol string
	Count       int
}

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Printf("No .env file found: %v", err)
	}

	// Initialize logger
	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer logger.Sync()

	// Connect to database
	pgDSN := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		"localhost", "5432", "crypto_user", "crypto_password", "crypto_platform")

	db, err := sql.Open("postgres", pgDSN)
	if err != nil {
		logger.Fatal("Failed to connect to PostgreSQL", zap.Error(err))
	}
	defer db.Close()

	// Initialize exchange factory
	factory, err := exchanges.NewExchangeFactory("configs/exchanges.json", logger)
	if err != nil {
		logger.Fatal("Failed to create exchange factory", zap.Error(err))
	}

	// Initialize symbol resolver
	resolver := symbol.NewResolver(db, logger)

	// Collect missing tokens
	missingTokens := make(map[string]*MissingToken)

	// Get tickers from all exchanges
	clients := factory.CreateAllClients()
	logger.Info("Collecting tickers from exchanges", zap.Int("exchange_count", len(clients)))

	for exchangeID, client := range clients {
		if !client.IsHealthy() {
			logger.Warn("Skipping unhealthy exchange", zap.String("exchange", exchangeID))
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		tickers, err := client.GetAllTickers(ctx)
		cancel()

		if err != nil {
			logger.Error("Failed to get tickers", 
				zap.String("exchange", exchangeID),
				zap.Error(err))
			continue
		}

		logger.Info("Got tickers", 
			zap.String("exchange", exchangeID),
			zap.Int("count", len(tickers)))

		// Check each ticker for missing tokens
		for _, ticker := range tickers {
			// Try to resolve base token
			if ticker.BaseSymbol != "" {
				_, err := resolver.ResolveSymbol(exchangeID, ticker.BaseSymbol)
				if err != nil {
					key := strings.ToUpper(ticker.BaseSymbol)
					if token, exists := missingTokens[key]; exists {
						token.Count++
					} else {
						missingTokens[key] = &MissingToken{
							Symbol:      ticker.BaseSymbol,
							ExchangeID:  exchangeID,
							PairSymbol:  ticker.Symbol,
							BaseSymbol:  ticker.BaseSymbol,
							QuoteSymbol: ticker.QuoteSymbol,
							Count:       1,
						}
					}
				}
			}

			// Try to resolve quote token (if not a common quote currency)
			if ticker.QuoteSymbol != "" && !isCommonQuote(ticker.QuoteSymbol) {
				_, err := resolver.ResolveSymbol(exchangeID, ticker.QuoteSymbol)
				if err != nil {
					key := strings.ToUpper(ticker.QuoteSymbol)
					if token, exists := missingTokens[key]; exists {
						token.Count++
					} else {
						missingTokens[key] = &MissingToken{
							Symbol:      ticker.QuoteSymbol,
							ExchangeID:  exchangeID,
							PairSymbol:  ticker.Symbol,
							BaseSymbol:  ticker.BaseSymbol,
							QuoteSymbol: ticker.QuoteSymbol,
							Count:       1,
						}
					}
				}
			}
		}
	}

	// Report missing tokens
	logger.Info("Missing tokens found", zap.Int("total", len(missingTokens)))

	// Create SQL for adding missing tokens
	fmt.Println("\n-- SQL to add missing tokens (tokens seen on 3+ exchanges):")
	fmt.Println("-- Run this in your PostgreSQL database\n")

	fmt.Println("BEGIN;")
	fmt.Println("\n-- Add missing tokens")

	count := 0
	for symbol, token := range missingTokens {
		// Only add tokens seen on multiple exchanges to avoid spam tokens
		if token.Count >= 3 {
			name := generateTokenName(symbol)
			slug := strings.ToLower(symbol)
			tokenType := determineTokenType(symbol)

			fmt.Printf(`INSERT INTO tokens (symbol, slug, name, token_type, is_active, created_at, updated_at)
VALUES ('%s', '%s', '%s', '%s', true, NOW(), NOW())
ON CONFLICT (symbol) DO NOTHING;
`, symbol, slug, name, tokenType)
			count++
		}
	}

	fmt.Printf("\n-- Added %d missing tokens\n", count)
	fmt.Println("\nCOMMIT;")

	// Also save to a file
	saveToFile(missingTokens, "missing_tokens.sql")
	logger.Info("SQL script saved to missing_tokens.sql")
}

func isCommonQuote(symbol string) bool {
	commonQuotes := map[string]bool{
		"USDT": true, "USDC": true, "BTC": true, "ETH": true,
		"USD": true, "EUR": true, "BUSD": true, "BNB": true,
		"FDUSD": true, "TUSD": true, "DAI": true,
	}
	return commonQuotes[strings.ToUpper(symbol)]
}

func generateTokenName(symbol string) string {
	// Generate a reasonable name from the symbol
	if strings.HasSuffix(symbol, "USDT") || strings.HasSuffix(symbol, "USDC") {
		base := strings.TrimSuffix(strings.TrimSuffix(symbol, "USDT"), "USDC")
		return base + " Token"
	}
	return symbol + " Token"
}

func determineTokenType(symbol string) string {
	// Determine token type
	stables := map[string]bool{
		"USDT": true, "USDC": true, "BUSD": true, "DAI": true,
		"TUSD": true, "USDP": true, "FRAX": true, "GUSD": true,
	}
	
	fiats := map[string]bool{
		"USD": true, "EUR": true, "GBP": true, "JPY": true,
		"TRY": true, "BRL": true, "MXN": true, "ZAR": true,
	}

	upperSymbol := strings.ToUpper(symbol)
	if stables[upperSymbol] {
		return "stablecoin"
	}
	if fiats[upperSymbol] {
		return "fiat"
	}
	return "crypto"
}

func saveToFile(tokens map[string]*MissingToken, filename string) {
	content := "-- Missing tokens SQL script\n"
	content += "-- Generated at: " + time.Now().Format(time.RFC3339) + "\n\n"
	content += "BEGIN;\n\n"

	for symbol, token := range tokens {
		if token.Count >= 3 {
			name := generateTokenName(symbol)
			slug := strings.ToLower(symbol)
			tokenType := determineTokenType(symbol)

			content += fmt.Sprintf(`INSERT INTO tokens (symbol, slug, name, token_type, is_active, created_at, updated_at)
VALUES ('%s', '%s', '%s', '%s', true, NOW(), NOW())
ON CONFLICT (symbol) DO NOTHING;
`, symbol, slug, name, tokenType)
		}
	}

	content += "\nCOMMIT;\n"

	// Write to file
	if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
		log.Printf("Failed to write file: %v", err)
	}
}