package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

// Database models
type Token struct {
	ID       int                    `json:"id"`
	Symbol   string                 `json:"symbol"`
	Name     string                 `json:"name"`
	Metadata map[string]interface{} `json:"metadata"`
}

// Exchange data structures
type ExchangeData struct {
	Data struct {
		Name        string       `json:"name"`
		Slug        string       `json:"slug"`
		MarketPairs []MarketPair `json:"marketPairs"`
	} `json:"data"`
}

type MarketPair struct {
	BaseSymbol       string  `json:"baseSymbol"`
	BaseCurrencyName string  `json:"baseCurrencyName"`
	BaseCurrencySlug string  `json:"baseCurrencySlug"`
	BaseCurrencyID   int     `json:"baseCurrencyId"`
	QuoteSymbol      string  `json:"quoteSymbol"`
	QuoteCurrencyID  int     `json:"quoteCurrencyId"`
	QuoteCurrencySlug string `json:"quoteCurrencySlug"`
	MarketPair       string  `json:"marketPair"`
	Price            float64 `json:"price"`
	VolumeUSD        float64 `json:"volumeUsd"`
	ExchangeName     string  // Added during processing
	ExchangeSlug     string  // Added during processing
	SourceFile       string  // Added during processing
}

// Processing results
type ProcessingResult struct {
	File         string    `json:"file"`
	Success      bool      `json:"success"`
	ExchangeName string    `json:"exchange_name"`
	ExchangeSlug string    `json:"exchange_slug"`
	PairsLoaded  int       `json:"pairs_loaded"`
	Error        string    `json:"error,omitempty"`
	Timestamp    time.Time `json:"timestamp"`
}

// Mapping structures
type TokenMapping struct {
	ExchangeName    string `json:"exchange_name"`
	ExchangeSlug    string `json:"exchange_slug"`
	Symbol          string `json:"symbol"`
	Name            string `json:"name"`
	Slug            string `json:"slug"`
	DatabaseTokenID int    `json:"database_token_id"`
	MarketPair      string `json:"market_pair"`
	SourceFile      string `json:"source_file"`
}

type ExchangeInfo struct {
	ExchangeName string `json:"exchange_name"`
	ExchangeSlug string `json:"exchange_slug"`
	Symbol       string `json:"symbol"`
	MarketPair   string `json:"market_pair"`
}

type TokenInfo struct {
	DatabaseTokenID int    `json:"database_token_id"`
	Symbol          string `json:"symbol"`
	Name            string `json:"name"`
	Slug            string `json:"slug"`
	MarketPair      string `json:"market_pair"`
}

type UnmappedToken struct {
	Slug       string `json:"slug"`
	Symbol     string `json:"symbol"`
	Name       string `json:"name"`
	MarketPair string `json:"market_pair"`
}

type MappingData struct {
	AllMappings        []TokenMapping             `json:"all_mappings"`
	TokenToExchanges   map[int][]ExchangeInfo     `json:"token_to_exchanges"`
	ExchangeToTokens   map[string][]TokenInfo     `json:"exchange_to_tokens"`
	UnmappedByExchange map[string][]UnmappedToken `json:"unmapped_by_exchange"`
	Statistics         map[string]int             `json:"statistics"`
}

type ComprehensiveResult struct {
	ProcessingSummary struct {
		Timestamp         time.Time          `json:"timestamp"`
		FilesProcessed    int                `json:"files_processed"`
		SuccessfulFiles   int                `json:"successful_files"`
		FailedFiles       int                `json:"failed_files"`
		ProcessingDetails []ProcessingResult `json:"processing_details"`
	} `json:"processing_summary"`
	MappingStatistics   map[string]int                    `json:"mapping_statistics"`
	TokenCoverage       map[string]map[string]interface{} `json:"token_coverage"`
	MultiExchangeTokens map[string]map[string]interface{} `json:"multi_exchange_tokens"`
	AllMappings         []TokenMapping                    `json:"all_mappings"`
	UnmappedTokens      map[string][]UnmappedToken        `json:"unmapped_tokens"`
}

