package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"

	_ "github.com/lib/pq"
	"github.com/shopspring/decimal"
)

type ExchangeConfig struct {
	Exchanges []Exchange `json:"exchanges"`
}

type Exchange struct {
	ID                   string     `json:"id"`
	Name                 string     `json:"name"`
	BaseURL              string     `json:"base_url"`
	TickerEndpoint       string     `json:"ticker_endpoint"`
	SymbolsEndpoint      string     `json:"symbols_endpoint"`
	RateLimitPerMinute   int        `json:"rate_limit_per_minute"`
	Weight               float64    `json:"weight"`
	RequestTimeout       int        `json:"request_timeout"`
	RetryAttempts        int        `json:"retry_attempts"`
	SymbolFormat         string     `json:"symbol_format"`
	QuoteCurrencies      []string   `json:"quote_currencies"`
	Disabled             bool       `json:"disabled"`
}

func main() {
	// Read config file
	configFile := "configs/exchanges.json"
	data, err := ioutil.ReadFile(configFile)
	if err != nil {
		log.Fatalf("Error reading config file: %v", err)
	}

	var config ExchangeConfig
	if err := json.Unmarshal(data, &config); err != nil {
		log.Fatalf("Error parsing config file: %v", err)
	}

	// Connect to PostgreSQL
	dbURL := getEnv("DATABASE_URL", "postgresql://crypto_user:crypto_password@localhost:5432/crypto_platform?sslmode=disable")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("Error connecting to database: %v", err)
	}
	defer db.Close()

	// Create exchanges table if not exists
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS exchanges (
		id SERIAL PRIMARY KEY,
		exchange_id VARCHAR(50) UNIQUE NOT NULL,
		name VARCHAR(100) NOT NULL,
		base_url TEXT NOT NULL,
		ticker_endpoint TEXT NOT NULL,
		symbols_endpoint TEXT NOT NULL,
		rate_limit_per_minute INTEGER DEFAULT 60,
		request_timeout_ms INTEGER DEFAULT 5000,
		retry_attempts INTEGER DEFAULT 3,
		weight DECIMAL(5,4) DEFAULT 0.01,
		symbol_format VARCHAR(20) DEFAULT 'BTCUSDT',
		quote_currencies TEXT[] DEFAULT '{}',
		headers JSONB DEFAULT '{}',
		api_key VARCHAR(255),
		api_secret VARCHAR(255),
		is_active BOOLEAN DEFAULT true,
		last_successful_poll TIMESTAMP,
		consecutive_failures INTEGER DEFAULT 0,
		avg_response_time_ms INTEGER,
		created_at TIMESTAMP DEFAULT NOW(),
		updated_at TIMESTAMP DEFAULT NOW()
	);`
	
	if _, err := db.Exec(createTableSQL); err != nil {
		log.Printf("Warning creating table (may already exist): %v", err)
	}

	// Sync exchanges from config to database
	insertSQL := `
		INSERT INTO exchanges (
			exchange_id, name, base_url, ticker_endpoint, symbols_endpoint,
			rate_limit_per_minute, weight, request_timeout_ms, retry_attempts,
			symbol_format, quote_currencies, is_active
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (exchange_id) DO UPDATE SET
			name = EXCLUDED.name,
			base_url = EXCLUDED.base_url,
			ticker_endpoint = EXCLUDED.ticker_endpoint,
			symbols_endpoint = EXCLUDED.symbols_endpoint,
			rate_limit_per_minute = EXCLUDED.rate_limit_per_minute,
			weight = EXCLUDED.weight,
			request_timeout_ms = EXCLUDED.request_timeout_ms,
			retry_attempts = EXCLUDED.retry_attempts,
			symbol_format = EXCLUDED.symbol_format,
			quote_currencies = EXCLUDED.quote_currencies,
			is_active = EXCLUDED.is_active,
			updated_at = NOW()`

	successCount := 0
	for _, exchange := range config.Exchanges {
		// Skip disabled exchanges
		isActive := !exchange.Disabled
		
		// Convert quote currencies to PostgreSQL array format
		quoteCurrencies := "{" + strings.Join(exchange.QuoteCurrencies, ",") + "}"
		
		// Default values
		if exchange.RequestTimeout == 0 {
			exchange.RequestTimeout = 15000
		}
		if exchange.RetryAttempts == 0 {
			exchange.RetryAttempts = 3
		}
		if exchange.RateLimitPerMinute == 0 {
			exchange.RateLimitPerMinute = 600
		}
		
		weight := decimal.NewFromFloat(exchange.Weight)
		
		_, err := db.Exec(insertSQL,
			exchange.ID,
			exchange.Name,
			exchange.BaseURL,
			exchange.TickerEndpoint,
			exchange.SymbolsEndpoint,
			exchange.RateLimitPerMinute,
			weight,
			exchange.RequestTimeout,
			exchange.RetryAttempts,
			exchange.SymbolFormat,
			quoteCurrencies,
			isActive,
		)
		
		if err != nil {
			log.Printf("Error syncing exchange %s: %v", exchange.ID, err)
		} else {
			successCount++
			fmt.Printf("✓ Synced exchange: %s (%s)\n", exchange.ID, exchange.Name)
		}
	}

	fmt.Printf("\n✅ Successfully synced %d/%d exchanges to database\n", successCount, len(config.Exchanges))
	
	// Verify
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM exchanges WHERE is_active = true").Scan(&count)
	if err != nil {
		log.Printf("Error counting exchanges: %v", err)
	} else {
		fmt.Printf("📊 Total active exchanges in database: %d\n", count)
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}