package core

import (
	"context"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"time"
)

// searchPrefixLatest scans a directory and returns the path and timestamp of the latest file matching the prefix.
func searchPrefixLatest(dir, prefix string) (string, int64, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", 0, fmt.Errorf("failed to read directory %s: %w", dir, err)
	}

	var latestPath string
	var latestTime int64
	pattern := prefix + "_"

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		// Match files like "prefix_1718815709.json"
		if len(name) > len(pattern)+5 && name[:len(pattern)] == pattern && name[len(name)-5:] == ".json" {
			var ts int64
			_, err := fmt.Sscanf(name[len(pattern):len(name)-5], "%d", &ts)
			if err == nil && ts > latestTime {
				latestTime = ts
				latestPath = fmt.Sprintf("%s/%s", dir, name)
			}
		}
	}

	if latestPath == "" {
		return "", 0, fmt.Errorf("no files found matching %s/%s*.json", dir, prefix)
	}

	return latestPath, latestTime, nil
}

// FindLatestFile searches the past 7 days of yyyy/mm/dd directories, falling back to the legacy root.
func FindLatestFile(dir, prefix string) (string, int64, error) {
	now := time.Now().UTC()
	for i := 0; i < 7; i++ {
		t := now.AddDate(0, 0, -i)
		dateDir := fmt.Sprintf("%s/%04d/%02d/%02d", dir, t.Year(), int(t.Month()), t.Day())
		path, ts, err := searchPrefixLatest(dateDir, prefix)
		if err == nil {
			return path, ts, nil
		}
	}

	// Fallback to legacy root directory
	path, ts, err := searchPrefixLatest(dir, prefix)
	if err == nil {
		return path, ts, nil
	}

	return "", 0, fmt.Errorf("no files found matching %s in date dirs or legacy %s", prefix, dir)
}

// formatCommas formats an integer with thousands-separator commas.
func formatCommas(n int64) string {
	in := fmt.Sprintf("%d", n)
	var out []byte
	if n < 0 {
		in = in[1:]
		out = append(out, '-')
	}
	l := len(in)
	for i, c := range in {
		if i > 0 && (l-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, byte(c))
	}
	return string(out)
}

// formatCompact formats large GP values into a compact string with custom granularity:
// - Less than 100k: full number (e.g. 95000)
// - Up to 1M: 2 decimal places with "K" (e.g. 300.23K)
// - 1M and beyond: 3 decimal places with "M" (e.g. 5.123M)
func formatCompact(n int64) string {
	absN := n
	sign := ""
	if n < 0 {
		absN = -n
		sign = "-"
	}
	if absN >= 1_000_000 {
		return fmt.Sprintf("%s%.3fM", sign, float64(absN)/1_000_000.0)
	}
	if absN >= 100_000 {
		return fmt.Sprintf("%s%.2fK", sign, float64(absN)/1_000.0)
	}
	return fmt.Sprintf("%s%d", sign, absN)
}

// OutlierStats holds the computed mean and stddev for price outlier detection.
type OutlierStats struct {
	Mean   float64
	StdDev float64
	Valid  bool
}

// calculateRollingStats computes the mean and standard deviation from a continuous rolling window.
func calculateRollingStats(vals []float64) OutlierStats {
	if len(vals) < 2 {
		return OutlierStats{Valid: false}
	}

	var sumVals float64
	for _, v := range vals {
		sumVals += v
	}
	mean := sumVals / float64(len(vals))

	var sumVariance float64
	for _, v := range vals {
		diff := v - mean
		sumVariance += diff * diff
	}

	variance := sumVariance / float64(len(vals)-1) // Sample variance
	stdDev := math.Sqrt(variance)

	// Volatility floor: standard deviation must be at least 1% of the mean or 2.0 (whichever is higher)
	safeStdDev := math.Max(stdDev, math.Max(mean*0.01, 2.0))

	return OutlierStats{
		Mean:   mean,
		StdDev: safeStdDev,
		Valid:  true,
	}
}

