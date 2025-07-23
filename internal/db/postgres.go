package db

import (
	"database/sql"
	"fmt"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/ashmitsharp/trading/internal/config"
	_ "github.com/lib/pq"
)

// InitPostgres initializes PostgreSQL connection and creates necessary tables
func InitPostgres(cfg config.PostgresConfig) (*sql.DB, error) {
	db, err := sql.Open("postgres", cfg.ConnectionString())
	if err != nil {
		return nil, fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping PostgreSQL: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)

	return db, nil
}

// CreatePostgresTables creates the required PostgreSQL tables
func CreatePostgresTables(db *sql.DB) error {
	// Create tokens metadata table
	tokensTableSQL := `
		CREATE TABLE IF NOT EXISTS tokens (
			id SERIAL PRIMARY KEY,
			symbol VARCHAR(20) UNIQUE NOT NULL,
			name VARCHAR(100) NOT NULL,
			category VARCHAR(50),
			description TEXT,
			market_cap DECIMAL(20, 2),
			circulating_supply DECIMAL(20, 8),
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);

		CREATE INDEX IF NOT EXISTS idx_tokens_symbol ON tokens(symbol);
		CREATE INDEX IF NOT EXISTS idx_tokens_category ON tokens(category);

		-- Create trigger to update updated_at timestamp
		CREATE OR REPLACE FUNCTION update_updated_at_column()
		RETURNS TRIGGER AS $$
		BEGIN
			NEW.updated_at = CURRENT_TIMESTAMP;
			RETURN NEW;
		END;
		$$ language 'plpgsql';

		DROP TRIGGER IF EXISTS update_tokens_updated_at ON tokens;
		CREATE TRIGGER update_tokens_updated_at
			BEFORE UPDATE ON tokens
			FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
	`

	if _, err := db.Exec(tokensTableSQL); err != nil {
		return fmt.Errorf("failed to create tokens table: %w", err)
	}

	return nil
}

// InitSchemas initializes both ClickHouse and PostgreSQL schemas
func InitSchemas(clickhouseConn driver.Conn, postgresDB *sql.DB) error {
	// Initialize ClickHouse tables
	if err := CreateClickHouseTables(clickhouseConn); err != nil {
		return fmt.Errorf("failed to initialize ClickHouse schema: %w", err)
	}

	// Initialize PostgreSQL tables
	if err := CreatePostgresTables(postgresDB); err != nil {
		return fmt.Errorf("failed to initialize PostgreSQL schema: %w", err)
	}

	// Insert initial token data
	if err := InsertInitialTokens(postgresDB); err != nil {
		return fmt.Errorf("failed to insert initial tokens: %w", err)
	}

	return nil
}

// Token represents a cryptocurrency token
type Token struct {
	ID                int     `json:"id" db:"id"`
	Symbol            string  `json:"symbol" db:"symbol"`
	Name              string  `json:"name" db:"name"`
	Category          string  `json:"category" db:"category"`
	Description       string  `json:"description" db:"description"`
	MarketCap         float64 `json:"market_cap" db:"market_cap"`
	CirculatingSupply float64 `json:"circulating_supply" db:"circulating_supply"`
	CreatedAt         string  `json:"created_at" db:"created_at"`
	UpdatedAt         string  `json:"updated_at" db:"updated_at"`
}

