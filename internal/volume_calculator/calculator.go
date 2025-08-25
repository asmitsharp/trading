package volume_calculator

import (
	"fmt"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// VolumeCalculator calculates 1-minute volumes from 24-hour rolling data
type VolumeCalculator struct {
	buffers map[string]*CandleBuffer // key: "exchange:symbol"
	config  CalculationConfig
	logger  *zap.Logger
}

// NewVolumeCalculator creates a new volume calculator
func NewVolumeCalculator(config CalculationConfig, logger *zap.Logger) *VolumeCalculator {
	// Set default config values
	if config.MaxBufferSize == 0 {
		config.MaxBufferSize = 1440
	}
	if config.FallbackWindow == 0 {
		config.FallbackWindow = 7 // Use 7 recent candles for average
	}
	if config.TimestampTolerance == 0 {
		config.TimestampTolerance = 30 * time.Second
	}
	if config.MinValidVolume.IsZero() {
		config.MinValidVolume = decimal.NewFromFloat(0.000001) // 1 satoshi equivalent
	}
	if config.MaxValidVolume.IsZero() {
		config.MaxValidVolume = decimal.NewFromFloat(1e12) // 1 trillion
	}

	return &VolumeCalculator{
		buffers: make(map[string]*CandleBuffer),
		config:  config,
		logger:  logger,
	}
}

// Calculate1MinuteVolume calculates 1-minute volume from 24h rolling data
func (vc *VolumeCalculator) Calculate1MinuteVolume(
	current VolumeSnapshot,
	previous VolumeSnapshot,
	timestamp time.Time,
) VolumeCalculationResult {

	key := fmt.Sprintf("%s:%s", current.ExchangeID, current.Symbol)

	result := VolumeCalculationResult{
		Timestamp:    timestamp,
		Symbol:       current.Symbol,
		ExchangeID:   current.ExchangeID,
		BaseTokenID:  current.BaseTokenID,
		QuoteTokenID: current.QuoteTokenID,
		Method:       "unknown",
		IsValid:      false,
	}

	// Validate input data
	if err := vc.validateInputs(current, previous); err != nil {
		result.ErrorMessage = err.Error()
		vc.logger.Warn("Invalid input data",
			zap.String("key", key),
			zap.Error(err))
		return result
	}

	// Get or create buffer for this symbol/exchange
	buffer := vc.getOrCreateBuffer(key, current.Symbol, current.ExchangeID)

	result.InputData = VolumeInputData{
		Current24hVolume:  current.Volume24h,
		Previous24hVolume: previous.Volume24h,
		BufferSize:        buffer.Size(),
	}

	// Try standard calculation first
	volume, method, err := vc.calculateStandardVolume(current, previous, buffer, timestamp)
	if err == nil && vc.isValidVolume(volume) {
		result.CalculatedVolume = volume
		result.Method = method
		result.IsValid = true
		result.InputData.Volume1440MinAgo = vc.getVolume1440MinAgo(buffer, timestamp)

		vc.logger.Debug("Calculated 1m volume",
			zap.String("key", key),
			zap.String("method", method),
			zap.String("volume", volume.String()))

		return result
	}

	// Fall back to average-based calculation
	vc.logger.Debug("Standard calculation failed, using fallback",
		zap.String("key", key),
		zap.Error(err))

	fallbackVolume, fallbackErr := vc.calculateFallbackVolume(buffer)
	if fallbackErr == nil && vc.isValidVolume(fallbackVolume) {
		result.CalculatedVolume = fallbackVolume
		result.Method = "fallback_average"
		result.IsValid = true
		result.InputData.FallbackAverage = fallbackVolume
		result.InputData.UsedFallback = true

		vc.logger.Debug("Used fallback volume calculation",
			zap.String("key", key),
			zap.String("volume", fallbackVolume.String()))

		return result
	}

	// Last resort: simple difference
	simpleDiff := current.Volume24h.Sub(previous.Volume24h)
	if vc.isValidVolume(simpleDiff) {
		result.CalculatedVolume = simpleDiff
		result.Method = "simple_diff"
		result.IsValid = true

		vc.logger.Debug("Used simple difference calculation",
			zap.String("key", key),
			zap.String("volume", simpleDiff.String()))

		return result
	}

	// All methods failed
	result.ErrorMessage = fmt.Sprintf("all calculation methods failed: standard=%v, fallback=%v", err, fallbackErr)
	vc.logger.Warn("All volume calculation methods failed",
		zap.String("key", key),
		zap.Error(err),
		zap.NamedError("fallback_error", fallbackErr))

	return result
}

// calculateStandardVolume implements the main algorithm:
// 1min_volume = current_24h - previous_24h + buffer[-1440].volume
func (vc *VolumeCalculator) calculateStandardVolume(
	current, previous VolumeSnapshot,
	buffer *CandleBuffer,
	timestamp time.Time,
) (decimal.Decimal, string, error) {

	// Check if we have enough buffer data
	if buffer.Size() < vc.config.MaxBufferSize {
		// Use simple difference if buffer not full
		diff := current.Volume24h.Sub(previous.Volume24h)
		if diff.IsNegative() {
			return decimal.Zero, "", fmt.Errorf("negative volume difference and insufficient buffer")
		}
		return diff, "simple_diff", nil
	}

	// Get the volume from 1440 minutes ago
	candle1440, err := buffer.GetCandle1440MinutesAgo(timestamp, vc.config.TimestampTolerance)
	if err != nil {
		return decimal.Zero, "", fmt.Errorf("failed to get candle from 1440 min ago: %w", err)
	}

	// Calculate: current_24h - previous_24h + volume_1440_min_ago
	diff24h := current.Volume24h.Sub(previous.Volume24h)
	calculatedVolume := diff24h.Add(candle1440.Volume)

	// Validate result
	if calculatedVolume.IsNegative() {
		return decimal.Zero, "", fmt.Errorf("calculated volume is negative: %s", calculatedVolume.String())
	}

	return calculatedVolume, "standard", nil
}

// calculateFallbackVolume calculates average of recent valid candles
func (vc *VolumeCalculator) calculateFallbackVolume(buffer *CandleBuffer) (decimal.Decimal, error) {
	if buffer.Size() == 0 {
		return decimal.Zero, fmt.Errorf("buffer is empty")
	}

	avgVolume, err := buffer.GetAverageVolume(vc.config.FallbackWindow)
	if err != nil {
		return decimal.Zero, fmt.Errorf("failed to calculate average volume: %w", err)
	}

	return avgVolume, nil
}

// AddCalculatedCandle adds a calculated candle to the buffer
func (vc *VolumeCalculator) AddCalculatedCandle(candle OHLCVCandle) {
	key := fmt.Sprintf("%s:%s", candle.ExchangeID, candle.Symbol)
	buffer := vc.getOrCreateBuffer(key, candle.Symbol, candle.ExchangeID)
	buffer.Add(candle)

	vc.logger.Debug("Added candle to buffer",
		zap.String("key", key),
		zap.Time("timestamp", candle.Timestamp),
		zap.String("volume", candle.Volume.String()),
		zap.Int("buffer_size", buffer.Size()))
}

// validateInputs validates the volume snapshots
func (vc *VolumeCalculator) validateInputs(current, previous VolumeSnapshot) error {
	if current.Symbol != previous.Symbol {
		return fmt.Errorf("symbol mismatch: current=%s, previous=%s", current.Symbol, previous.Symbol)
	}

	if current.ExchangeID != previous.ExchangeID {
		return fmt.Errorf("exchange mismatch: current=%s, previous=%s", current.ExchangeID, previous.ExchangeID)
	}

	if current.Volume24h.IsNegative() {
		return fmt.Errorf("current 24h volume is negative: %s", current.Volume24h.String())
	}

	if previous.Volume24h.IsNegative() {
		return fmt.Errorf("previous 24h volume is negative: %s", previous.Volume24h.String())
	}

	if current.Volume24h.LessThan(vc.config.MinValidVolume) {
		return fmt.Errorf("current volume too small: %s", current.Volume24h.String())
	}

	if current.Volume24h.GreaterThan(vc.config.MaxValidVolume) {
		return fmt.Errorf("current volume too large: %s", current.Volume24h.String())
	}

	return nil
}

// isValidVolume checks if a volume value is valid
func (vc *VolumeCalculator) isValidVolume(volume decimal.Decimal) bool {
	return volume.GreaterThanOrEqual(vc.config.MinValidVolume) &&
		volume.LessThanOrEqual(vc.config.MaxValidVolume)
}

// getOrCreateBuffer gets existing buffer or creates new one
func (vc *VolumeCalculator) getOrCreateBuffer(key, symbol, exchangeID string) *CandleBuffer {
	buffer, exists := vc.buffers[key]
	if !exists {
		buffer = NewCandleBuffer(vc.config.MaxBufferSize, symbol, exchangeID)
		vc.buffers[key] = buffer
		vc.logger.Info("Created new candle buffer",
			zap.String("key", key),
			zap.Int("capacity", vc.config.MaxBufferSize))
	}
	return buffer
}

// getVolume1440MinAgo helper to get volume from 1440 minutes ago (for logging)
func (vc *VolumeCalculator) getVolume1440MinAgo(buffer *CandleBuffer, timestamp time.Time) decimal.Decimal {
	candle, err := buffer.GetCandle1440MinutesAgo(timestamp, vc.config.TimestampTolerance)
	if err != nil {
		return decimal.Zero
	}
	return candle.Volume
}

// GetBufferStats returns statistics about all buffers
func (vc *VolumeCalculator) GetBufferStats() map[string]interface{} {
	stats := make(map[string]interface{})

	for key, buffer := range vc.buffers {
		stats[key] = map[string]interface{}{
			"size":         buffer.Size(),
			"capacity":     buffer.capacity,
			"is_full":      buffer.IsFull(),
			"volume_stats": buffer.GetVolumeStats(),
		}
	}

	return stats
}

// ClearBuffer clears a specific buffer
func (vc *VolumeCalculator) ClearBuffer(exchangeID, symbol string) {
	key := fmt.Sprintf("%s:%s", exchangeID, symbol)
	if buffer, exists := vc.buffers[key]; exists {
		buffer.Clear()
		vc.logger.Info("Cleared buffer", zap.String("key", key))
	}
}

// ClearAllBuffers clears all buffers
func (vc *VolumeCalculator) ClearAllBuffers() {
	for _, buffer := range vc.buffers {
		buffer.Clear()
	}
	vc.logger.Info("Cleared all buffers", zap.Int("count", len(vc.buffers)))
}