// Helper function to get environment variables with default
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// Database connection
func connectDatabase(host, dbname, user, password, port string) (*sql.DB, error) {
	psqlInfo := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	db, err := sql.Open("postgres", psqlInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %v", err)
	}

	err = db.Ping()
	if err != nil {
		return nil, fmt.Errorf("failed to ping database: %v", err)
	}

	log.Println("Database connection successful")
	return db, nil
}

// Get all tokens from database (symbol -> ID mapping)
func getAllTokens(db *sql.DB) (map[string]int, error) {
	query := `
		SELECT id, symbol 
		FROM tokens 
		WHERE is_active = true
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query tokens: %v", err)
	}
	defer rows.Close()

	symbolToID := make(map[string]int)

	for rows.Next() {
		var id int
		var symbol string

		err := rows.Scan(&id, &symbol)
		if err != nil {
			log.Printf("Error scanning row: %v", err)
			continue
		}

		symbolToID[strings.ToUpper(symbol)] = id
	}

	log.Printf("Loaded %d tokens from database", len(symbolToID))
	return symbolToID, nil
}

// Extract quote symbol from market pair
func extractQuoteSymbol(marketPair, baseSymbol string) string {
	// Remove base symbol from the market pair to get quote
	pair := strings.ToUpper(marketPair)
	base := strings.ToUpper(baseSymbol)
	
	// Common quote currencies to check
	commonQuotes := []string{"USDT", "USDC", "USD", "BTC", "ETH", "BNB", "BUSD", "EUR", "GBP", "TRY", "BRL"}
	
	// Try to find quote currency at the end of the pair
	for _, quote := range commonQuotes {
		if strings.HasSuffix(pair, quote) && strings.HasPrefix(pair, base) {
			return quote
		}
	}
	
	// If no common quote found, try to extract by removing base
	if strings.HasPrefix(pair, base) {
		return strings.TrimPrefix(pair, base)
	}
	
	return ""
}

// Get tokens by slug from database
func getTokensBySlug(db *sql.DB) (map[string]int, error) {
	query := `
		SELECT id, symbol, name, metadata
		FROM tokens 
		WHERE is_active = true 
		AND metadata IS NOT NULL
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query tokens: %v", err)
	}
	defer rows.Close()

	slugToID := make(map[string]int)
	tokenCount := 0

	for rows.Next() {
		var token Token
		var metadataJSON []byte

		err := rows.Scan(&token.ID, &token.Symbol, &token.Name, &metadataJSON)
		if err != nil {
			log.Printf("Error scanning row: %v", err)
			continue
		}

		tokenCount++

		// Parse metadata
		if err := json.Unmarshal(metadataJSON, &token.Metadata); err != nil {
			log.Printf("Error parsing metadata for token %s: %v", token.Symbol, err)
			continue
		}

		// Try to extract slug from metadata
		var slug string
		if s, ok := token.Metadata["slug"].(string); ok && s != "" {
			slug = s
		} else if s, ok := token.Metadata["coinmarketcap_slug"].(string); ok && s != "" {
			slug = s
		} else if s, ok := token.Metadata["coingecko_id"].(string); ok && s != "" {
			slug = s
		}

		if slug != "" {
			slugToID[slug] = token.ID
			// Only log first 20 to avoid clutter
			if len(slugToID) <= 20 {
				log.Printf("Token %s (ID: %d) - slug: %s", token.Symbol, token.ID, slug)
			}
		}
	}

	log.Printf("Loaded %d tokens with slug out of %d total tokens", len(slugToID), tokenCount)
	return slugToID, nil
}

// Find all exchange folders
func findExchangeFolders(rootPath string) ([]string, error) {
	var exchangeFolders []string

	// List of exchange folder names based on your structure
	exchangeNames := []string{
		"1binance", "2bitget", "3bybit", "4okx", "5mexc", "6htx", "7cryptocom", "8kucoin",
		"9lbank", "10bitmart", "11deepcoin", "12kraken", "13gateio", "14gemini", "15coinbase",
		"16whitebit", "17biconomy", "18coinw", "19toobit", "20pionex", "21bitunix", "22bitstamp",
		"23hashkey", "24digifinex", "25digifinex", "26coinstore", "27bitrue", "28bigone",
		"29coinex", "30btse",
	}

	for _, exchangeName := range exchangeNames {
		folderPath := filepath.Join(rootPath, exchangeName)
		if info, err := os.Stat(folderPath); err == nil && info.IsDir() {
			exchangeFolders = append(exchangeFolders, folderPath)
		}
	}

	return exchangeFolders, nil
}

