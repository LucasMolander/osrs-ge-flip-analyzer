package main

import (
	"fmt"
	"os"
	"sort"
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

// formatCompact formats large GP values into a compact string (e.g. 1.25M, 450K).
func formatCompact(n int64) string {
	absN := n
	sign := ""
	if n < 0 {
		absN = -n
		sign = "-"
	}
	if absN >= 1_000_000 {
		return fmt.Sprintf("%s%.2fM", sign, float64(absN)/1_000_000.0)
	}
	if absN >= 1_000 {
		return fmt.Sprintf("%s%.1fK", sign, float64(absN)/1_000.0)
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
) []ReportItem {
	var items []ReportItem

	for id, item := range metadata {
		// 1. Skip items with invalid buy limits (cannot flip or unknown)
		if item.Limit <= 0 {
			continue
		}

		// 2. Fetch price details
		price, ok := prices[fmt.Sprintf("%d", id)]
		if !ok || price.High == nil || price.Low == nil {
			continue // No transaction data
		}

		high := *price.High
		low := *price.Low
		spread := high - low

		// 3. Skip items with zero or negative spreads
		if spread <= 0 {
			continue
		}

		// 4. Calculate competitive spread buffer (max(5, 10% of spread))
		buffer := int64(spread / 10)
		if buffer < 5 {
			buffer = 5
		}

		highMod := high - buffer
		lowMod := low + buffer

		// 5. Skip if buffer completely closes the spread
		if highMod <= lowMod {
			continue
		}

		// 6. Calculate 2% tax (floor division, capped at 5M gp, 0 tax if under 50 gp)
		tax := int64(0)
		if highMod >= 50 {
			tax = int64(float64(highMod) * 0.02)
			if tax > 5_000_000 {
				tax = 5_000_000
			}
		}

		// 7. Calculate after-tax profit per item
		profitPerItem := highMod - tax - lowMod
		if profitPerItem <= 0 {
			continue
		}

		// 8. Calculate volumes (sum of buy and sell hourly volume)
		var volume int64
		if volData, ok := volumes[fmt.Sprintf("%d", id)]; ok {
			volume = volData.HighPriceVolume + volData.LowPriceVolume
		}

		// 9. Capital required for a full limit
		capitalRequired := lowMod * int64(item.Limit)

		// 10. Compute scoring factors
		potentialProfit := profitPerItem * int64(item.Limit)
		roi := (float64(profitPerItem) / float64(lowMod)) * 100.0

		// Capital Penalty Factor: K_cap / (K_cap + CapitalRequired)
		capitalFactor := float64(capitalThreshold) / float64(capitalThreshold+capitalRequired)

		// Volume Penalty Factor: Volume / (Volume + K_vol)
		volumeFactor := float64(volume) / float64(volume+volThreshold)

		// Nudge multiplier from historical flips
		nudge := 1.0
		if val, ok := nudgeMultipliers[id]; ok {
			nudge = val
		}

		// Calculate final score
		score := float64(potentialProfit) * capitalFactor * volumeFactor * nudge

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
			Score:           score,
			NudgeMultiplier: nudge,
			IsSink:          SinkItems[item.Name],
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

// GenerateMarkdownReport returns a beautifully formatted markdown string representing the report.
func GenerateMarkdownReport(items []ReportItem, timestamp int64, capThreshold, volThreshold int64, limit int) string {
	t := time.Unix(timestamp, 0).UTC()
	formattedTime := t.Format("2006-01-02 15:04:05 UTC")

	md := "# OSRS Grand Exchange Flip Analyzer Report\n\n"
	md += fmt.Sprintf("- **Generated At:** `%s` (Timestamp: `%d`)\n", formattedTime, timestamp)
	md += fmt.Sprintf("- **Configured Reference Capital ($K_{cap}$):** `%s gp` (Penalizes high-capital items)\n", formatCommas(capThreshold))
	md += fmt.Sprintf("- **Configured Reference Volume ($K_{vol}$):** `%d trades/hour` (Penalizes low-liquidity items)\n\n", volThreshold)

	md += "## Top Recommended Flips\n\n"
	md += "| Rank | Item Name | Score | Potential Profit | Profit/Item | Raw Spread | Adj Spread (Buy $\\to$ Sell) | Limit | Capital Req | ROI | Vol (hr) |\n"
	md += "| :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- |\n"

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

		md += fmt.Sprintf("| %d | %s | %.1f%s | **%s** | %s | %s | %s | %s | %s | %.2f%% | %d |\n",
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
		)
	}

	return md
}