// calculateMidpointVolatility calculates the average absolute percentage change of the midpoint between consecutive steps,
// capped at 10% per step, with a denominator floor of 100gp to prevent penny-stock volatility skewing.
func calculateMidpointVolatility(highs, lows []float64) float64 {
	length := len(highs)
	if len(lows) < length {
		length = len(lows)
	}
	if length < 2 {
		return 0.0
	}
	var sumPctChange float64
	for i := 1; i < length; i++ {
		// Index 0 is newest, so i is older and i-1 is newer
		midOld := (highs[i] + lows[i]) / 2.0
		midNew := (highs[i-1] + lows[i-1]) / 2.0

		// 1. Enforce a minimum denominator of 100 gp
		safeDenominator := math.Max(midOld, 100.0)

		// 2. Divide by the OLDER tick
		stepVariation := math.Abs(midNew-midOld) / safeDenominator

		// 3. Clamp the variation to max 10% per step
		cappedStep := math.Min(stepVariation, 0.10)
		sumPctChange += cappedStep
	}
	return sumPctChange / float64(length-1)
}

// calculateSpreadJitterAndSpike calculates the average absolute percentage change of the spread between consecutive steps,
// and the ratio of the current spread to the historical mean spread over the given window.
func calculateSpreadJitterAndSpike(highs, lows []float64, currentSpread float64) (jitter float64, spikeRatio float64, meanSpread float64, meanHigh float64) {
	length := len(highs)
	if len(lows) < length {
		length = len(lows)
	}
	if length < 2 {
		return 0.0, 0.0, 0.0, 0.0
	}

	var sumSpread float64
	var sumPctChange float64
	var sumHigh float64
	for i := 1; i < length; i++ {
		spreadOld := highs[i] - lows[i]
		spreadNew := highs[i-1] - lows[i-1]
		sumSpread += spreadOld
		sumHigh += highs[i]

		// 1. Enforce a minimum denominator of 10 gp
		safeSpread := math.Max(spreadOld, 10.0)

		// 2. Divide by the OLDER tick
		stepVariation := math.Abs(spreadNew-spreadOld) / safeSpread

		// 3. Clamp the variation to max 50% per step
		cappedStep := math.Min(stepVariation, 0.50)
		sumPctChange += cappedStep
	}

	// Add the newest spread to the mean sum
	sumSpread += highs[0] - lows[0]
	sumHigh += highs[0]
	meanSpread = sumSpread / float64(length)
	meanHigh = sumHigh / float64(length)
	
	jitter = sumPctChange / float64(length-1)
	spikeRatio = currentSpread / math.Max(meanSpread, 1.0)
	return jitter, spikeRatio, meanSpread, meanHigh
}