// Load exchange data with tracking
func loadExchangeDataWithTracking(exchangeFolders []string) ([]MarketPair, []ProcessingResult) {
	var allMarketPairs []MarketPair
	var processingResults []ProcessingResult

	for _, folderPath := range exchangeFolders {
		// Check for 1.json and 2.json in each folder
		for i := 1; i <= 2; i++ {
			jsonFile := filepath.Join(folderPath, fmt.Sprintf("%d.json", i))

			// Skip if file doesn't exist
			if _, err := os.Stat(jsonFile); os.IsNotExist(err) {
				continue
			}

			result := ProcessingResult{
				File:      jsonFile,
				Timestamp: time.Now(),
			}

			// Read file
			data, err := ioutil.ReadFile(jsonFile)
			if err != nil {
				result.Error = fmt.Sprintf("Failed to read file: %v", err)
				processingResults = append(processingResults, result)
				log.Printf("✗ Failed to read %s: %v", jsonFile, err)
				continue
			}

			// Parse JSON
			var exchangeData ExchangeData
			if err := json.Unmarshal(data, &exchangeData); err != nil {
				result.Error = fmt.Sprintf("Failed to parse JSON: %v", err)
				processingResults = append(processingResults, result)
				log.Printf("✗ Failed to parse %s: %v", jsonFile, err)
				continue
			}

			// Add exchange info to each pair
			for idx := range exchangeData.Data.MarketPairs {
				exchangeData.Data.MarketPairs[idx].ExchangeName = exchangeData.Data.Name
				exchangeData.Data.MarketPairs[idx].ExchangeSlug = exchangeData.Data.Slug
				exchangeData.Data.MarketPairs[idx].SourceFile = jsonFile
			}

			allMarketPairs = append(allMarketPairs, exchangeData.Data.MarketPairs...)

			// Update result
			result.Success = true
			result.ExchangeName = exchangeData.Data.Name
			result.ExchangeSlug = exchangeData.Data.Slug
			result.PairsLoaded = len(exchangeData.Data.MarketPairs)

			processingResults = append(processingResults, result)
			log.Printf("✓ Loaded %d market pairs from %s (%s)",
				len(exchangeData.Data.MarketPairs), exchangeData.Data.Name, jsonFile)
		}
	}

	log.Printf("Total loaded market pairs: %d", len(allMarketPairs))
	return allMarketPairs, processingResults
}

