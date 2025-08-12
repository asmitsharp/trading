package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"log"
	"sort"
	"strings"

	_ "github.com/lib/pq"
)

type MappingResults struct {
	UnmappedTokens map[string][]UnmappedToken `json:"unmapped_tokens"`
	MappingStatistics struct {
		TotalPairs    int `json:"total_pairs"`
		MappedPairs   int `json:"mapped_pairs"`
		UnmappedPairs int `json:"unmapped_pairs"`
	} `json:"mapping_statistics"`
}

type UnmappedToken struct {
	Slug       string `json:"slug"`
	Symbol     string `json:"symbol"`
	Name       string `json:"name"`
	MarketPair string `json:"market_pair"`
}

type MissingToken struct {
	Symbol    string
	Slug      string
	Name      string
	Count     int
	Exchanges []string
	Type      string // categorization
	Priority  int    // 1=high, 2=medium, 3=low
}

func main() {
	// Read the mapping results file
	data, err := os.ReadFile("multi_exchange_mapping_results.json")
	if err != nil {
		log.Fatal("Error reading mapping results:", err)
	}

	var results MappingResults
	if err := json.Unmarshal(data, &results); err != nil {
		log.Fatal("Error parsing mapping results:", err)
	}

	// Extract missing tokens
	missingTokens := extractMissingTokens(results)

	// Connect to database to check which ones are truly missing
	db := connectDB()
	defer db.Close()

	// Filter out tokens that actually exist
	trulyMissing := filterExistingTokens(db, missingTokens)

	// Categorize tokens
	categorized := categorizeTokens(trulyMissing)

	// Output results
	outputResults(categorized)

	// Generate SQL insert statements
	generateSQL(categorized)
}

func connectDB() *sql.DB {
	dbURL := getEnv("DATABASE_URL", "postgresql://admin:password@localhost:5432/trading_app?sslmode=disable")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal("Error connecting to database:", err)
	}
	return db
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func extractMissingTokens(results MappingResults) map[string]*MissingToken {
	tokens := make(map[string]*MissingToken)

	// Process unmapped tokens from each exchange
	for exchangeName, unmappedList := range results.UnmappedTokens {
		for _, unmapped := range unmappedList {
			// Use symbol as the key
			key := unmapped.Symbol
			if key == "" {
				continue
			}

			if _, exists := tokens[key]; !exists {
				tokens[key] = &MissingToken{
					Symbol:    unmapped.Symbol,
					Slug:      unmapped.Slug,
					Name:      unmapped.Name,
					Count:     0,
					Exchanges: []string{},
				}
			}

			tokens[key].Count++

			// Update slug and name if empty
			if tokens[key].Slug == "" && unmapped.Slug != "" {
				tokens[key].Slug = unmapped.Slug
			}
			if tokens[key].Name == "" && unmapped.Name != "" {
				tokens[key].Name = unmapped.Name
			}

			// Add exchange if not already present
			exchangeFound := false
			for _, ex := range tokens[key].Exchanges {
				if ex == exchangeName {
					exchangeFound = true
					break
				}
			}
			if !exchangeFound {
				tokens[key].Exchanges = append(tokens[key].Exchanges, exchangeName)
			}
		}
	}

	return tokens
}

func filterExistingTokens(db *sql.DB, tokens map[string]*MissingToken) map[string]*MissingToken {
	filtered := make(map[string]*MissingToken)

	for symbol, token := range tokens {
		var exists bool
		// Check both by symbol and slug
		query := `
			SELECT EXISTS(
				SELECT 1 FROM tokens 
				WHERE symbol = $1 
				   OR (metadata->>'slug' = $2 AND $2 != '')
			)
		`
		err := db.QueryRow(query, symbol, token.Slug).Scan(&exists)
		if err != nil {
			log.Printf("Error checking token %s: %v", symbol, err)
			// Include it in case of error
			filtered[symbol] = token
			continue
		}

		if !exists {
			filtered[symbol] = token
		}
	}

	return filtered
}