// AnalyzePrices runs the analysis algorithm and returns a sorted slice of ReportItems.
func AnalyzePrices(
	ctx context.Context,
	runTs int64,
	prices map[string]LatestPrice,
	volumes map[string]HourlyVolume,
	metadata map[int]ItemMetadata,
	nudgeMultipliers map[int]float64,
	hist1h map[string]HourlyVolume,
	hist24h map[string]HourlyVolume,
	hist30d map[string]HourlyVolume,
	vol5m map[string]HourlyVolume,
	vol24h map[string]HourlyVolume,
	filterName string,
	config *RankingConfig,
	rolling24 []map[string]HourlyVolume,
) []ReportItem {
	var items []ReportItem

	for id, item := range metadata {
		isTarget := false
		isGoldenMargin := false
		if filterName != "" {
			if !strings.Contains(strings.ToLower(item.Name), filterName) {
				continue
			}
			isTarget = true
		}

		// 1. Skip items with invalid buy limits (cannot flip or unknown)
		if item.Limit <= 0 && !isTarget {
			continue
		}

		// 2. Fetch price details
		price, ok := prices[fmt.Sprintf("%d", id)]
		if (!ok || price.High == nil || price.Low == nil) && !isTarget {
			continue // No transaction data
		}

		var high, low int64
		if ok && price.High != nil {
			high = *price.High
		}
		if ok && price.Low != nil {
			low = *price.Low
		}

		var priceTrendIndicators []string
		outlierTrendMultiplier := 1.0
		idStr := fmt.Sprintf("%d", id)

		// 2b. Outlier Detection (Z-Score) using Continuous Rolling Window
		rawHigh := float64(high)
		var rollingHighs []float64
		var rollingLows []float64
		for _, tickMap := range rolling24 {
			if tickData, ok := tickMap[idStr]; ok {
				if tickData.AvgHighPrice != nil {
					rollingHighs = append(rollingHighs, float64(*tickData.AvgHighPrice))
				}
				if tickData.AvgLowPrice != nil {
					rollingLows = append(rollingLows, float64(*tickData.AvgLowPrice))
				}
			}
		}

		highStats := calculateRollingStats(rollingHighs)
		if highStats.Valid && highStats.StdDev > 0 {
			zHigh := (float64(high) - highStats.Mean) / highStats.StdDev
			if zHigh > config.OutlierZScoreThreshold {
				safeHighPrice := int64(highStats.Mean + (config.OutlierZScoreThreshold * highStats.StdDev))
				if high > safeHighPrice {
					high = safeHighPrice
					priceTrendIndicators = append(priceTrendIndicators, fmt.Sprintf("HC(%.1f)", zHigh))
				}
			}
		}

		lowStats := calculateRollingStats(rollingLows)
		if lowStats.Valid && lowStats.StdDev > 0 {
			zLow := (lowStats.Mean - float64(low)) / lowStats.StdDev
			if zLow > config.OutlierZScoreThreshold {
				isHighDropping := false
				if highStats.Valid && float64(high) < highStats.Mean {
					isHighDropping = true
				}
				if isHighDropping {
					marginalDiff := zLow - config.OutlierZScoreThreshold
					penaltyMultiplier := math.Exp(-(marginalDiff * config.OutlierPenaltyMultiplier))
					outlierTrendMultiplier *= penaltyMultiplier
					priceTrendIndicators = append(priceTrendIndicators, fmt.Sprintf("KC(%.1f)", zLow))
				} else {
					isGoldenMargin = true
					priceTrendIndicators = append(priceTrendIndicators, fmt.Sprintf("GM(%.1f)", zLow))
				}
			}
		}

		// 2c. Analyze Step-to-Step Volatility (Past Hour = First 12 ticks)
		var hourHighs, hourLows []float64
		for i := 0; i < len(rollingHighs) && i < 12; i++ {
			hourHighs = append(hourHighs, rollingHighs[i])
		}
		for i := 0; i < len(rollingLows) && i < 12; i++ {
			hourLows = append(hourLows, rollingLows[i])
		}

		midpointVol := calculateMidpointVolatility(hourHighs, hourLows)

		if midpointVol > config.VolatilityThresholdPercent {
			excessVol := midpointVol - config.VolatilityThresholdPercent
			// 4. Use safe exponential decay
			penalty := math.Exp(-(excessVol * config.VolatilityPenaltyMultiplier))
			outlierTrendMultiplier *= penalty
			priceTrendIndicators = append(priceTrendIndicators, fmt.Sprintf("JP(%.0f%%)", midpointVol*100))
		}

		spread := float64(high - low)
		rawSpread := rawHigh - float64(low)
		
		if len(hourHighs) >= 2 && len(hourLows) >= 2 {
			spreadJitter, spreadSpike, meanSpread, meanHigh := calculateSpreadJitterAndSpike(hourHighs, hourLows, rawSpread)

			if spreadJitter > config.SpreadJitterHighThreshold {
				excessJitter := spreadJitter - config.SpreadJitterHighThreshold
				penalty := math.Exp(-(excessJitter * config.SpreadJitterPenaltyMultiplier))
				outlierTrendMultiplier *= penalty
				priceTrendIndicators = append(priceTrendIndicators, fmt.Sprintf("SJP(%.0f%%)", spreadJitter*100))
			} else if spreadJitter < config.SpreadJitterLowThreshold {
				// Linear scaling reward: Max reward at 0 jitter, 1.0 at threshold
				denom := math.Max(config.SpreadJitterLowThreshold, 0.001)
				scale := (config.SpreadJitterLowThreshold - spreadJitter) / denom
				reward := 1.0 + (config.SpreadJitterRewardMultiplier-1.0)*scale
				outlierTrendMultiplier *= reward
				priceTrendIndicators = append(priceTrendIndicators, fmt.Sprintf("SS(%.0f%%)", spreadJitter*100))
			}

			if spreadSpike > config.SpreadSpikeThreshold && rawHigh > meanHigh && (rawSpread-meanSpread) > 5.0 && !isGoldenMargin {
				excessSpike := spreadSpike - config.SpreadSpikeThreshold
				penalty := math.Exp(-(excessSpike * config.SpreadSpikePenaltyMultiplier))
				outlierTrendMultiplier *= penalty
				priceTrendIndicators = append(priceTrendIndicators, fmt.Sprintf("SSP(%.1fx)", spreadSpike))
			}
		}

		// 3. Skip items with zero or negative spreads
		if spread <= 0 && !isTarget {
			continue
		}

		// 4. Calculate competitive spread buffer (max(5, 10% of spread))
		buffer := int64(spread / 10)
		if buffer < 5 {
			buffer = 5
		}

		highMod := high - buffer
		lowMod := low + buffer
		if spread <= 0 {
			highMod = high
			lowMod = low
		}

		// 5. Skip if buffer completely closes the spread
		if highMod <= lowMod && !isTarget {
			continue
		}

		// 6. Calculate tax (default 2% floor division, capped at 5M gp, 0 tax if under 50 gp)
		tax := int64(0)
		if highMod >= 50 {
			tax = int64(float64(highMod) * config.TaxRate)
			if tax > config.TaxCap {
				tax = config.TaxCap
			}
		}

		// 7. Calculate after-tax profit per item
		profitPerItem := highMod - tax - lowMod
		if profitPerItem <= 0 && !isTarget {
			continue
		}

		// 8. Calculate volumes (sum of buy and sell hourly volume)
		var volume int64
		if volData, ok := volumes[fmt.Sprintf("%d", id)]; ok {
			volume = volData.HighPriceVolume + volData.LowPriceVolume
		}
		
		var volume5m int64
		if volData, ok := vol5m[fmt.Sprintf("%d", id)]; ok {
			volume5m = volData.HighPriceVolume + volData.LowPriceVolume
		}
		
		var volume24h int64
		if volData, ok := vol24h[fmt.Sprintf("%d", id)]; ok {
			volume24h = volData.HighPriceVolume + volData.LowPriceVolume
		}

		// 9. Affordable limit based on Base Capital
		safeLowMod := math.Max(float64(lowMod), 1.0)
		affordableQty := int64(math.Floor(float64(config.BaseCapital) / safeLowMod))
		if affordableQty > int64(item.Limit) {
			affordableQty = int64(item.Limit)
		}
		if affordableQty <= 0 {
			if isTarget {
				affordableQty = 1
			} else {
				continue
			}
		}
		capitalRequired := lowMod * affordableQty

		// 10. Compute scoring factors
		potentialProfit := profitPerItem * affordableQty
		roi := (float64(profitPerItem) / safeLowMod) * 100.0
		
		// A. Total Profit Multiplier (Piecewise)
		profitRatio := float64(potentialProfit) / config.TargetProfitBenchmark
		var profitMultiplier float64
		if profitRatio < 1.0 {
			profitMultiplier = profitRatio
		} else {
			profitMultiplier = 1.0 + math.Log2(profitRatio)
		}
		profitMultiplier = math.Max(0.01, math.Min(config.ProfitRewardCap, profitMultiplier))
		if potentialProfit < 0 {
			profitMultiplier = 1.0
		}

		// B. Bounded ROI Multiplier
		rawROI := float64(profitPerItem) / safeLowMod
		roiMultiplier := math.Max(0.50, math.Min(config.ROIRewardCap, rawROI / config.TargetROI))

		// Volume Penalty Factors:
		// A. Volume Ratio Factor/Filter:
		// - Assume a 5% market capture rate
		volumeRatioFactor := 1.0
		limitVal := math.Max(float64(item.Limit), 1.0)
		volumeVal := float64(volume)
		globalRatio := (volumeVal * 4.0) / limitVal
		
		if globalRatio <= config.VolumeRatioFilterThreshold && !isTarget {
			continue // Filtered out completely by global volume ratio!
		} else if globalRatio < 1.0 {
			penalty := (1.0 - globalRatio) / (1.0 - config.VolumeRatioFilterThreshold)
			if penalty < 0 {
				penalty = 0
			}
			volumeRatioFactor = 1.0 - config.VolumeRatioPenaltyMax*(penalty*penalty)
			if volumeRatioFactor < 0.01 {
				volumeRatioFactor = 0.01
			}
		}

		// B. Absolute Volume Factor/Filter:
		// - If Volume <= config.MinAbsoluteVolume: completely filtered out
		// - If Volume >= 100: 1.0 (no penalty)
		absoluteVolumeFactor := 1.0
		minVol := float64(config.MinAbsoluteVolume)
		if volumeVal <= minVol && !isTarget {
			continue // Filtered out by absolute volume!
		} else if volumeVal < 100 {
			penalty := (100.0 - volumeVal) / (100.0 - minVol)
			if penalty < 0 {
				penalty = 0
			}
			absoluteVolumeFactor = 1.0 - config.AbsoluteVolumePenaltyMax*(penalty*penalty)
			if absoluteVolumeFactor < 0.01 {
				absoluteVolumeFactor = 0.01
			}
		}

		// Nudge multiplier from historical flips
		nudge := 1.0
		if val, ok := nudgeMultipliers[id]; ok {
			nudge = val
		}

		// Calculate trend penalties
		trendMultiplier := 1.0
		volumeSpikeIndicators := []string{}

		// 1-hour trend
		if h1, ok := hist1h[idStr]; ok {
			if avg1h, valid := getHistoricHigh(h1); valid && int64(rawHigh) < avg1h {
				trendMultiplier *= config.PriceTrendPenalty1h
				priceTrendIndicators = append(priceTrendIndicators, "↓1h")
			}
		}

		// 24-hour trend
		if h24, ok := hist24h[idStr]; ok {
			if avg24h, valid := getHistoricHigh(h24); valid && int64(rawHigh) < avg24h {
				trendMultiplier *= config.PriceTrendPenalty24h
				priceTrendIndicators = append(priceTrendIndicators, "↓24h")
			}
		}

		// (Removed old Z-Score Block)

		if len(priceTrendIndicators) == 0 {
			priceTrendIndicators = []string{"↗"}
		}

		spikeMultiplier := 1.0
		if volume > 0 {
			expected5m := float64(volume) / 12.0
			if float64(volume5m) > expected5m*3 && volume5m > 10 {
				spikeMultiplier *= config.VolumeSpike5mMultiplier
				volumeSpikeIndicators = append(volumeSpikeIndicators, "↑5m")
			}
		}

		if volume24h > 0 {
			expected1h := float64(volume24h) / 24.0
			if float64(volume) > expected1h*3 && volume > 50 {
				spikeMultiplier *= config.VolumeSpike24hMultiplier
				volumeSpikeIndicators = append(volumeSpikeIndicators, "↑1h")
			}
		}

		trendMultiplier *= outlierTrendMultiplier

		// Stale Price Penalty
		thresholdSeconds := int64(config.StalePriceThresholdMinutes * 60)
		if price.HighTime == nil || price.LowTime == nil || (runTs-*price.HighTime) > thresholdSeconds || (runTs-*price.LowTime) > thresholdSeconds {
			trendMultiplier *= config.StalePricePenaltyMultiplier
			priceTrendIndicators = append(priceTrendIndicators, "STALE")
		}

		// Final Score Calculation
		// Base * ProfitMultiplier * ROI * Volume * Nudges * Trend * Spikes
		score := float64(potentialProfit)
		multipliers := profitMultiplier * roiMultiplier * volumeRatioFactor * absoluteVolumeFactor * nudge * trendMultiplier * spikeMultiplier

		if score > 0 {
			score *= multipliers
		} else if multipliers > 0 {
			score /= multipliers
		}

		items = append(items, ReportItem{
			ID:              item.ID,
			Name:            item.Name,
			Icon:            item.Icon,
			BuyLimit:        item.Limit,
			High:            high,
			Low:             low,
			HighMod:         highMod,
			LowMod:          lowMod,
			Tax:             tax,
			ProfitPerItem:   profitPerItem,
			PotentialProfit: potentialProfit,
			CapitalRequired: capitalRequired,
			ROI:             roi,
			Volume:          volume,
			Score:                 score,
			ProfitMultiplier:      profitMultiplier,
			NudgeMultiplier:       nudge,
			TrendMultiplier:       trendMultiplier,
			PriceTrendIndicators:  priceTrendIndicators,
			VolumeSpikeIndicators: volumeSpikeIndicators,
			IsSink:                SinkItems[item.Name],
		})
	}

	// Sort by Score descending, then PotentialProfit descending
	sort.Slice(items, func(i, j int) bool {
		if items[i].Score != items[j].Score {
			return items[i].Score > items[j].Score
		}
		return items[i].PotentialProfit > items[j].PotentialProfit
	})

	return items
}

