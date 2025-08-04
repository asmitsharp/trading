package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/lib/pq"
	_ "github.com/lib/pq"
)

// TokenMetadata represents the structure of your JSON data
type TokenMetadata struct {
	Name                string     `json:"name"`
	Symbol              string     `json:"symbol"`
	Slug                string     `json:"slug"`
	CirculatingSupply   float64    `json:"circulatingSupply"`
	TotalSupply         float64    `json:"totalSupply"`
	MaxSupply           *float64   `json:"maxSupply"` // Pointer to handle null values
	IsInfiniteMaxSupply int        `json:"isInfiniteMaxSupply"`
	URLs                TokenURLs  `json:"urls"`
	Contracts           []Contract `json:"contracts"`
	Decimals            int        `json:"decimals"`
	Categories          []string   `json:"categories"`
	MarketCapRank       *int       `json:"marketCapRank"`
}

type TokenURLs struct {
	Website      []string `json:"website"`
	TechnicalDoc []string `json:"technical_doc"`
	Explorer     []string `json:"explorer"`
	SourceCode   []string `json:"source_code"`
	Reddit       []string `json:"reddit"`
	Chat         []string `json:"chat"`
	Announcement []string `json:"announcement"`
	Twitter      []string `json:"twitter"`
}

type Contract struct {
	No               int      `json:"no"`
	ContractAddress  string   `json:"contractAddress"`
	ContractPlatform string   `json:"contractPlatform"`
	ContractRpcURL   []string `json:"contractRpcUrl"`
}

func main() {
	// Load .env file
	godotenv.Load()

	// Parse command line flags
	var (
		jsonFile = flag.String("file", "", "Path to JSON file containing token data")
		verbose  = flag.Bool("v", false, "Verbose output")
		help     = flag.Bool("h", false, "Show help")
	)
	flag.Parse()

	if *help || *jsonFile == "" {
		fmt.Println("Token Seeder - Import token data from JSON into PostgreSQL")
		fmt.Println("\nUsage:")
		fmt.Println("  go run cmd/seed/main.go -file <path_to_json>")
		fmt.Println("\nOptions:")
		fmt.Println("  -file string    Path to JSON file containing token data (required)")
		fmt.Println("  -v              Verbose output")
		fmt.Println("  -h              Show this help message")
		fmt.Println("\nExample:")
		fmt.Println("  go run cmd/seed/main.go -file configs/tokens.json")
		os.Exit(0)
	}

	// Connect to database
	db, err := connectDB()
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	defer db.Close()

	fmt.Println("✓ Connected to database")

	// Read and parse JSON file
	tokens, err := readTokensFromFile(*jsonFile)
	if err != nil {
		log.Fatal("Failed to read tokens from file:", err)
	}

	fmt.Printf("✓ Loaded %d tokens from %s\n", len(tokens), *jsonFile)

	// Create unique index if it doesn't exist
	if err := createUniqueConstraint(db); err != nil {
		log.Fatal("Failed to create unique constraint:", err)
	}

	// Seed tokens into database
	err = seedTokens(db, tokens, *verbose)
	if err != nil {
		log.Fatal("Failed to seed tokens:", err)
	}

	fmt.Printf("\n✓ Successfully processed %d tokens\n", len(tokens))
}

func connectDB() (*sql.DB, error) {
	// Use same environment variables as migrate command
	host := getEnv("POSTGRES_HOST", "localhost")
	port := getEnv("POSTGRES_PORT", "5432")
	user := getEnv("POSTGRES_USERNAME", "crypto_user")
	password := getEnv("POSTGRES_PASSWORD", "crypto_password")
	dbname := getEnv("POSTGRES_DATABASE", "crypto_platform")
	sslmode := getEnv("POSTGRES_SSLMODE", "disable")

	psqlInfo := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		host, port, user, password, dbname, sslmode)

	db, err := sql.Open("postgres", psqlInfo)
	if err != nil {
		return nil, err
	}

	if err = db.Ping(); err != nil {
		return nil, err
	}

	return db, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func readTokensFromFile(filePath string) ([]TokenMetadata, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("error opening file: %v", err)
	}
	defer file.Close()

	data, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("error reading file: %v", err)
	}

	var tokens []TokenMetadata
	err = json.Unmarshal(data, &tokens)
	if err != nil {
		return nil, fmt.Errorf("error parsing JSON: %v", err)
	}

	return tokens, nil
}

func createUniqueConstraint(db *sql.DB) error {
	// Create a unique constraint on symbol + contract_address + chain combination
	// This prevents duplicate entries for the same token
	query := `
		CREATE UNIQUE INDEX IF NOT EXISTS idx_tokens_unique_symbol_contract_chain 
		ON tokens(symbol, COALESCE(contract_address, ''), COALESCE(chain, ''))
	`
	
	_, err := db.Exec(query)
	if err != nil {
		return fmt.Errorf("error creating unique index: %v", err)
	}
	
	fmt.Println("✓ Ensured unique constraint exists")
	return nil
}

