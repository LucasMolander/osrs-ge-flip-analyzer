package core

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

// FindLatestFile scans a directory and returns the path and timestamp of the latest file matching the prefix.
func FindLatestFile(dir, prefix string) (string, int64, error) {
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

// AnalyzePrices runs the analysis algorithm and returns a sorted slice of ReportItems.
func AnalyzePrices(
	prices map[string]LatestPrice,
	volumes map[string]HourlyVolume,
	metadata map[int]ItemMetadata,
	capitalThreshold int64,
	volThreshold int64,
	nudgeMultipliers map[int]float64,
	hist1h map[string]HourlyVolume,
	hist24h map[string]HourlyVolume,
	hist30d map[string]HourlyVolume,
	vol5m map[string]HourlyVolume,
	vol24h map[string]HourlyVolume,
	filterName string,
	config *RankingConfig,
) []ReportItem {
	var items []ReportItem

	for id, item := range metadata {
		isTarget := false
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
		spread := high - low

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

		// 9. Capital required for a full limit
		affordableQty := int64(item.Limit)
		if lowMod > 0 {
			maxAffordable := capitalThreshold / lowMod
			if maxAffordable < affordableQty {
				affordableQty = maxAffordable
			}
		}
		capitalRequired := lowMod * affordableQty

		// 10. Compute scoring factors
		potentialProfit := profitPerItem * affordableQty
		roiMultiplier := float64(profitPerItem) / float64(lowMod)
		roi := roiMultiplier * 100.0

		// Capital Penalty Factor: K_cap / (K_cap + CapitalRequired)
		capitalFactor := float64(capitalThreshold) / float64(capitalThreshold+capitalRequired)
		capitalFactor = config.CapitalPenaltyBaseWeight + config.CapitalPenaltyScaleWeight*capitalFactor

		// Volume Penalty Factors:
		// A. Volume Ratio Factor/Filter:
		// - If Volume >= Limit: 1.0 (no penalty)
		// - If Volume <= 0.1 * Limit: 0.0 (completely filtered out)
		volumeRatioFactor := 1.0
		limitVal := float64(item.Limit)
		volumeVal := float64(volume)
		projected4hVolume := volumeVal * 4.0
		if projected4hVolume <= 0.1*limitVal && !isTarget {
			continue // Filtered out by volume ratio!
		} else if projected4hVolume < limitVal {
			ratio := projected4hVolume / limitVal
			penalty := (1.0 - ratio) / 0.9
			if penalty < 0 {
				penalty = 0
			}
			volumeRatioFactor = 1.0 - config.VolumeRatioPenaltyMax*(penalty*penalty)
			if volumeRatioFactor < 0.01 {
				volumeRatioFactor = 0.01
			}
		}

		// B. Absolute Volume Factor/Filter:
		// - If Volume <= 10: completely filtered out
		// - If Volume >= 100: 1.0 (no penalty)
		absoluteVolumeFactor := 1.0
		if volumeVal <= 10 && !isTarget {
			continue // Filtered out by absolute volume!
		} else if volumeVal < 100 {
			penalty := (100.0 - volumeVal) / 90.0
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
		var priceTrendIndicators []string
		volumeSpikeIndicators := []string{}
		idStr := fmt.Sprintf("%d", id)

		// 1-hour trend
		if h1, ok := hist1h[idStr]; ok {
			if avg1h, valid := getHistoricAvg(h1); valid && highMod < avg1h {
				trendMultiplier *= config.PriceTrendPenalty1h
				priceTrendIndicators = append(priceTrendIndicators, "↓1h")
			}
		}

		// 24-hour trend
		if h24, ok := hist24h[idStr]; ok {
			if avg24h, valid := getHistoricAvg(h24); valid && highMod < avg24h {
				trendMultiplier *= config.PriceTrendPenalty24h
				priceTrendIndicators = append(priceTrendIndicators, "↓24h")
			}
		}

		// 30-day trend
		if h30, ok := hist30d[idStr]; ok {
			if avg30d, valid := getHistoricAvg(h30); valid && highMod < avg30d {
				trendMultiplier *= config.PriceTrendPenalty30d
				priceTrendIndicators = append(priceTrendIndicators, "↓30d")
			}
		}

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

		// Calculate final score
		score := float64(potentialProfit) * capitalFactor * volumeRatioFactor * absoluteVolumeFactor * nudge * trendMultiplier * spikeMultiplier * roiMultiplier

		items = append(items, ReportItem{
			ID:              item.ID,
			Name:            item.Name,
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

// getHistoricAvg calculates the average price of a historical HourlyVolume record.
func getHistoricAvg(vol HourlyVolume) (int64, bool) {
	if vol.AvgHighPrice != nil && vol.AvgLowPrice != nil {
		return (*vol.AvgHighPrice + *vol.AvgLowPrice) / 2, true
	} else if vol.AvgHighPrice != nil {
		return *vol.AvgHighPrice, true
	} else if vol.AvgLowPrice != nil {
		return *vol.AvgLowPrice, true
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