func categorizeTokens(tokens map[string]*MissingToken) map[string]*MissingToken {
	for symbol, token := range tokens {
		// Categorize based on symbol patterns and frequency

		// High priority: Used frequently across multiple exchanges
		if token.Count > 100 || len(token.Exchanges) > 5 {
			token.Type = "high_volume"
			token.Priority = 1
		} else if isStablecoin(symbol) {
			token.Type = "stablecoin"
			token.Priority = 1
		} else if isWrappedToken(symbol) {
			token.Type = "wrapped"
			token.Priority = 2
		} else if isMemeToken(symbol, token.Name) {
			token.Type = "meme"
			token.Priority = 3
		} else if isDerivativeToken(symbol) {
			token.Type = "derivative"
			token.Priority = 3
		} else if isLeveragedToken(symbol) {
			token.Type = "leveraged"
			token.Priority = 3
		} else if token.Count > 50 {
			token.Type = "medium_volume"
			token.Priority = 2
		} else if token.Count > 20 {
			token.Type = "low_volume"
			token.Priority = 3
		} else {
			token.Type = "rare"
			token.Priority = 3
		}
	}

	return tokens
}

func isStablecoin(symbol string) bool {
	stablecoins := []string{
		"USDT", "USDC", "BUSD", "DAI", "TUSD", "USDP", "UST", "GUSD", "HUSD", 
		"PAX", "SUSD", "LUSD", "CUSD", "RSV", "USDD", "USDN", "USTC", "USDX", 
		"MUSD", "DUSD", "FRAX", "USDB", "PYUSD", "FDUSD", "EURC", "EURC", "GYEN",
		"XSGD", "IDRT", "BIDR", "BRLD", "NGNT", "TRYB", "DKKT", "EURT", "EURS",
	}
	symbol = strings.ToUpper(symbol)
	for _, stable := range stablecoins {
		if symbol == stable {
			return true
		}
	}
	return false
}

func isWrappedToken(symbol string) bool {
	symbol = strings.ToUpper(symbol)
	// Common wrapped token patterns
	if strings.HasPrefix(symbol, "W") && len(symbol) > 3 {
		// Check for common wrapped tokens like WBTC, WETH, WBNB
		base := symbol[1:]
		if base == "BTC" || base == "ETH" || base == "BNB" || base == "AVAX" || base == "MATIC" {
			return true
		}
	}
	// Staked/liquid staking tokens
	if strings.HasPrefix(symbol, "ST") || strings.HasPrefix(symbol, "S") {
		base := symbol[2:]
		if len(symbol) > 2 && (base == "ETH" || base == "SOL" || base == "ATOM") {
			return true
		}
	}
	// Check for other patterns
	return strings.Contains(symbol, "WRAPPED") || 
		   strings.HasPrefix(symbol, "C") && len(symbol) > 3 || // cTokens
		   strings.HasPrefix(symbol, "A") && len(symbol) > 3 || // aTokens
		   strings.HasPrefix(symbol, "X") && len(symbol) > 3    // xTokens
}

func isMemeToken(symbol, name string) bool {
	memePatterns := []string{
		"DOGE", "SHIB", "PEPE", "FLOKI", "MEME", "ELON", "MOON", "SAFE", 
		"BABY", "INU", "AKITA", "KISHU", "HOKK", "PIG", "ASS", "CUMROCKET",
		"WOJAK", "BONK", "WIF", "MYRO", "PONKE", "SMOG", "SNEK", "WEN",
		"BOME", "SLERF", "PUMP", "CAT", "NEIRO", "TURBO", "LADYS", "AIDOGE",
		"PEPECOIN", "BOBO", "MONG", "TSUKA", "VOLT", "BONE", "LEASH", "CAW",
	}
	
	symbolUpper := strings.ToUpper(symbol)
	nameUpper := strings.ToUpper(name)
	
	for _, pattern := range memePatterns {
		if strings.Contains(symbolUpper, pattern) || strings.Contains(nameUpper, pattern) {
			return true
		}
	}
	
	// Check name for meme-related terms
	if strings.Contains(nameUpper, "MEME") || strings.Contains(nameUpper, "JOKE") ||
	   strings.Contains(nameUpper, "MOON") || strings.Contains(nameUpper, "ROCKET") {
		return true
	}
	
	return false
}