// Map tokens with relationships
func mapTokensWithRelationships(marketPairs []MarketPair, slugToID map[string]int) *MappingData {
	mappingData := &MappingData{
		AllMappings:        []TokenMapping{},
		TokenToExchanges:   make(map[int][]ExchangeInfo),
		ExchangeToTokens:   make(map[string][]TokenInfo),
		UnmappedByExchange: make(map[string][]UnmappedToken),
		Statistics:         make(map[string]int),
	}

	for _, pair := range marketPairs {
		if tokenID, exists := slugToID[pair.BaseCurrencySlug]; exists {
			// Create mapping entry
			tokenMapping := TokenMapping{
				ExchangeName:    pair.ExchangeName,
				ExchangeSlug:    pair.ExchangeSlug,
				Symbol:          pair.BaseSymbol,
				Name:            pair.BaseCurrencyName,
				Slug:            pair.BaseCurrencySlug,
				DatabaseTokenID: tokenID,
				MarketPair:      pair.MarketPair,
				SourceFile:      pair.SourceFile,
			}

			mappingData.AllMappings = append(mappingData.AllMappings, tokenMapping)

			// Track token -> exchanges relationship
			exchangeInfo := ExchangeInfo{
				ExchangeName: pair.ExchangeName,
				ExchangeSlug: pair.ExchangeSlug,
				Symbol:       pair.BaseSymbol,
				MarketPair:   pair.MarketPair,
			}
			mappingData.TokenToExchanges[tokenID] = append(mappingData.TokenToExchanges[tokenID], exchangeInfo)

			// Track exchange -> tokens relationship
			tokenInfo := TokenInfo{
				DatabaseTokenID: tokenID,
				Symbol:          pair.BaseSymbol,
				Name:            pair.BaseCurrencyName,
				Slug:            pair.BaseCurrencySlug,
				MarketPair:      pair.MarketPair,
			}
			mappingData.ExchangeToTokens[pair.ExchangeName] = append(mappingData.ExchangeToTokens[pair.ExchangeName], tokenInfo)

			log.Printf("Mapped: %s -> DB ID %d on %s", pair.BaseCurrencySlug, tokenID, pair.ExchangeName)
		} else {
			// Track unmapped tokens
			unmapped := UnmappedToken{
				Slug:       pair.BaseCurrencySlug,
				Symbol:     pair.BaseSymbol,
				Name:       pair.BaseCurrencyName,
				MarketPair: pair.MarketPair,
			}
			mappingData.UnmappedByExchange[pair.ExchangeName] = append(mappingData.UnmappedByExchange[pair.ExchangeName], unmapped)
		}
	}

	// Update statistics
	mappingData.Statistics["total_mappings"] = len(mappingData.AllMappings)
	mappingData.Statistics["unique_tokens"] = len(mappingData.TokenToExchanges)
	mappingData.Statistics["exchanges_processed"] = len(mappingData.ExchangeToTokens)

	log.Printf("Mapping completed: %d total mappings", len(mappingData.AllMappings))
	return mappingData
}

// Save comprehensive results
func saveComprehensiveResults(mappingData *MappingData, processingResults []ProcessingResult, outputFile string) error {
	result := ComprehensiveResult{}

	// Processing summary
	result.ProcessingSummary.Timestamp = time.Now()
	result.ProcessingSummary.FilesProcessed = len(processingResults)
	result.ProcessingSummary.ProcessingDetails = processingResults

	for _, pr := range processingResults {
		if pr.Success {
			result.ProcessingSummary.SuccessfulFiles++
		} else {
			result.ProcessingSummary.FailedFiles++
		}
	}

	// Mapping statistics
	result.MappingStatistics = mappingData.Statistics

	// Token coverage analysis
	result.TokenCoverage = make(map[string]map[string]interface{})
	result.MultiExchangeTokens = make(map[string]map[string]interface{})

	for tokenID, exchanges := range mappingData.TokenToExchanges {
		exchangeNames := []string{}
		for _, ex := range exchanges {
			exchangeNames = append(exchangeNames, ex.ExchangeName)
		}

		tokenIDStr := fmt.Sprintf("%d", tokenID)
		result.TokenCoverage[tokenIDStr] = map[string]interface{}{
			"exchange_count": len(exchangeNames),
			"exchanges":      exchangeNames,
		}

		if len(exchangeNames) > 1 {
			result.MultiExchangeTokens[tokenIDStr] = result.TokenCoverage[tokenIDStr]
		}
	}

	// All mappings and unmapped tokens
	result.AllMappings = mappingData.AllMappings
	result.UnmappedTokens = mappingData.UnmappedByExchange

	// Save to file
	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal result: %v", err)
	}

	err = ioutil.WriteFile(outputFile, jsonData, 0644)
	if err != nil {
		return fmt.Errorf("failed to write file: %v", err)
	}

	log.Printf("Comprehensive results saved to %s", outputFile)
	return nil
}

