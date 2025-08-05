package polling

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/ashmitsharp/trading/internal/exchanges"
	"github.com/ashmitsharp/trading/internal/symbol"
	"go.uber.org/zap"
)

// Service handles polling exchanges for price data
type Service struct {
	postgresDB      *sql.DB
	clickhouseConn  driver.Conn
	symbolResolver  *symbol.Resolver
	exchangeClients []exchanges.ExchangeClient
	logger          *zap.Logger
	
	pollingInterval time.Duration
	ctx             context.Context
	cancel          context.CancelFunc
	wg              sync.WaitGroup
}

// NewService creates a new polling service
func NewService(
	postgresDB *sql.DB,
	clickhouseConn driver.Conn,
	exchangeClients []exchanges.ExchangeClient,
	logger *zap.Logger,
) *Service {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &Service{
		postgresDB:      postgresDB,
		clickhouseConn:  clickhouseConn,
		symbolResolver:  symbol.NewResolver(postgresDB, logger),
		exchangeClients: exchangeClients,
		logger:          logger,
		pollingInterval: 15 * time.Second,
		ctx:             ctx,
		cancel:          cancel,
	}
}

// Start begins polling exchanges
func (s *Service) Start() error {
	s.logger.Info("Starting polling service",
		zap.Int("exchanges", len(s.exchangeClients)),
		zap.Duration("interval", s.pollingInterval))
	
	// Start polling loop
	s.wg.Add(1)
	go s.pollLoop()
	
	return nil
}

// Stop gracefully stops the polling service
func (s *Service) Stop() error {
	s.logger.Info("Stopping polling service")
	s.cancel()
	s.wg.Wait()
	return nil
}

func (s *Service) pollLoop() {
	defer s.wg.Done()
	
	// Initial poll
	s.pollExchanges()
	
	ticker := time.NewTicker(s.pollingInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			s.pollExchanges()
		case <-s.ctx.Done():
			return
		}
	}
}

func (s *Service) pollExchanges() {
	start := time.Now()
	
	var wg sync.WaitGroup
	tickerChan := make(chan []exchanges.TickerData, len(s.exchangeClients))
	
	// Poll all exchanges concurrently
	for _, client := range s.exchangeClients {
		wg.Add(1)
		go func(client exchanges.ExchangeClient) {
			defer wg.Done()
			
			ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
			defer cancel()
			
			tickers, err := client.GetAllTickers(ctx)
			if err != nil {
				s.logger.Error("Failed to get tickers",
					zap.String("exchange", client.GetID()),
					zap.Error(err))
				client.UpdateHealth(false, 0)
				return
			}
			
			client.UpdateHealth(true, time.Since(start))
			
			// Resolve token IDs for each ticker
			for i := range tickers {
				s.resolveTickerTokenIDs(&tickers[i])
			}
			
			tickerChan <- tickers
		}(client)
	}
	
	// Wait for all polls to complete
	go func() {
		wg.Wait()
		close(tickerChan)
	}()
	
	// Collect all tickers
	var allTickers []exchanges.TickerData
	for tickers := range tickerChan {
		allTickers = append(allTickers, tickers...)
	}
	
	// Store in ClickHouse
	if err := s.storeTickers(allTickers); err != nil {
		s.logger.Error("Failed to store tickers", zap.Error(err))
	}
	
	s.logger.Info("Polling cycle completed",
		zap.Duration("duration", time.Since(start)),
		zap.Int("tickers", len(allTickers)))
}

func (s *Service) resolveTickerTokenIDs(ticker *exchanges.TickerData) {
	// Try to resolve the trading pair
	pair, err := s.symbolResolver.ResolveTradingPair(ticker.ExchangeID, ticker.Symbol)
	if err == nil {
		ticker.BaseTokenID = pair.BaseTokenID
		ticker.QuoteTokenID = pair.QuoteTokenID
		return
	}
	
	// Fallback: try to resolve individual symbols
	baseID, err1 := s.symbolResolver.ResolveSymbol(ticker.ExchangeID, ticker.BaseSymbol)
	quoteID, err2 := s.symbolResolver.ResolveSymbol(ticker.ExchangeID, ticker.QuoteSymbol)
	
	if err1 == nil && err2 == nil {
		ticker.BaseTokenID = baseID
		ticker.QuoteTokenID = quoteID
		
		// Add this pair to the database for future use
		s.symbolResolver.AddTradingPair(baseID, quoteID, ticker.ExchangeID, ticker.Symbol)
	} else {
		// Try normalized symbols as last resort
		if err1 != nil {
			if id, err := s.symbolResolver.GetTokenByNormalizedSymbol(ticker.BaseSymbol); err == nil {
				ticker.BaseTokenID = id
				// Add mapping for future use
				s.symbolResolver.AddSymbolMapping(id, ticker.ExchangeID, ticker.BaseSymbol, ticker.BaseSymbol)
			}
		}
		
		if err2 != nil {
			if id, err := s.symbolResolver.GetTokenByNormalizedSymbol(ticker.QuoteSymbol); err == nil {
				ticker.QuoteTokenID = id
				// Add mapping for future use
				s.symbolResolver.AddSymbolMapping(id, ticker.ExchangeID, ticker.QuoteSymbol, ticker.QuoteSymbol)
			}
		}
	}
	
	// Log unresolved pairs for investigation
	if ticker.BaseTokenID == 0 || ticker.QuoteTokenID == 0 {
		s.logger.Warn("Failed to resolve token IDs",
			zap.String("exchange", ticker.ExchangeID),
			zap.String("symbol", ticker.Symbol),
			zap.String("base", ticker.BaseSymbol),
			zap.String("quote", ticker.QuoteSymbol))
	}
}

func (s *Service) storeTickers(tickers []exchanges.TickerData) error {
	if len(tickers) == 0 {
		return nil
	}
	
	ctx := context.Background()
	
	// Store in price_tickers table
	batch, err := s.clickhouseConn.PrepareBatch(ctx, `
		INSERT INTO price_tickers (
			timestamp, exchange_id, base_token_id, quote_token_id,
			price, volume_24h, quote_volume_24h, high_24h, low_24h, price_change_24h
		)`)
	if err != nil {
		return fmt.Errorf("failed to prepare batch: %w", err)
	}
	
	timestamp := time.Now()
	validCount := 0
	
	for _, ticker := range tickers {
		// Skip tickers without resolved token IDs
		if ticker.BaseTokenID == 0 || ticker.QuoteTokenID == 0 {
			continue
		}
		
		if err := batch.Append(
			timestamp,
			ticker.ExchangeID,
			uint32(ticker.BaseTokenID),
			uint32(ticker.QuoteTokenID),
			ticker.Price,
			ticker.Volume24h,
			ticker.QuoteVolume24h,
			ticker.High24h,
			ticker.Low24h,
			ticker.PriceChange24h,
		); err != nil {
			s.logger.Error("Failed to append ticker to batch",
				zap.String("exchange", ticker.ExchangeID),
				zap.String("symbol", ticker.Symbol),
				zap.Error(err))
			continue
		}
		
		validCount++
	}
	
	if validCount > 0 {
		if err := batch.Send(); err != nil {
			return fmt.Errorf("failed to send batch: %w", err)
		}
	}
	
	s.logger.Debug("Stored tickers in ClickHouse",
		zap.Int("total", len(tickers)),
		zap.Int("stored", validCount))
	
	return nil
}

// GetSymbolResolver returns the symbol resolver instance
func (s *Service) GetSymbolResolver() *symbol.Resolver {
	return s.symbolResolver
}