// getHistoricHigh calculates the average high price of a historical HourlyVolume record.
func getHistoricHigh(vol HourlyVolume) (int64, bool) {
	if vol.AvgHighPrice != nil {
		return *vol.AvgHighPrice, true
	}
	return 0, false
}

// GenerateMarkdownReport returns a beautifully formatted markdown string representing the report.
func GenerateMarkdownReport(items []ReportItem, timestamp int64, capThreshold, volThreshold int64, limit int) string {
	t := time.Unix(timestamp, 0).UTC()
	formattedTime := t.Format("2006-01-02 15:04:05 UTC")

	md := "# OSRS Grand Exchange Flip Analyzer Report\n\n"
	md += fmt.Sprintf("- **Generated At:** `%s` (Timestamp: `%d`)\n", formattedTime, timestamp)
	md += fmt.Sprintf("- **Configured Reference Capital ($K_{cap}$):** `%s gp` (Penalizes high-capital items)\n", formatCommas(capThreshold))
	md += fmt.Sprintf("- **Configured Reference Volume ($K_{vol}$):** `%d trades/hour` (Penalizes low-liquidity items)\n\n", volThreshold)

	md += "## Top Recommended Flips\n\n"
	md += "| Rank | Item Name | Score | Potential Profit | Profit/Item | Raw Spread | Adj Spread (Buy $\\to$ Sell) | Limit | Capital | ROI | Vol (hr) | Trend |\n"
	md += "| :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- |\n"

	displayLimit := limit
	if len(items) < displayLimit {
		displayLimit = len(items)
	}

	for i := 0; i < displayLimit; i++ {
		item := items[i]
		nameStr := item.Name
		if item.IsSink {
			nameStr = "**" + item.Name + "** `[SINK]`"
		}

		rawSpreadStr := fmt.Sprintf("%s / %s", formatCompact(item.Low), formatCompact(item.High))
		adjSpreadStr := fmt.Sprintf("%s $\\to$ %s", formatCompact(item.LowMod), formatCompact(item.HighMod))

		nudgeStr := ""
		if item.NudgeMultiplier != 1.0 {
			nudgeStr = fmt.Sprintf(" *(x%.2f)*", item.NudgeMultiplier)
		}

		trendStr := ""
		if len(item.PriceTrendIndicators) > 0 {
			for idx, ind := range item.PriceTrendIndicators {
				if idx > 0 {
					trendStr += " "
				}
				trendStr += ind
			}
		}
		if len(item.VolumeSpikeIndicators) > 0 {
			for _, ind := range item.VolumeSpikeIndicators {
				if trendStr != "" {
					trendStr += " "
				}
				trendStr += "⚠️" + ind + "-Spike"
			}
		}

		md += fmt.Sprintf("| %d | %s | %.1f%s | **%s** | %s | %s | %s | %s | %s | %.2f%% | %d | %s |\n",
			i+1,
			nameStr,
			item.Score,
			nudgeStr,
			formatCompact(item.PotentialProfit),
			formatCommas(item.ProfitPerItem),
			rawSpreadStr,
			adjSpreadStr,
			formatCommas(int64(item.BuyLimit)),
			formatCompact(item.CapitalRequired),
			item.ROI,
			item.Volume,
			trendStr,
		)
	}

	return md
}
