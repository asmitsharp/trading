package volume_calculator

import (
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

// NewCandleBuffer creates a new circular buffer for OHLCV candles
func NewCandleBuffer(capacity int, symbol, exchangeID string) *CandleBuffer {
	return &CandleBuffer{
		candles:    make([]OHLCVCandle, capacity),
		capacity:   capacity,
		size:       0,
		writePos:   0,
		symbol:     symbol,
		exchangeID: exchangeID,
	}
}

// Add adds a new candle to the buffer
func (cb *CandleBuffer) Add(candle OHLCVCandle) {
	cb.candles[cb.writePos] = candle
	cb.writePos = (cb.writePos + 1) % cb.capacity
	
	if cb.size < cb.capacity {
		cb.size++
	}
}

// GetCandle returns a candle at the specified index (0 = most recent)
func (cb *CandleBuffer) GetCandle(index int) (OHLCVCandle, error) {
	if index >= cb.size {
		return OHLCVCandle{}, fmt.Errorf("index %d out of range, buffer size: %d", index, cb.size)
	}
	
	// Calculate the actual position in the circular buffer
	pos := (cb.writePos - 1 - index + cb.capacity) % cb.capacity
	return cb.candles[pos], nil
}

// GetCandleByTime returns the candle closest to the specified timestamp
func (cb *CandleBuffer) GetCandleByTime(targetTime time.Time, tolerance time.Duration) (OHLCVCandle, error) {
	if cb.size == 0 {
		return OHLCVCandle{}, fmt.Errorf("buffer is empty")
	}
	
	var closestCandle OHLCVCandle
	minDiff := time.Duration(1<<63 - 1) // Max duration
	found := false
	
	for i := 0; i < cb.size; i++ {
		candle, err := cb.GetCandle(i)
		if err != nil {
			continue
		}
		
		diff := abs(candle.Timestamp.Sub(targetTime))
		if diff <= tolerance && diff < minDiff {
			closestCandle = candle
			minDiff = diff
			found = true
		}
	}
	
	if !found {
		return OHLCVCandle{}, fmt.Errorf("no candle found within tolerance %v of target time %v", tolerance, targetTime)
	}
	
	return closestCandle, nil
}

// GetCandle1440MinutesAgo returns the candle from approximately 1440 minutes ago
func (cb *CandleBuffer) GetCandle1440MinutesAgo(referenceTime time.Time, tolerance time.Duration) (OHLCVCandle, error) {
	target1440MinAgo := referenceTime.Add(-1440 * time.Minute)
	return cb.GetCandleByTime(target1440MinAgo, tolerance)
}

// GetRecentCandles returns the N most recent candles for fallback averaging
func (cb *CandleBuffer) GetRecentCandles(count int) ([]OHLCVCandle, error) {
	if count > cb.size {
		count = cb.size
	}
	
	if count == 0 {
		return nil, fmt.Errorf("no candles available")
	}
	
	candles := make([]OHLCVCandle, count)
	for i := 0; i < count; i++ {
		candle, err := cb.GetCandle(i)
		if err != nil {
			return candles[:i], err
		}
		candles[i] = candle
	}
	
	return candles, nil
}

// Size returns the current number of candles in the buffer
func (cb *CandleBuffer) Size() int {
	return cb.size
}

// IsFull returns true if the buffer is at capacity
func (cb *CandleBuffer) IsFull() bool {
	return cb.size == cb.capacity
}

// GetAverageVolume calculates average volume from recent candles
func (cb *CandleBuffer) GetAverageVolume(count int) (decimal.Decimal, error) {
	candles, err := cb.GetRecentCandles(count)
	if err != nil {
		return decimal.Zero, err
	}
	
	if len(candles) == 0 {
		return decimal.Zero, fmt.Errorf("no candles for average calculation")
	}
	
	sum := decimal.Zero
	validCount := 0
	
	for _, candle := range candles {
		if candle.Volume.IsPositive() {
			sum = sum.Add(candle.Volume)
			validCount++
		}
	}
	
	if validCount == 0 {
		return decimal.Zero, fmt.Errorf("no valid volumes for average calculation")
	}
	
	return sum.Div(decimal.NewFromInt(int64(validCount))), nil
}

// GetVolumeStats returns statistics about volumes in the buffer
func (cb *CandleBuffer) GetVolumeStats() map[string]interface{} {
	if cb.size == 0 {
		return map[string]interface{}{
			"count": 0,
			"min":   0,
			"max":   0,
			"avg":   0,
		}
	}
	
	min := decimal.NewFromFloat(1e18)
	max := decimal.Zero
	sum := decimal.Zero
	count := 0
	
	for i := 0; i < cb.size; i++ {
		candle, err := cb.GetCandle(i)
		if err != nil {
			continue
		}
		
		if candle.Volume.IsPositive() {
			if candle.Volume.LessThan(min) {
				min = candle.Volume
			}
			if candle.Volume.GreaterThan(max) {
				max = candle.Volume
			}
			sum = sum.Add(candle.Volume)
			count++
		}
	}
	
	avg := decimal.Zero
	if count > 0 {
		avg = sum.Div(decimal.NewFromInt(int64(count)))
	}
	
	return map[string]interface{}{
		"count": count,
		"min":   min.String(),
		"max":   max.String(),
		"avg":   avg.String(),
	}
}

// Clear empties the buffer
func (cb *CandleBuffer) Clear() {
	cb.size = 0
	cb.writePos = 0
}

// abs returns the absolute duration
func abs(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}