package core

import (
	"context"
	"math"
	"time"
)

func CalculateNudges(ctx context.Context, config *RankingConfig, flips []FlipRecord, failedSells []FailedSellRecord) map[int]float64 {
	nudges := make(map[int]float64)
	netNudges := make(map[int]float64)

	now := time.Now()

	// 1. Process successful flips
	halfLifeFlips := time.Duration(config.FlipHalfLifeHours) * time.Hour
	lambdaFlips := 0.69314718056 / halfLifeFlips.Seconds()

	for _, record := range flips {
		itemID := record.ItemID
		age := now.Sub(record.Timestamp).Seconds()
		if age < 0 {
			age = 0
		}
		weight := math.Exp(-lambdaFlips * age)

		direction := 0.0
		switch record.Rating {
		case "Meh":
			direction = config.FlipModifierMeh
		case "Mid":
			direction = config.FlipModifierMid
		case "Good":
			direction = config.FlipModifierGood
		case "Great":
			direction = config.FlipModifierGreat
		}

		netNudges[itemID] += weight * direction
	}

	// 2. Process failed sells
	halfLifeFailed := time.Duration(config.FailedSellHalfLifeHours) * time.Hour
	lambdaFailed := 0.69314718056 / halfLifeFailed.Seconds()

	for _, record := range failedSells {
		itemID := record.ItemID
		age := now.Sub(record.Timestamp).Seconds()
		if age < 0 {
			age = 0
		}
		weight := math.Exp(-lambdaFailed * age)

		// Static heavy penalty per failed sell
		direction := config.FailedSellPenalty
		netNudges[itemID] += weight * direction
	}

	// Calculate Exponentially Decayed Sum
	for itemID, sum := range netNudges {

		multiplier := 1.0 + sum
		// Clamp multiplier between configured min and an absolute max of 3.0
		if multiplier < config.NudgeMin {
			multiplier = config.NudgeMin
		}
		if multiplier > 3.0 {
			multiplier = 3.0
		}
		nudges[itemID] = multiplier
	}

	return nudges
}
