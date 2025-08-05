package symbol

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// TokenPair represents a base/quote token pair
type TokenPair struct {
	BaseTokenID  int
	QuoteTokenID int
}

// ExchangeSymbol represents a symbol mapping for an exchange
type ExchangeSymbol struct {
	TokenID          int
	ExchangeID       string
	ExchangeSymbol   string
	NormalizedSymbol string
}

// TradingPair represents a trading pair on an exchange
type TradingPair struct {
	ID                  int
	BaseTokenID         int
	QuoteTokenID        int
	ExchangeID          string
	ExchangePairSymbol  string
	IsActive            bool
}

// Resolver handles symbol to token ID resolution
type Resolver struct {
	db                *sql.DB
	logger            *zap.Logger
	
	// Caches
	symbolCache       map[string]map[string]int    // exchangeID -> symbol -> tokenID
	pairCache         map[string]map[string]TokenPair // exchangeID -> pairSymbol -> TokenPair
	normalizedCache   map[string]int               // normalizedSymbol -> tokenID
	
	mu                sync.RWMutex
	lastRefresh       time.Time
	refreshInterval   time.Duration
}

// NewResolver creates a new symbol resolver
func NewResolver(db *sql.DB, logger *zap.Logger) *Resolver {
	r := &Resolver{
		db:              db,
		logger:          logger,
		symbolCache:     make(map[string]map[string]int),
		pairCache:       make(map[string]map[string]TokenPair),
		normalizedCache: make(map[string]int),
		refreshInterval: 5 * time.Minute,
	}
	
	// Load initial cache
	if err := r.RefreshCache(context.Background()); err != nil {
		logger.Error("Failed to load initial symbol cache", zap.Error(err))
	}
	
	// Start background refresh
	go r.startBackgroundRefresh()
	
	return r
}

// ResolveSymbol resolves an exchange symbol to a token ID
func (r *Resolver) ResolveSymbol(exchangeID, symbol string) (int, error) {
	r.mu.RLock()
	if exchangeSymbols, ok := r.symbolCache[exchangeID]; ok {
		if tokenID, ok := exchangeSymbols[symbol]; ok {
			r.mu.RUnlock()
			return tokenID, nil
		}
	}
	r.mu.RUnlock()
	
	// Not in cache, try to fetch from database
	tokenID, err := r.fetchSymbolFromDB(exchangeID, symbol)
	if err != nil {
		// Try normalized lookup as fallback
		normalized := r.normalizeSymbol(symbol)
		if id, ok := r.normalizedCache[normalized]; ok {
			return id, nil
		}
		return 0, fmt.Errorf("symbol %s not found for exchange %s", symbol, exchangeID)
	}
	
	// Update cache
	r.mu.Lock()
	if r.symbolCache[exchangeID] == nil {
		r.symbolCache[exchangeID] = make(map[string]int)
	}
	r.symbolCache[exchangeID][symbol] = tokenID
	r.mu.Unlock()
	
	return tokenID, nil
}

// ResolveTradingPair resolves a trading pair symbol to base and quote token IDs
func (r *Resolver) ResolveTradingPair(exchangeID, pairSymbol string) (*TokenPair, error) {
	r.mu.RLock()
	if pairs, ok := r.pairCache[exchangeID]; ok {
		if pair, ok := pairs[pairSymbol]; ok {
			r.mu.RUnlock()
			return &pair, nil
		}
	}
	r.mu.RUnlock()
	
	// Not in cache, try to fetch from database
	pair, err := r.fetchPairFromDB(exchangeID, pairSymbol)
	if err != nil {
		// Try to parse and resolve individually
		base, quote := r.parsePairSymbol(pairSymbol, exchangeID)
		baseID, err1 := r.ResolveSymbol(exchangeID, base)
		quoteID, err2 := r.ResolveSymbol(exchangeID, quote)
		
		if err1 != nil || err2 != nil {
			return nil, fmt.Errorf("pair %s not found for exchange %s", pairSymbol, exchangeID)
		}
		
		pair = &TokenPair{BaseTokenID: baseID, QuoteTokenID: quoteID}
	}
	
	// Update cache
	r.mu.Lock()
	if r.pairCache[exchangeID] == nil {
		r.pairCache[exchangeID] = make(map[string]TokenPair)
	}
	r.pairCache[exchangeID][pairSymbol] = *pair
	r.mu.Unlock()
	
	return pair, nil
}

