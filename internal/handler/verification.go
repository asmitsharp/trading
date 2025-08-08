package handler

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/ashmitsharp/trading/internal/outlier"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// VerificationHandler handles mapping verification endpoints
type VerificationHandler struct {
	db       *sql.DB
	detector *outlier.Detector
	logger   *zap.Logger
}

// NewVerificationHandler creates a new verification handler
func NewVerificationHandler(db *sql.DB, detector *outlier.Detector, logger *zap.Logger) *VerificationHandler {
	return &VerificationHandler{
		db:       db,
		detector: detector,
		logger:   logger,
	}
}

// UnverifiedMapping represents a mapping that needs verification
type UnverifiedMapping struct {
	ID              int     `json:"id"`
	ExchangeID      string  `json:"exchange_id"`
	ExchangeSymbol  string  `json:"exchange_symbol"`
	TokenSymbol     string  `json:"token_symbol"`
	TokenName       string  `json:"token_name"`
	MappingMethod   string  `json:"mapping_method"`
	ConfidenceScore float64 `json:"confidence_score"`
	HasOutliers     bool    `json:"has_outliers"`
	CreatedAt       string  `json:"created_at"`
}

// GetUnverifiedMappings returns all unverified symbol-based mappings
func (h *VerificationHandler) GetUnverifiedMappings(c *gin.Context) {
	query := `
		SELECT 
			tes.id,
			tes.exchange_id,
			tes.exchange_symbol,
			t.symbol as token_symbol,
			t.name as token_name,
			tes.mapping_method,
			tes.confidence_score,
			tes.created_at,
			EXISTS(
				SELECT 1 FROM price_outliers po 
				WHERE po.exchange_id = tes.exchange_id 
				AND po.base_token_id = tes.token_id 
				AND po.is_resolved = false
			) as has_outliers
		FROM token_exchange_symbols tes
		JOIN tokens t ON tes.token_id = t.id
		WHERE tes.needs_verification = true
			AND tes.mapping_method = 'symbol'
		ORDER BY tes.confidence_score ASC, tes.created_at DESC
		LIMIT 100
	`
	
	rows, err := h.db.Query(query)
	if err != nil {
		h.logger.Error("Failed to fetch unverified mappings", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch mappings"})
		return
	}
	defer rows.Close()
	
	var mappings []UnverifiedMapping
	for rows.Next() {
		var m UnverifiedMapping
		err := rows.Scan(
			&m.ID,
			&m.ExchangeID,
			&m.ExchangeSymbol,
			&m.TokenSymbol,
			&m.TokenName,
			&m.MappingMethod,
			&m.ConfidenceScore,
			&m.CreatedAt,
			&m.HasOutliers,
		)
		if err != nil {
			continue
		}
		mappings = append(mappings, m)
	}
	
	c.JSON(http.StatusOK, gin.H{
		"mappings": mappings,
		"total":    len(mappings),
	})
}

// VerifyMapping marks a mapping as verified
func (h *VerificationHandler) VerifyMapping(c *gin.Context) {
	mappingID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid mapping ID"})
		return
	}
	
	var req struct {
		VerifiedBy string `json:"verified_by" binding:"required"`
		Notes      string `json:"notes"`
	}
	
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	// Update the mapping
	query := `
		UPDATE token_exchange_symbols
		SET needs_verification = false,
		    verified_by = $2,
		    verified_at = NOW(),
		    confidence_score = 1.0
		WHERE id = $1
	`
	
	_, err = h.db.Exec(query, mappingID, req.VerifiedBy)
	if err != nil {
		h.logger.Error("Failed to verify mapping", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify mapping"})
		return
	}
	
	// Log to audit table
	auditQuery := `
		INSERT INTO mapping_audit_log (
			token_id, exchange_id, exchange_symbol,
			mapping_method, confidence_score, action,
			performed_by, notes
		)
		SELECT 
			token_id, exchange_id, exchange_symbol,
			mapping_method, 1.0, 'verified',
			$2, $3
		FROM token_exchange_symbols
		WHERE id = $1
	`
	
	h.db.Exec(auditQuery, mappingID, req.VerifiedBy, req.Notes)
	
	c.JSON(http.StatusOK, gin.H{
		"message": "Mapping verified successfully",
		"id":      mappingID,
	})
}

// FlagMapping marks a mapping as incorrect
func (h *VerificationHandler) FlagMapping(c *gin.Context) {
	mappingID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid mapping ID"})
		return
	}
	
	var req struct {
		FlaggedBy string `json:"flagged_by" binding:"required"`
		Reason    string `json:"reason" binding:"required"`
		NewTokenID int   `json:"new_token_id,omitempty"`
	}
	
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	tx, err := h.db.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start transaction"})
		return
	}
	defer tx.Rollback()
	
	// If a new token ID is provided, update the mapping
	if req.NewTokenID > 0 {
		updateQuery := `
			UPDATE token_exchange_symbols
			SET token_id = $2,
			    mapping_method = 'manual',
			    confidence_score = 1.0,
			    needs_verification = false,
			    verified_by = $3,
			    verified_at = NOW()
			WHERE id = $1
		`
		_, err = tx.Exec(updateQuery, mappingID, req.NewTokenID, req.FlaggedBy)
	} else {
		// Otherwise, just mark it as needing more verification
		updateQuery := `
			UPDATE token_exchange_symbols
			SET confidence_score = 0.25,
			    needs_verification = true
			WHERE id = $1
		`
		_, err = tx.Exec(updateQuery, mappingID)
	}
	
	if err != nil {
		h.logger.Error("Failed to update mapping", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update mapping"})
		return
	}
	
	// Log to audit table
	auditQuery := `
		INSERT INTO mapping_audit_log (
			token_id, exchange_id, exchange_symbol,
			mapping_method, confidence_score, action,
			performed_by, notes
		)
		SELECT 
			token_id, exchange_id, exchange_symbol,
			'manual', 0.25, 'flagged',
			$2, $3
		FROM token_exchange_symbols
		WHERE id = $1
	`
	
	tx.Exec(auditQuery, mappingID, req.FlaggedBy, req.Reason)
	
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to commit transaction"})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"message": "Mapping flagged successfully",
		"id":      mappingID,
	})
}

