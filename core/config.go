package core

import (
	"encoding/json"
	"fmt"
	"os"
)

// RankingConfig defines the penalties, weights, and multipliers that affect the final score of an item.
type RankingConfig struct {
	// Taxation
	TaxRate float64 `json:"tax_rate"` // Default: 0.02 (2%)
	TaxCap  int64   `json:"tax_cap"`  // Default: 5000000

	// Modifiers for Flips (Values added to the base 1.0 multiplier)
	FlipHalfLifeHours float64 `json:"flip_half_life_hours"` // Time for flip impact to halve (e.g. 24.0)
	FlipModifierMeh   float64 `json:"flip_modifier_meh"`    // e.g. -0.10
	FlipModifierMid   float64 `json:"flip_modifier_mid"`    // e.g. 0.0
	FlipModifierGood  float64 `json:"flip_modifier_good"`   // e.g. 0.10
	FlipModifierGreat float64 `json:"flip_modifier_great"`  // e.g. 0.20

	// Modifiers for Failed Buys (Values subtracted from the multiplier)
	FailedSellHalfLifeHours float64 `json:"failed_sell_half_life_hours"` // Time for penalty to halve (e.g. 72.0)
	FailedSellPenalty       float64 `json:"failed_sell_penalty"`         // Base penalty (e.g. -0.40)

	// Nudge Constraints (Absolute min/max limits for historical multipliers)
	NudgeMin float64 `json:"nudge_min"` // e.g. 0.05
	NudgeMax float64 `json:"nudge_max"` // e.g. 2.0

	// Base Score Penalties
	CapitalPenaltyBaseWeight  float64 `json:"capital_penalty_base_weight"`  // e.g. 0.5
	CapitalPenaltyScaleWeight float64 `json:"capital_penalty_scale_weight"` // e.g. 0.5
	VolumeRatioPenaltyMax     float64 `json:"volume_ratio_penalty_max"`     // e.g. 1.30
	AbsoluteVolumePenaltyMax  float64 `json:"absolute_volume_penalty_max"`  // e.g. 1.30

	// Price Trend Penalties (Multipliers applied when item price is crashing vs moving averages)
	PriceTrendPenalty1h  float64 `json:"price_trend_penalty_1h"`  // e.g. 0.80
	PriceTrendPenalty24h float64 `json:"price_trend_penalty_24h"` // e.g. 0.90
	PriceTrendPenalty30d float64 `json:"price_trend_penalty_30d"` // e.g. 0.95

	// Volume Spikes
	VolumeSpike5mMultiplier  float64 `json:"volume_spike_5m_multiplier"`  // Reward for sudden 5m volume spike (e.g. 1.50)
	VolumeSpike24hMultiplier float64 `json:"volume_spike_24h_multiplier"` // Reward for sudden 1h volume relative to 24h (e.g. 1.20)
}

// DefaultRankingConfig returns a configuration struct matching the original hardcoded values.
func DefaultRankingConfig() *RankingConfig {
	return &RankingConfig{
		TaxRate:                   0.02,
		TaxCap:                    5000000,
		FlipHalfLifeHours:         168.0,
		FlipModifierMeh:           -0.10,
		FlipModifierMid:           0.0,
		FlipModifierGood:          0.10,
		FlipModifierGreat:         0.20,
		FailedSellHalfLifeHours:   72.0,
		FailedSellPenalty:         -0.40,
		NudgeMin:                  0.05,
		NudgeMax:                  2.0,
		CapitalPenaltyBaseWeight:  0.5,
		CapitalPenaltyScaleWeight: 0.5,
		VolumeRatioPenaltyMax:     1.30,
		AbsoluteVolumePenaltyMax:  1.30,
		PriceTrendPenalty1h:       0.80,
		PriceTrendPenalty24h:      0.90,
		PriceTrendPenalty30d:      0.95,
		VolumeSpike5mMultiplier:   1.50,
		VolumeSpike24hMultiplier:  1.20,
	}
}

// LoadConfig reads the JSON configuration from the given path.
// If the file does not exist, it creates it populated with default values.
func LoadConfig(path string) (*RankingConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Generate the default config
			config := DefaultRankingConfig()
			out, err := json.MarshalIndent(config, "", "  ")
			if err != nil {
				return nil, fmt.Errorf("failed to marshal default config: %w", err)
			}
			if err := os.WriteFile(path, out, 0644); err != nil {
				return nil, fmt.Errorf("failed to write default config to %s: %w", path, err)
			}
			return config, nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	config := &RankingConfig{}
	if err := json.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config file %s: %w", path, err)
	}

	return config, nil
}