// AddSymbolMapping adds a new symbol mapping
func (r *Resolver) AddSymbolMapping(tokenID int, exchangeID, exchangeSymbol, normalizedSymbol string) error {
	query := `
		INSERT INTO token_exchange_symbols (token_id, exchange_id, exchange_symbol, normalized_symbol)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (exchange_id, exchange_symbol) 
		DO UPDATE SET token_id = $1, normalized_symbol = $4, updated_at = NOW()
	`
	
	_, err := r.db.Exec(query, tokenID, exchangeID, exchangeSymbol, normalizedSymbol)
	if err != nil {
		return fmt.Errorf("failed to add symbol mapping: %w", err)
	}
	
	// Update cache
	r.mu.Lock()
	if r.symbolCache[exchangeID] == nil {
		r.symbolCache[exchangeID] = make(map[string]int)
	}
	r.symbolCache[exchangeID][exchangeSymbol] = tokenID
	r.normalizedCache[normalizedSymbol] = tokenID
	r.mu.Unlock()
	
	return nil
}

// AddTradingPair adds a new trading pair mapping
func (r *Resolver) AddTradingPair(baseTokenID, quoteTokenID int, exchangeID, pairSymbol string) error {
	query := `
		INSERT INTO trading_pairs (base_token_id, quote_token_id, exchange_id, exchange_pair_symbol)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (exchange_id, exchange_pair_symbol)
		DO UPDATE SET base_token_id = $1, quote_token_id = $2, updated_at = NOW()
	`
	
	_, err := r.db.Exec(query, baseTokenID, quoteTokenID, exchangeID, pairSymbol)
	if err != nil {
		return fmt.Errorf("failed to add trading pair: %w", err)
	}
	
	// Update cache
	r.mu.Lock()
	if r.pairCache[exchangeID] == nil {
		r.pairCache[exchangeID] = make(map[string]TokenPair)
	}
	r.pairCache[exchangeID][pairSymbol] = TokenPair{
		BaseTokenID:  baseTokenID,
		QuoteTokenID: quoteTokenID,
	}
	r.mu.Unlock()
	
	return nil
}

// RefreshCache refreshes the symbol cache from the database
func (r *Resolver) RefreshCache(ctx context.Context) error {
	// Load symbol mappings
	symbolQuery := `
		SELECT token_id, exchange_id, exchange_symbol, normalized_symbol
		FROM token_exchange_symbols
		WHERE is_active = true
	`
	
	rows, err := r.db.QueryContext(ctx, symbolQuery)
	if err != nil {
		return fmt.Errorf("failed to query symbol mappings: %w", err)
	}
	defer rows.Close()
	
	newSymbolCache := make(map[string]map[string]int)
	newNormalizedCache := make(map[string]int)
	
	for rows.Next() {
		var tokenID int
		var exchangeID, exchangeSymbol, normalizedSymbol string
		
		if err := rows.Scan(&tokenID, &exchangeID, &exchangeSymbol, &normalizedSymbol); err != nil {
			r.logger.Error("Failed to scan symbol mapping", zap.Error(err))
			continue
		}
		
		if newSymbolCache[exchangeID] == nil {
			newSymbolCache[exchangeID] = make(map[string]int)
		}
		newSymbolCache[exchangeID][exchangeSymbol] = tokenID
		newNormalizedCache[normalizedSymbol] = tokenID
	}
	
	// Load trading pairs
	pairQuery := `
		SELECT base_token_id, quote_token_id, exchange_id, exchange_pair_symbol
		FROM trading_pairs
		WHERE is_active = true
	`
	
	pairRows, err := r.db.QueryContext(ctx, pairQuery)
	if err != nil {
		return fmt.Errorf("failed to query trading pairs: %w", err)
	}
	defer pairRows.Close()
	
	newPairCache := make(map[string]map[string]TokenPair)
	
	for pairRows.Next() {
		var baseTokenID, quoteTokenID int
		var exchangeID, pairSymbol string
		
		if err := pairRows.Scan(&baseTokenID, &quoteTokenID, &exchangeID, &pairSymbol); err != nil {
			r.logger.Error("Failed to scan trading pair", zap.Error(err))
			continue
		}
		
		if newPairCache[exchangeID] == nil {
			newPairCache[exchangeID] = make(map[string]TokenPair)
		}
		newPairCache[exchangeID][pairSymbol] = TokenPair{
			BaseTokenID:  baseTokenID,
			QuoteTokenID: quoteTokenID,
		}
	}
	
	// Update caches atomically
	r.mu.Lock()
	r.symbolCache = newSymbolCache
	r.pairCache = newPairCache
	r.normalizedCache = newNormalizedCache
	r.lastRefresh = time.Now()
	r.mu.Unlock()
	
	r.logger.Info("Symbol cache refreshed",
		zap.Int("symbols", len(newNormalizedCache)),
		zap.Int("exchanges", len(newSymbolCache)))
	
	return nil
}