// Print enhanced summary
func printEnhancedSummary(mappingData *MappingData, processingResults []ProcessingResult) {
	fmt.Println(strings.Repeat("=", 80))
	fmt.Println("Multi-Exchange Token Mapping Results")
	fmt.Println(strings.Repeat("=", 80))

	// File processing summary
	fmt.Println("\nFile Processing Status:")
	fmt.Printf("%-50s %-10s %-20s %-10s %s\n", "File", "Status", "Exchange", "Pairs", "Error")
	fmt.Println(strings.Repeat("-", 80))

	for _, result := range processingResults {
		status := "✓ Success"
		if !result.Success {
			status = "✗ Failed"
		}
		exchange := result.ExchangeName
		if exchange == "" {
			exchange = "N/A"
		}
		error := result.Error
		if len(error) > 30 {
			error = error[:30] + "..."
		}

		// Extract just the filename for display
		fileName := filepath.Base(result.File)
		folderName := filepath.Base(filepath.Dir(result.File))
		displayPath := fmt.Sprintf("%s/%s", folderName, fileName)

		fmt.Printf("%-50s %-10s %-20s %-10d %s\n",
			displayPath, status, exchange, result.PairsLoaded, error)
	}

	// Overall statistics
	fmt.Printf("\nOverall Statistics:\n")
	fmt.Printf("  - Total mappings: %d\n", mappingData.Statistics["total_mappings"])
	fmt.Printf("  - Unique tokens: %d\n", mappingData.Statistics["unique_tokens"])
	fmt.Printf("  - Exchanges processed: %d\n", mappingData.Statistics["exchanges_processed"])

	// Token distribution by exchange
	fmt.Println("\nTokens by exchange:")
	for exchange, tokens := range mappingData.ExchangeToTokens {
		fmt.Printf("  - %s: %d tokens\n", exchange, len(tokens))
	}

	// Multi-exchange tokens
	multiExchangeCount := 0
	for _, exchanges := range mappingData.TokenToExchanges {
		if len(exchanges) > 1 {
			multiExchangeCount++
		}
	}

	fmt.Printf("\nTokens on multiple exchanges: %d tokens\n", multiExchangeCount)

	// Show first few multi-exchange tokens
	count := 0
	for tokenID, exchanges := range mappingData.TokenToExchanges {
		if len(exchanges) > 1 && count < 10 {
			exchangeNames := []string{}
			symbol := exchanges[0].Symbol
			for _, ex := range exchanges {
				exchangeNames = append(exchangeNames, ex.ExchangeName)
			}
			fmt.Printf("  - %s (ID: %d): %s\n", symbol, tokenID, strings.Join(exchangeNames, ", "))
			count++
		}
	}

	if multiExchangeCount > 10 {
		fmt.Printf("  ... and %d more\n", multiExchangeCount-10)
	}

	// Unmapped tokens summary
	fmt.Println("\nUnmapped tokens by exchange:")
	for exchange, unmapped := range mappingData.UnmappedByExchange {
		if len(unmapped) > 0 {
			fmt.Printf("  - %s: %d unmapped tokens\n", exchange, len(unmapped))
		}
	}
}

// Save mappings to database
func saveMappingsToDatabase(db *sql.DB, marketPairs []MarketPair, slugToID map[string]int) error {
	// First, we need to get all tokens including quote currencies
	allTokens, err := getAllTokens(db)
	if err != nil {
		return fmt.Errorf("failed to get all tokens: %v", err)
	}

	// Prepare the insert statement for trading_pairs
	insertQuery := `
		INSERT INTO trading_pairs (
			base_token_id, quote_token_id,
			exchange_id, exchange_pair_symbol,
			is_active, last_volume_24h,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (exchange_id, exchange_pair_symbol) 
		DO UPDATE SET 
			last_volume_24h = EXCLUDED.last_volume_24h,
			updated_at = NOW()
	`

	stmt, err := db.Prepare(insertQuery)
	if err != nil {
		return fmt.Errorf("failed to prepare insert statement: %v", err)
	}
	defer stmt.Close()

	successCount := 0
	failCount := 0
	skipCount := 0

	for _, pair := range marketPairs {
		// Get base token ID
		baseTokenID, baseExists := allTokens[strings.ToUpper(pair.BaseSymbol)]
		if !baseExists {
			// Try with slug
			baseTokenID, baseExists = slugToID[pair.BaseCurrencySlug]
		}

		// Get quote token ID - use the quote symbol from JSON
		quoteTokenID, quoteExists := allTokens[strings.ToUpper(pair.QuoteSymbol)]
		if !quoteExists && pair.QuoteCurrencySlug != "" {
			// Try with slug
			quoteTokenID, quoteExists = slugToID[pair.QuoteCurrencySlug]
		}

		// Skip if we can't find both tokens
		if !baseExists || !quoteExists {
			if !baseExists {
				log.Printf("Skipping %s on %s: base token %s (slug: %s) not found", 
					pair.MarketPair, pair.ExchangeName, pair.BaseSymbol, pair.BaseCurrencySlug)
			}
			if !quoteExists {
				log.Printf("Skipping %s on %s: quote token %s (slug: %s) not found", 
					pair.MarketPair, pair.ExchangeName, pair.QuoteSymbol, pair.QuoteCurrencySlug)
			}
			skipCount++
			continue
		}

		// Create exchange ID from slug (remove spaces, lowercase)
		exchangeID := strings.ToLower(strings.ReplaceAll(pair.ExchangeSlug, " ", ""))

		// Execute insert
		_, err := stmt.Exec(
			baseTokenID,                   // base_token_id
			quoteTokenID,                  // quote_token_id
			exchangeID,                    // exchange_id
			pair.MarketPair,               // exchange_pair_symbol
			true,                          // is_active
			pair.VolumeUSD,                // last_volume_24h
			time.Now(),                    // created_at
			time.Now(),                    // updated_at
		)

		if err != nil {
			log.Printf("Failed to insert pair %s on %s: %v", pair.MarketPair, pair.ExchangeName, err)
			failCount++
		} else {
			successCount++
		}
	}

	log.Printf("Database save complete: %d successful, %d failed, %d skipped (missing tokens)", successCount, failCount, skipCount)
	return nil
}

