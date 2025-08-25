package main

import (
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ExchangeData represents data availability for an exchange/symbol combination
type ExchangeData struct {
	Exchange     string
	Symbol       string
	BaseSymbol   string
	QuoteSymbol  string
	FilePath     string
	EarliestDate time.Time
	LatestDate   time.Time
	RecordCount  int
	DataQuality  float64 // Percentage of non-zero volume records
}

// SymbolAnalysis represents the analysis for a specific symbol across exchanges
type SymbolAnalysis struct {
	Symbol           string
	BaseSymbol       string
	QuoteSymbol      string
	BestExchange     string
	EarliestDate     time.Time
	TotalExchanges   int
	ExchangeData     []ExchangeData
	RecommendedSource string
}

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Usage: go run main.go <csv_directory>")
	}

	csvDir := os.Args[1]
	
	fmt.Println("=== Historical Data Analysis ===")
	fmt.Printf("Analyzing CSV files in: %s\n\n", csvDir)

	// Parse all CSV files
	exchangeDataList, err := parseAllCSVFiles(csvDir)
	if err != nil {
		log.Fatal("Failed to parse CSV files:", err)
	}

	fmt.Printf("Found %d CSV files with valid data\n\n", len(exchangeDataList))

	// Group by symbol and analyze
	symbolAnalyses := analyzeBySymbol(exchangeDataList)

	// Generate reports
	generateDetailedReport(symbolAnalyses)
	generateSummaryReport(symbolAnalyses)
	generateRecommendations(symbolAnalyses)
}

// parseAllCSVFiles parses all CSV files in the directory
func parseAllCSVFiles(csvDir string) ([]ExchangeData, error) {
	var exchangeDataList []ExchangeData

	err := filepath.Walk(csvDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !strings.HasSuffix(strings.ToLower(path), ".csv") {
			return nil
		}

		fmt.Printf("Processing: %s\n", filepath.Base(path))
		
		exchangeDataArray, err := parseCSVFile(path)
		if err != nil {
			fmt.Printf("  Error: %v\n", err)
			return nil // Continue with other files
		}

		if len(exchangeDataArray) > 0 {
			exchangeDataList = append(exchangeDataList, exchangeDataArray...)
			fmt.Printf("  ✅ Found %d exchanges in file:\n", len(exchangeDataArray))
			for _, data := range exchangeDataArray {
				fmt.Printf("    %s: %d records (%s to %s, %.1f%% quality)\n", 
					data.Exchange, 
					data.RecordCount,
					data.EarliestDate.Format("2006-01-02"),
					data.LatestDate.Format("2006-01-02"),
					data.DataQuality)
			}
		}

		return nil
	})

	return exchangeDataList, err
}