// Helper methods

func (r *Resolver) fetchSymbolFromDB(exchangeID, symbol string) (int, error) {
	var tokenID int
	query := `
		SELECT token_id FROM token_exchange_symbols
		WHERE exchange_id = $1 AND exchange_symbol = $2 AND is_active = true
	`
	
	err := r.db.QueryRow(query, exchangeID, symbol).Scan(&tokenID)
	if err != nil {
		return 0, err
	}
	
	return tokenID, nil
}

func (r *Resolver) fetchPairFromDB(exchangeID, pairSymbol string) (*TokenPair, error) {
	var pair TokenPair
	query := `
		SELECT base_token_id, quote_token_id FROM trading_pairs
		WHERE exchange_id = $1 AND exchange_pair_symbol = $2 AND is_active = true
	`
	
	err := r.db.QueryRow(query, exchangeID, pairSymbol).Scan(&pair.BaseTokenID, &pair.QuoteTokenID)
	if err != nil {
		return nil, err
	}
	
	return &pair, nil
}

func (r *Resolver) normalizeSymbol(symbol string) string {
	// Remove common suffixes and normalize
	normalized := strings.ToUpper(symbol)
	
	// Remove exchange-specific prefixes
	prefixes := []string{"X", "XX", "t"}
	for _, prefix := range prefixes {
		if strings.HasPrefix(normalized, prefix) && len(normalized) > len(prefix) {
			normalized = normalized[len(prefix):]
			break
		}
	}
	
	// Handle special cases
	replacements := map[string]string{
		"XBT": "BTC",
	}
	
	for old, new := range replacements {
		if normalized == old {
			return new
		}
	}
	
	return normalized
}

func (r *Resolver) parsePairSymbol(pairSymbol, exchangeID string) (base, quote string) {
	// Try common separators
	separators := []string{"-", "_", "/"}
	
	for _, sep := range separators {
		if strings.Contains(pairSymbol, sep) {
			parts := strings.Split(pairSymbol, sep)
			if len(parts) == 2 {
				return parts[0], parts[1]
			}
		}
	}
	
	// Try to match against known quote currencies
	quoteCurrencies := []string{"USDT", "USDC", "USD", "BTC", "ETH", "EUR", "GBP", "JPY", "KRW", "BNB"}
	upper := strings.ToUpper(pairSymbol)
	
	for _, quote := range quoteCurrencies {
		if strings.HasSuffix(upper, quote) {
			base = upper[:len(upper)-len(quote)]
			return base, quote
		}
	}
	
	// Default: assume 3-letter base and remaining as quote
	if len(pairSymbol) >= 6 {
		return pairSymbol[:3], pairSymbol[3:]
	}
	
	return pairSymbol, ""
}

func (r *Resolver) startBackgroundRefresh() {
	ticker := time.NewTicker(r.refreshInterval)
	defer ticker.Stop()
	
	for range ticker.C {
		if err := r.RefreshCache(context.Background()); err != nil {
			r.logger.Error("Failed to refresh symbol cache", zap.Error(err))
		}
	}
}

// GetTokenByNormalizedSymbol gets token ID by normalized symbol
func (r *Resolver) GetTokenByNormalizedSymbol(symbol string) (int, error) {
	normalized := r.normalizeSymbol(symbol)
	
	r.mu.RLock()
	if tokenID, ok := r.normalizedCache[normalized]; ok {
		r.mu.RUnlock()
		return tokenID, nil
	}
	r.mu.RUnlock()
	
	// Try to fetch from database
	var tokenID int
	query := `
		SELECT id FROM tokens 
		WHERE UPPER(symbol) = $1 AND is_active = true
		LIMIT 1
	`
	
	err := r.db.QueryRow(query, normalized).Scan(&tokenID)
	if err != nil {
		return 0, fmt.Errorf("token not found for symbol %s", symbol)
	}
	
	return tokenID, nil
}