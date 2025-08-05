package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"

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

// Database connection using environment variables

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Usage: go run main.go <json_file_path>")
	}

	jsonFilePath := os.Args[1]

	// Connect to database
	db, err := connectDB()
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	defer db.Close()

	// Create unique constraint on symbol only
	err = createUniqueConstraint(db)
	if err != nil {
		log.Printf("Warning: Could not create unique constraint: %v", err)
	}

	// Read and parse JSON file
	tokens, err := readTokensFromFile(jsonFilePath)
	if err != nil {
		log.Fatal("Failed to read tokens from file:", err)
	}

	// Seed tokens into database
	err = seedTokens(db, tokens)
	if err != nil {
		log.Fatal("Failed to seed tokens:", err)
	}

	fmt.Printf("Successfully processed %d tokens\n", len(tokens))
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
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

func createUniqueConstraint(db *sql.DB) error {
	// Create unique constraint on symbol only - one entry per token
	query := `CREATE UNIQUE INDEX IF NOT EXISTS idx_tokens_unique_symbol ON tokens(symbol)`

	_, err := db.Exec(query)
	if err != nil {
		return fmt.Errorf("error creating unique constraint: %v", err)
	}

	fmt.Println("✓ Ensured unique constraint on symbol exists")
	return nil
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

func seedTokens(db *sql.DB, tokens []TokenMetadata) error {
	// Begin transaction
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("error starting transaction: %v", err)
	}
	defer tx.Rollback()

	insertedCount := 0
	updatedCount := 0

	for _, token := range tokens {
		wasUpdate, err := insertToken(tx, token)
		if err != nil {
			return fmt.Errorf("error inserting token %s: %v", token.Symbol, err)
		}

		if wasUpdate {
			updatedCount++
		} else {
			insertedCount++
		}
	}

	// Commit transaction
	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("error committing transaction: %v", err)
	}

	fmt.Printf("✓ Inserted: %d new tokens, Updated: %d existing tokens\n", insertedCount, updatedCount)
	return nil
}

func insertToken(tx *sql.Tx, token TokenMetadata) (bool, error) {
	// Create comprehensive metadata that includes ALL information
	metadata := createTokenMetadata(token)
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return false, fmt.Errorf("error marshaling metadata: %v", err)
	}

	// Leave contract_address and chain as NULL since we store everything in metadata
	query := `
		INSERT INTO tokens (
			symbol, name, contract_address, chain,
			circulating_supply, total_supply, max_supply,
			metadata, is_active
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (symbol)
		DO UPDATE SET
			name = EXCLUDED.name,
			circulating_supply = EXCLUDED.circulating_supply,
			total_supply = EXCLUDED.total_supply,
			max_supply = EXCLUDED.max_supply,
			metadata = EXCLUDED.metadata,
			updated_at = NOW()
		WHERE tokens.symbol = EXCLUDED.symbol
	`

	var maxSupply *float64
	if token.MaxSupply != nil && token.IsInfiniteMaxSupply == 0 {
		maxSupply = token.MaxSupply
	}

	result, err := tx.Exec(query,
		token.Symbol,
		token.Name,
		nil, // contract_address stays NULL
		nil, // chain stays NULL
		token.CirculatingSupply,
		token.TotalSupply,
		maxSupply,
		string(metadataJSON),
		true,
	)

	if err != nil {
		return false, fmt.Errorf("error executing token insert: %v", err)
	}

	// Check if this was an update or insert
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("error getting rows affected: %v", err)
	}

	isUpdate := rowsAffected == 0 // ON CONFLICT DO UPDATE doesn't count as affected rows in some cases

	// Check if token already existed by trying to get the ID
	var existingID int
	checkQuery := `SELECT id FROM tokens WHERE symbol = $1`
	err = tx.QueryRow(checkQuery, token.Symbol).Scan(&existingID)
	isUpdate = (err == nil && existingID > 0)

	fmt.Printf("✓ %s: %s (%s) - %d contracts, %d URLs\n",
		map[bool]string{true: "Updated", false: "Inserted"}[isUpdate],
		token.Name,
		token.Symbol,
		len(token.Contracts),
		countURLs(token.URLs))

	// Log contract summary
	if len(token.Contracts) > 0 {
		fmt.Printf("  Contracts: ")
		for i, contract := range token.Contracts {
			if i > 0 {
				fmt.Printf(", ")
			}
			fmt.Printf("%s", contract.ContractPlatform)
		}
		fmt.Println()
	}

	return isUpdate, nil
}

func createTokenMetadata(token TokenMetadata) map[string]interface{} {
	metadata := make(map[string]interface{})

	// Add all URLs to metadata
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

	// Add all contracts to metadata
	if len(token.Contracts) > 0 {
		contracts := make([]map[string]interface{}, len(token.Contracts))
		for i, contract := range token.Contracts {
			contracts[i] = map[string]interface{}{
				"contract_address": contract.ContractAddress,
				"platform":         contract.ContractPlatform,
				"rpc_urls":         contract.ContractRpcURL,
				"number":           contract.No,
			}
		}
		metadata["contracts"] = contracts
	}

	// Add other token metadata
	metadata["slug"] = token.Slug
	metadata["is_infinite_max_supply"] = token.IsInfiniteMaxSupply == 1

	return metadata
}

func countURLs(urls TokenURLs) int {
	count := 0
	count += len(urls.Website)
	count += len(urls.TechnicalDoc)
	count += len(urls.Explorer)
	count += len(urls.SourceCode)
	count += len(urls.Reddit)
	count += len(urls.Chat)
	count += len(urls.Announcement)
	count += len(urls.Twitter)
	return count
}

// Helper function to display token information after seeding
func displayTokenInfo(db *sql.DB, symbol string) error {
	query := `
		SELECT 
			symbol, 
			name, 
			circulating_supply,
			total_supply,
			max_supply,
			jsonb_pretty(metadata) as metadata_json
		FROM tokens 
		WHERE symbol = $1
	`

	var tokenSymbol, tokenName string
	var circSupply, totalSupply sql.NullFloat64
	var maxSupply sql.NullFloat64
	var metadataJSON string

	err := db.QueryRow(query, symbol).Scan(
		&tokenSymbol, &tokenName, &circSupply, &totalSupply, &maxSupply, &metadataJSON)
	if err != nil {
		return fmt.Errorf("error querying token: %v", err)
	}

	fmt.Printf("\n=== %s (%s) ===\n", tokenName, tokenSymbol)
	fmt.Printf("Circulating Supply: %.0f\n", circSupply.Float64)
	fmt.Printf("Total Supply: %.0f\n", totalSupply.Float64)
	if maxSupply.Valid {
		fmt.Printf("Max Supply: %.0f\n", maxSupply.Float64)
	} else {
		fmt.Printf("Max Supply: Unlimited\n")
	}
	fmt.Printf("\nMetadata:\n%s\n", metadataJSON)

	return nil
}