// parseCSVFile parses a single CSV file and extracts metadata for all exchanges in that file
func parseCSVFile(csvPath string) ([]ExchangeData, error) {
	// Extract symbol from filename (e.g., 1INCHBTC.csv)
	filename := filepath.Base(csvPath)
	symbolFromFile := strings.TrimSuffix(filename, ".csv")
	
	// Open and read CSV file
	file, err := os.Open(csvPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	if len(records) < 2 {
		return nil, fmt.Errorf("empty or invalid CSV file")
	}

	// Group records by exchange
	exchangeRecords := make(map[string][]ExchangeRecord)

	// Skip header row and process data
	for _, record := range records[1:] {
		if len(record) < 7 {
			continue
		}

		// Parse datetime
		datetime, err := time.Parse("2006-01-02 15:04:05", record[0])
		if err != nil {
			continue
		}

		// Extract exchange and symbol from symbol column (e.g., "BINANCE:1INCHBTC")
		symbolParts := strings.Split(record[1], ":")
		if len(symbolParts) != 2 {
			continue
		}

		exchange := strings.ToUpper(symbolParts[0])
		symbol := symbolParts[1]

		// Check for valid volume (data quality indicator)
		hasValidVolume := len(record) > 6 && record[6] != "0" && record[6] != "0.0" && record[6] != ""

		exchangeRecords[exchange] = append(exchangeRecords[exchange], ExchangeRecord{
			DateTime:       datetime,
			Symbol:         symbol,
			HasValidVolume: hasValidVolume,
		})
	}

	// Convert to ExchangeData for each exchange
	var results []ExchangeData
	
	for exchange, records := range exchangeRecords {
		if len(records) == 0 {
			continue
		}

		// Calculate metrics for this exchange
		var earliestDate, latestDate time.Time
		validRecords := 0

		for i, record := range records {
			if i == 0 || record.DateTime.Before(earliestDate) {
				earliestDate = record.DateTime
			}
			if i == 0 || record.DateTime.After(latestDate) {
				latestDate = record.DateTime
			}
			if record.HasValidVolume {
				validRecords++
			}
		}

		// Parse symbol into base and quote
		baseSymbol, quoteSymbol := parseSymbolPair(records[0].Symbol)

		// Calculate data quality
		dataQuality := float64(validRecords) / float64(len(records)) * 100

		results = append(results, ExchangeData{
			Exchange:     exchange,
			Symbol:       records[0].Symbol,
			BaseSymbol:   baseSymbol,
			QuoteSymbol:  quoteSymbol,
			FilePath:     csvPath,
			EarliestDate: earliestDate,
			LatestDate:   latestDate,
			RecordCount:  len(records),
			DataQuality:  dataQuality,
		})
	}

	return results, nil
}

// ExchangeRecord represents a single record for grouping
type ExchangeRecord struct {
	DateTime       time.Time
	Symbol         string
	HasValidVolume bool
}

// parseSymbolPair splits a symbol into base and quote currencies
func parseSymbolPair(symbol string) (base, quote string) {
	// Common quote currencies (longest first)
	quoteCurrencies := []string{
		"USDT", "USDC", "BUSD", "TUSD", "FDUSD", "DAI",
		"BTC", "ETH", "BNB", "XRP", "ADA", "DOT", "SOL",
		"EUR", "USD", "GBP", "JPY", "KRW", "CNY", "CAD", "AUD",
		"TRY", "BRL", "UAH", "ZAR", "NGN", "INR", "MXN", "ARS",
	}

	for _, quoteCurrency := range quoteCurrencies {
		if strings.HasSuffix(symbol, quoteCurrency) {
			potentialBase := strings.TrimSuffix(symbol, quoteCurrency)
			if len(potentialBase) > 0 {
				return potentialBase, quoteCurrency
			}
		}
	}

	// Fallback
	if len(symbol) > 6 {
		return symbol[:len(symbol)-4], symbol[len(symbol)-4:]
	}
	if len(symbol) > 3 {
		return symbol[:len(symbol)-3], symbol[len(symbol)-3:]
	}
	
	return symbol, "USD"
}

// analyzeBySymbol groups exchange data by symbol and finds the best source
func analyzeBySymbol(exchangeDataList []ExchangeData) []SymbolAnalysis {
	// Group by symbol
	symbolGroups := make(map[string][]ExchangeData)
	
	for _, data := range exchangeDataList {
		// Use base/quote pair as key for proper grouping
		key := fmt.Sprintf("%s/%s", data.BaseSymbol, data.QuoteSymbol)
		symbolGroups[key] = append(symbolGroups[key], data)
	}

	var analyses []SymbolAnalysis

	for symbolKey, exchangeDataGroup := range symbolGroups {
		parts := strings.Split(symbolKey, "/")
		baseSymbol := parts[0]
		quoteSymbol := parts[1]

		// Find the exchange with the earliest data
		var bestExchange string
		var earliestDate time.Time
		var recommendedSource string

		// Sort by earliest date
		sort.Slice(exchangeDataGroup, func(i, j int) bool {
			return exchangeDataGroup[i].EarliestDate.Before(exchangeDataGroup[j].EarliestDate)
		})

		if len(exchangeDataGroup) > 0 {
			bestData := exchangeDataGroup[0]
			bestExchange = bestData.Exchange
			earliestDate = bestData.EarliestDate

			// Determine recommended source based on multiple factors
			recommendedSource = determineRecommendedSource(exchangeDataGroup)
		}

		analyses = append(analyses, SymbolAnalysis{
			Symbol:           symbolKey,
			BaseSymbol:       baseSymbol,
			QuoteSymbol:      quoteSymbol,
			BestExchange:     bestExchange,
			EarliestDate:     earliestDate,
			TotalExchanges:   len(exchangeDataGroup),
			ExchangeData:     exchangeDataGroup,
			RecommendedSource: recommendedSource,
		})
	}

	// Sort analyses by symbol
	sort.Slice(analyses, func(i, j int) bool {
		return analyses[i].Symbol < analyses[j].Symbol
	})

	return analyses
}

// determineRecommendedSource chooses the best exchange considering multiple factors
func determineRecommendedSource(exchangeData []ExchangeData) string {
	if len(exchangeData) == 0 {
		return ""
	}

	// Exchange reliability scores (higher is better)
	reliability := map[string]int{
		"COINBASE": 100,
		"BINANCE":  95,
		"KRAKEN":   90,
		"OKX":      85,
		"BYBIT":    80,
		"BITGET":   75,
		"GATEIO":   70,
		"GATE.IO":  70,
		"MEXC":     65,
		"KUCOIN":   70,
		"HUOBI":    75,
		"HTX":      75,
	}

	bestScore := -1
	bestExchange := exchangeData[0].Exchange

	for _, data := range exchangeData {
		score := 0
		
		// Factor 1: Exchange reliability (40% weight)
		if rel, exists := reliability[data.Exchange]; exists {
			score += rel * 4 / 10
		} else {
			score += 20 // Default for unknown exchanges
		}
		
		// Factor 2: Data history length (30% weight)
		daysSinceEarliest := int(time.Since(data.EarliestDate).Hours() / 24)
		historyScore := daysSinceEarliest / 10 // 1 point per 10 days
		if historyScore > 30 {
			historyScore = 30 // Cap at 30 points
		}
		score += historyScore * 3 / 10

		// Factor 3: Data quality (20% weight)
		qualityScore := int(data.DataQuality * 20 / 100)
		score += qualityScore

		// Factor 4: Record count (10% weight)
		recordScore := data.RecordCount / 100 // 1 point per 100 records
		if recordScore > 10 {
			recordScore = 10 // Cap at 10 points
		}
		score += recordScore

		if score > bestScore {
			bestScore = score
			bestExchange = data.Exchange
		}
	}

	return bestExchange
}

// generateDetailedReport generates a detailed analysis report
func generateDetailedReport(analyses []SymbolAnalysis) {
	fmt.Println("\n=== DETAILED ANALYSIS REPORT ===")
	fmt.Println()

	for _, analysis := range analyses {
		fmt.Printf("Symbol: %s (%s/%s)\n", analysis.Symbol, analysis.BaseSymbol, analysis.QuoteSymbol)
		fmt.Printf("  Available on %d exchanges\n", analysis.TotalExchanges)
		fmt.Printf("  Earliest data: %s (%s)\n", 
			analysis.EarliestDate.Format("2006-01-02"), analysis.BestExchange)
		fmt.Printf("  Recommended source: %s\n", analysis.RecommendedSource)
		
		fmt.Println("  Exchange details:")
		for _, data := range analysis.ExchangeData {
			daysSince := int(time.Since(data.EarliestDate).Hours() / 24)
			fmt.Printf("    %s: %s to %s (%d days, %d records, %.1f%% quality)\n",
				data.Exchange,
				data.EarliestDate.Format("2006-01-02"),
				data.LatestDate.Format("2006-01-02"),
				daysSince,
				data.RecordCount,
				data.DataQuality)
		}
		fmt.Println()
	}
}

// generateSummaryReport generates a summary report
func generateSummaryReport(analyses []SymbolAnalysis) {
	fmt.Println("=== SUMMARY REPORT ===")
	fmt.Println()

	// Count by exchange
	exchangeCounts := make(map[string]int)
	recommendedCounts := make(map[string]int)
	
	for _, analysis := range analyses {
		exchangeCounts[analysis.BestExchange]++
		recommendedCounts[analysis.RecommendedSource]++
	}

	fmt.Printf("Total symbols analyzed: %d\n", len(analyses))
	fmt.Println()

	fmt.Println("Exchanges with oldest data:")
	for exchange, count := range exchangeCounts {
		fmt.Printf("  %s: %d symbols\n", exchange, count)
	}
	fmt.Println()

	fmt.Println("Recommended data sources:")
	for exchange, count := range recommendedCounts {
		fmt.Printf("  %s: %d symbols\n", exchange, count)
	}
	fmt.Println()
}

// generateRecommendations generates actionable recommendations
func generateRecommendations(analyses []SymbolAnalysis) {
	fmt.Println("=== RECOMMENDATIONS ===")
	fmt.Println()

	// Create mapping file content
	fmt.Println("Recommended exchange mapping for historical data import:")
	fmt.Println("Symbol,RecommendedExchange,EarliestDate,Reason")
	
	for _, analysis := range analyses {
		reason := "best_overall"
		if analysis.RecommendedSource == analysis.BestExchange {
			reason = "oldest_data"
		}
		
		fmt.Printf("%s,%s,%s,%s\n",
			analysis.Symbol,
			analysis.RecommendedSource,
			analysis.EarliestDate.Format("2006-01-02"),
			reason)
	}
	
	fmt.Println()
	fmt.Println("Summary:")
	fmt.Printf("• Use the above mapping to prioritize data sources\n")
	fmt.Printf("• Consider aggregating across multiple exchanges for better data quality\n")
	fmt.Printf("• Focus on symbols with 3+ years of history for trend analysis\n")
	fmt.Printf("• Monitor data quality scores - anything below 50%% needs review\n")
}