func seedTokens(db *sql.DB, tokens []TokenMetadata, verbose bool) error {
	// Begin transaction
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("error starting transaction: %v", err)
	}
	defer tx.Rollback()

	successCount := 0
	skipCount := 0

	// Process each token
	for _, token := range tokens {
		// Insert main/native token
		inserted, err := insertToken(tx, token, nil, verbose)
		if err != nil {
			return fmt.Errorf("error inserting token %s: %v", token.Symbol, err)
		}
		if inserted {
			successCount++
		} else {
			skipCount++
		}

		// Process contract tokens (wrapped/bridged versions)
		for _, contract := range token.Contracts {
			inserted, err := insertToken(tx, token, &contract, verbose)
			if err != nil {
				return fmt.Errorf("error inserting contract token %s on %s: %v",
					token.Symbol, contract.ContractPlatform, err)
			}
			if inserted {
				successCount++
			} else {
				skipCount++
			}
		}
	}

	// Commit transaction
	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("error committing transaction: %v", err)
	}

	fmt.Printf("\n✓ Inserted/Updated: %d tokens\n", successCount)
	if skipCount > 0 {
		fmt.Printf("✓ Unchanged: %d tokens\n", skipCount)
	}

	return nil
}

func insertToken(tx *sql.Tx, token TokenMetadata, contract *Contract, verbose bool) (bool, error) {
	// Prepare metadata
	metadata := createMetadata(token)
	
	// Add contract-specific metadata if this is a contract token
	var contractAddress, chain *string
	if contract != nil {
		contractAddress = &contract.ContractAddress
		chain = &contract.ContractPlatform
		metadata["contract_info"] = map[string]interface{}{
			"rpc_urls":        contract.ContractRpcURL,
			"contract_number": contract.No,
		}
	}

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return false, fmt.Errorf("error marshaling metadata: %v", err)
	}

	// Prepare categories for PostgreSQL array
	var categories interface{}
	if len(token.Categories) > 0 {
		categories = pq.Array(token.Categories)
	} else {
		categories = pq.Array([]string{})
	}

	// Handle max supply
	var maxSupply *float64
	if token.MaxSupply != nil && token.IsInfiniteMaxSupply == 0 {
		maxSupply = token.MaxSupply
	}

	// Use default decimals if not specified
	decimals := token.Decimals
	if decimals == 0 {
		// Set default decimals based on common tokens
		switch token.Symbol {
		case "BTC":
			decimals = 8
		case "ETH", "USDC", "USDT":
			decimals = 18
		default:
			decimals = 18
		}
	}

	// Use UPSERT to handle duplicates gracefully
	query := `
		INSERT INTO tokens (
			symbol, name, contract_address, chain, decimals,
			circulating_supply, total_supply, max_supply,
			market_cap_rank, categories, metadata, is_active
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (symbol, COALESCE(contract_address, ''), COALESCE(chain, ''))
		DO UPDATE SET
			name = EXCLUDED.name,
			decimals = EXCLUDED.decimals,
			circulating_supply = EXCLUDED.circulating_supply,
			total_supply = EXCLUDED.total_supply,
			max_supply = EXCLUDED.max_supply,
			market_cap_rank = COALESCE(EXCLUDED.market_cap_rank, tokens.market_cap_rank),
			categories = EXCLUDED.categories,
			metadata = tokens.metadata || EXCLUDED.metadata,
			updated_at = NOW()
		RETURNING (xmax = 0) AS inserted
	`

	var inserted bool
	err = tx.QueryRow(query,
		token.Symbol,
		token.Name,
		contractAddress,
		chain,
		decimals,
		token.CirculatingSupply,
		token.TotalSupply,
		maxSupply,
		token.MarketCapRank,
		categories,
		string(metadataJSON),
		true,
	).Scan(&inserted)

	if err != nil {
		return false, err
	}

	if verbose {
		if contract != nil {
			if inserted {
				fmt.Printf("  ✓ Inserted contract token: %s on %s (%s)\n",
					token.Symbol, contract.ContractPlatform, contract.ContractAddress[:10]+"...")
			} else {
				fmt.Printf("  → Updated contract token: %s on %s\n",
					token.Symbol, contract.ContractPlatform)
			}
		} else {
			if inserted {
				fmt.Printf("✓ Inserted native token: %s (%s)\n", token.Name, token.Symbol)
			} else {
				fmt.Printf("→ Updated native token: %s (%s)\n", token.Name, token.Symbol)
			}
		}
	}

	return inserted, nil
}

func createMetadata(token TokenMetadata) map[string]interface{} {
	metadata := make(map[string]interface{})

	// Add URLs if they exist
	urls := make(map[string]interface{})
	if len(token.URLs.Website) > 0 {
		urls["website"] = token.URLs.Website
	}
	if len(token.URLs.TechnicalDoc) > 0 {
		urls["technical_doc"] = token.URLs.TechnicalDoc
	}
	if len(token.URLs.Explorer) > 0 {
		urls["explorer"] = token.URLs.Explorer
	}
	if len(token.URLs.SourceCode) > 0 {
		urls["source_code"] = token.URLs.SourceCode
	}
	if len(token.URLs.Reddit) > 0 {
		urls["reddit"] = token.URLs.Reddit
	}
	if len(token.URLs.Chat) > 0 {
		urls["chat"] = token.URLs.Chat
	}
	if len(token.URLs.Announcement) > 0 {
		urls["announcement"] = token.URLs.Announcement
	}
	if len(token.URLs.Twitter) > 0 {
		urls["twitter"] = token.URLs.Twitter
	}

	if len(urls) > 0 {
		metadata["urls"] = urls
	}

	// Add other metadata
	metadata["slug"] = token.Slug
	metadata["is_infinite_max_supply"] = token.IsInfiniteMaxSupply == 1

	return metadata
}