func isDerivativeToken(symbol string) bool {
	// Perpetual and futures tokens
	return strings.HasSuffix(symbol, "PERP") || strings.Contains(symbol, "PERP") ||
		   strings.HasSuffix(symbol, "-P") || strings.HasSuffix(symbol, "-C")
}

func isLeveragedToken(symbol string) bool {
	// Leveraged tokens (3x, 5x, etc.)
	return strings.HasSuffix(symbol, "UP") || strings.HasSuffix(symbol, "DOWN") ||
		   strings.HasSuffix(symbol, "BULL") || strings.HasSuffix(symbol, "BEAR") ||
		   strings.Contains(symbol, "3L") || strings.Contains(symbol, "3S") || 
		   strings.Contains(symbol, "5L") || strings.Contains(symbol, "5S") ||
		   strings.Contains(symbol, "2L") || strings.Contains(symbol, "2S") ||
		   strings.Contains(symbol, "10L") || strings.Contains(symbol, "10S")
}

func outputResults(tokens map[string]*MissingToken) {
	// Convert map to slice for sorting
	var tokenList []*MissingToken
	for _, token := range tokens {
		tokenList = append(tokenList, token)
	}

	// Sort by priority and count
	sort.Slice(tokenList, func(i, j int) bool {
		if tokenList[i].Priority != tokenList[j].Priority {
			return tokenList[i].Priority < tokenList[j].Priority
		}
		return tokenList[i].Count > tokenList[j].Count
	})

	// Output categorized results
	fmt.Println("\n=== MISSING TOKENS ANALYSIS ===\n")
	fmt.Printf("Total missing tokens: %d\n", len(tokenList))
	fmt.Printf("Total unmapped occurrences: %d\n\n", sumCounts(tokenList))

	categories := map[string][]*MissingToken{
		"high_volume": {},
		"stablecoin":  {},
		"wrapped":     {},
		"derivative":  {},
		"leveraged":   {},
		"medium_volume": {},
		"meme":        {},
		"low_volume":  {},
		"rare":        {},
	}

	for _, token := range tokenList {
		categories[token.Type] = append(categories[token.Type], token)
	}

	// Print each category
	categoryNames := map[string]string{
		"high_volume":   "HIGH VOLUME TOKENS (>100 occurrences or >5 exchanges)",
		"stablecoin":    "STABLECOINS",
		"wrapped":       "WRAPPED/BRIDGED TOKENS",
		"derivative":    "DERIVATIVE TOKENS",
		"leveraged":     "LEVERAGED TOKENS (3x, 5x, etc.)",
		"medium_volume": "MEDIUM VOLUME TOKENS (>50 occurrences)",
		"meme":          "MEME TOKENS",
		"low_volume":    "LOW VOLUME TOKENS (>20 occurrences)",
		"rare":          "RARE TOKENS (<20 occurrences)",
	}

	for _, catKey := range []string{"high_volume", "stablecoin", "wrapped", "medium_volume", "derivative", "leveraged", "meme", "low_volume", "rare"} {
		catName := categoryNames[catKey]
		if len(categories[catKey]) > 0 {
			fmt.Printf("\n--- %s ---\n", catName)
			fmt.Printf("Total: %d tokens\n\n", len(categories[catKey]))
			fmt.Printf("%-12s %-30s %-10s %-12s %s\n", "Symbol", "Name", "Count", "Exchanges", "Exchange List")
			fmt.Println(strings.Repeat("-", 100))

			// Show top tokens for each category
			limit := 20
			if catKey == "high_volume" || catKey == "stablecoin" {
				limit = 50 // Show more for important categories
			}
			if len(categories[catKey]) < limit {
				limit = len(categories[catKey])
			}

			for i := 0; i < limit; i++ {
				token := categories[catKey][i]
				name := token.Name
				if name == "" {
					name = token.Slug
				}
				if len(name) > 28 {
					name = name[:25] + "..."
				}
				
				exchangeList := strings.Join(token.Exchanges, ", ")
				if len(exchangeList) > 25 {
					exchangeList = exchangeList[:22] + "..."
				}
				
				fmt.Printf("%-12s %-30s %-10d %-12d %s\n",
					token.Symbol,
					name,
					token.Count,
					len(token.Exchanges),
					exchangeList)
			}

			if len(categories[catKey]) > limit {
				fmt.Printf("... and %d more\n", len(categories[catKey])-limit)
			}
		}
	}

	// Summary statistics
	fmt.Println("\n=== SUMMARY ===")
	fmt.Printf("Total missing tokens: %d\n", len(tokenList))
	fmt.Printf("High priority (Priority 1): %d\n", countByPriority(tokenList, 1))
	fmt.Printf("Medium priority (Priority 2): %d\n", countByPriority(tokenList, 2))  
	fmt.Printf("Low priority (Priority 3): %d\n", countByPriority(tokenList, 3))
}