// InsertInitialTokens inserts initial token metadata
func InsertInitialTokens(db *sql.DB) error {
	tokens := []Token{
		{
			Symbol:      "BTCUSDT",
			Name:        "Bitcoin",
			Category:    "Layer 1",
			Description: "The first and most well-known cryptocurrency",
		},
		{
			Symbol:      "ETHUSDT",
			Name:        "Ethereum",
			Category:    "Layer 1",
			Description: "Smart contract platform and cryptocurrency",
		},
		{
			Symbol:      "ADAUSDT",
			Name:        "Cardano",
			Category:    "Layer 1",
			Description: "Proof-of-stake blockchain platform",
		},
		{
			Symbol:      "BNBUSDT",
			Name:        "Binance Coin",
			Category:    "Exchange Token",
			Description: "Native token of the Binance ecosystem",
		},
		{
			Symbol:      "XRPUSDT",
			Name:        "XRP",
			Category:    "Payment",
			Description: "Digital payment protocol and cryptocurrency",
		},
		{
			Symbol:      "SOLUSDT",
			Name:        "Solana",
			Category:    "Layer 1",
			Description: "High-performance blockchain platform",
		},
		{
			Symbol:      "DOTUSDT",
			Name:        "Polkadot",
			Category:    "Layer 0",
			Description: "Multi-chain protocol enabling blockchain interoperability",
		},
		{
			Symbol:      "LINKUSDT",
			Name:        "Chainlink",
			Category:    "Oracle",
			Description: "Decentralized oracle network",
		},
		{
			Symbol:      "LTCUSDT",
			Name:        "Litecoin",
			Category:    "Payment",
			Description: "Peer-to-peer cryptocurrency based on Bitcoin",
		},
		{
			Symbol:      "BCHUSDT",
			Name:        "Bitcoin Cash",
			Category:    "Payment",
			Description: "Fork of Bitcoin designed for faster transactions",
		},
	}

	for _, token := range tokens {
		query := `
			INSERT INTO tokens (symbol, name, category, description)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (symbol) DO UPDATE SET
				name = EXCLUDED.name,
				category = EXCLUDED.category,
				description = EXCLUDED.description,
				updated_at = CURRENT_TIMESTAMP
		`

		if _, err := db.Exec(query, token.Symbol, token.Name, token.Category, token.Description); err != nil {
			return fmt.Errorf("failed to insert token %s: %w", token.Symbol, err)
		}
	}

	return nil
}

// GetTokenBySymbol retrieves token metadata by symbol
func GetTokenBySymbol(db *sql.DB, symbol string) (*Token, error) {
	query := `
		SELECT id, symbol, name, category, description, 
			   COALESCE(market_cap, 0), COALESCE(circulating_supply, 0),
			   created_at, updated_at
		FROM tokens 
		WHERE symbol = $1
	`

	var token Token
	err := db.QueryRow(query, symbol).Scan(
		&token.ID, &token.Symbol, &token.Name, &token.Category, &token.Description,
		&token.MarketCap, &token.CirculatingSupply, &token.CreatedAt, &token.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("token not found: %s", symbol)
		}
		return nil, fmt.Errorf("failed to get token: %w", err)
	}

	return &token, nil
}

// GetAllTokens retrieves all token metadata
func GetAllTokens(db *sql.DB) ([]Token, error) {
	query := `
		SELECT id, symbol, name, category, description, 
			   COALESCE(market_cap, 0), COALESCE(circulating_supply, 0),
			   created_at, updated_at
		FROM tokens 
		ORDER BY symbol
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query tokens: %w", err)
	}
	defer rows.Close()

	var tokens []Token

	for rows.Next() {
		var token Token
		err := rows.Scan(
			&token.ID, &token.Symbol, &token.Name, &token.Category, &token.Description,
			&token.MarketCap, &token.CirculatingSupply, &token.CreatedAt, &token.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan token: %w", err)
		}
		tokens = append(tokens, token)
	}

	return tokens, nil
}

// UpdateTokenMarketData updates token market data
func UpdateTokenMarketData(db *sql.DB, symbol string, marketCap, circulatingSupply float64) error {
	query := `
		UPDATE tokens 
		SET market_cap = $2, circulating_supply = $3, updated_at = CURRENT_TIMESTAMP
		WHERE symbol = $1
	`

	result, err := db.Exec(query, symbol, marketCap, circulatingSupply)
	if err != nil {
		return fmt.Errorf("failed to update token market data: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("no token found with symbol: %s", symbol)
	}

	return nil
}