// Find which exchanges have a specific token
func findTokenExchanges(mappingData *MappingData, tokenSymbol string) []string {
	exchangeMap := make(map[string]bool)

	for _, mapping := range mappingData.AllMappings {
		if mapping.Symbol == tokenSymbol {
			exchangeMap[mapping.ExchangeName] = true
		}
	}

	exchanges := []string{}
	for exchange := range exchangeMap {
		exchanges = append(exchanges, exchange)
	}

	return exchanges
}

func main() {
	// Database configuration - using environment variables or defaults
	dbConfig := struct {
		Host     string
		Database string
		User     string
		Password string
		Port     string
	}{
		Host:     getEnv("POSTGRES_HOST", "localhost"),
		Database: getEnv("POSTGRES_DB", "crypto_platform"),
		User:     getEnv("POSTGRES_USER", "crypto_user"),
		Password: getEnv("POSTGRES_PASSWORD", "crypto_password"),
		Port:     getEnv("POSTGRES_PORT", "5432"),
	}

	// Root path containing exchange folders
	rootPath := getEnv("EXCHANGE_DATA_PATH", "./cmd/mapper/coinmarketcap exchange")

	// Connect to database
	db, err := connectDatabase(dbConfig.Host, dbConfig.Database, dbConfig.User, dbConfig.Password, dbConfig.Port)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Get tokens by slug from database
	slugToID, err := getTokensBySlug(db)
	if err != nil {
		log.Fatal(err)
	}

	// Find all exchange folders
	exchangeFolders, err := findExchangeFolders(rootPath)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Found %d exchange folders", len(exchangeFolders))

	// Load exchange data with tracking
	marketPairs, processingResults := loadExchangeDataWithTracking(exchangeFolders)

	// Map tokens with relationship tracking
	mappingData := mapTokensWithRelationships(marketPairs, slugToID)

	// Save mappings to database
	log.Println("Saving mappings to database...")
	if err := saveMappingsToDatabase(db, marketPairs, slugToID); err != nil {
		log.Printf("Warning: Failed to save mappings to database: %v", err)
	}

	// Save comprehensive results
	if err := saveComprehensiveResults(mappingData, processingResults, "multi_exchange_mapping_results.json"); err != nil {
		log.Printf("Failed to save results: %v", err)
	}

	// Print enhanced summary
	printEnhancedSummary(mappingData, processingResults)

	// Example: Find which exchanges have specific tokens
	fmt.Println("\nExample - Finding exchanges for specific tokens:")
	for _, symbol := range []string{"BTC", "ETH", "HSK"} {
		exchanges := findTokenExchanges(mappingData, symbol)
		if len(exchanges) > 0 {
			fmt.Printf("  %s is available on: %s\n", symbol, strings.Join(exchanges, ", "))
		}
	}
}