func sumCounts(tokens []*MissingToken) int {
	total := 0
	for _, token := range tokens {
		total += token.Count
	}
	return total
}

func countByPriority(tokens []*MissingToken, priority int) int {
	count := 0
	for _, token := range tokens {
		if token.Priority == priority {
			count++
		}
	}
	return count
}

func generateSQL(tokens map[string]*MissingToken) {
	// Convert map to slice for sorting
	var tokenList []*MissingToken
	for _, token := range tokens {
		tokenList = append(tokenList, token)
	}

	// Sort by priority and count
	sort.Slice(tokenList, func(i, j int) bool {
		if tokenList[i].Priority != tokenList[j].Priority {
			return tokenList[i].Priority < tokenList[j].Priority
		}
		return tokenList[i].Count > tokenList[j].Count
	})

	// Generate SQL files for different priority levels
	generateSQLForPriority(tokenList, 1, "high_priority_tokens.sql")
	generateSQLForPriority(tokenList, 2, "medium_priority_tokens.sql")
	generateSQLForPriority(tokenList, 3, "low_priority_tokens.sql")

	fmt.Println("\nSQL files generated:")
	fmt.Println("- high_priority_tokens.sql (recommended to add)")
	fmt.Println("- medium_priority_tokens.sql (consider adding)")
	fmt.Println("- low_priority_tokens.sql (optional)")
}

func generateSQLForPriority(tokens []*MissingToken, priority int, filename string) {
	file, err := os.Create(filename)
	if err != nil {
		log.Printf("Error creating SQL file %s: %v", filename, err)
		return
	}
	defer file.Close()

	fmt.Fprintf(file, "-- Missing tokens with priority %d\n", priority)
	fmt.Fprintf(file, "-- Generated from exchange mapping analysis\n\n")
	fmt.Fprintf(file, "INSERT INTO tokens (symbol, name, slug, metadata, is_active, created_at, updated_at) VALUES\n")

	first := true
	for _, token := range tokens {
		if token.Priority != priority {
			continue
		}

		if !first {
			fmt.Fprintf(file, ",\n")
		}
		first = false

		name := token.Name
		if name == "" {
			name = token.Symbol + " Token"
		}
		// Escape single quotes in name
		name = strings.ReplaceAll(name, "'", "''")
		
		slug := token.Slug
		if slug == "" {
			slug = strings.ToLower(token.Symbol)
		}

		metadata := fmt.Sprintf(`{"type": "%s", "exchange_count": %d, "occurrence_count": %d}`,
			token.Type, len(token.Exchanges), token.Count)

		fmt.Fprintf(file, "    ('%s', '%s', '%s', '%s', true, NOW(), NOW())",
			token.Symbol, name, slug, metadata)
	}

	if !first {
		fmt.Fprintf(file, "\nON CONFLICT (symbol) DO UPDATE\n")
		fmt.Fprintf(file, "SET\n")
		fmt.Fprintf(file, "    name = EXCLUDED.name,\n")
		fmt.Fprintf(file, "    slug = EXCLUDED.slug,\n")
		fmt.Fprintf(file, "    metadata = tokens.metadata || EXCLUDED.metadata,\n")
		fmt.Fprintf(file, "    updated_at = NOW();\n")
	}
}