// GetOutliers returns unresolved price outliers
func (h *VerificationHandler) GetOutliers(c *gin.Context) {
	outliers, err := h.detector.GetUnresolvedOutliers()
	if err != nil {
		h.logger.Error("Failed to fetch outliers", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch outliers"})
		return
	}
	
	// Enrich with token information
	type EnrichedOutlier struct {
		outlier.Outlier
		BaseTokenSymbol  string `json:"base_token_symbol"`
		QuoteTokenSymbol string `json:"quote_token_symbol"`
		BaseTokenName    string `json:"base_token_name"`
		QuoteTokenName   string `json:"quote_token_name"`
	}
	
	var enrichedOutliers []EnrichedOutlier
	for _, o := range outliers {
		var baseSymbol, quoteSymbol, baseName, quoteName string
		
		// Get token info
		h.db.QueryRow("SELECT symbol, name FROM tokens WHERE id = $1", o.BaseTokenID).
			Scan(&baseSymbol, &baseName)
		h.db.QueryRow("SELECT symbol, name FROM tokens WHERE id = $1", o.QuoteTokenID).
			Scan(&quoteSymbol, &quoteName)
		
		enrichedOutliers = append(enrichedOutliers, EnrichedOutlier{
			Outlier:          o,
			BaseTokenSymbol:  baseSymbol,
			QuoteTokenSymbol: quoteSymbol,
			BaseTokenName:    baseName,
			QuoteTokenName:   quoteName,
		})
	}
	
	c.JSON(http.StatusOK, gin.H{
		"outliers": enrichedOutliers,
		"total":    len(enrichedOutliers),
	})
}

// ResolveOutlier marks an outlier as resolved
func (h *VerificationHandler) ResolveOutlier(c *gin.Context) {
	outlierID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid outlier ID"})
		return
	}
	
	var req struct {
		ResolvedBy string `json:"resolved_by" binding:"required"`
		Notes      string `json:"notes" binding:"required"`
	}
	
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	if err := h.detector.ResolveOutlier(outlierID, req.ResolvedBy, req.Notes); err != nil {
		h.logger.Error("Failed to resolve outlier", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to resolve outlier"})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"message": "Outlier resolved successfully",
		"id":      outlierID,
	})
}