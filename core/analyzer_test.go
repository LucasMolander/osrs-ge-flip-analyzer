package core

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"testing"
	"time"
)

func TestFormatCommas(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{0, "0"},
		{123, "123"},
		{1234, "1,234"},
		{1234567, "1,234,567"},
		{-12345, "-12,345"},
	}

	for _, tt := range tests {
		actual := formatCommas(tt.input)
		if actual != tt.expected {
			t.Errorf("formatCommas(%d): expected %q, got %q", tt.input, tt.expected, actual)
		}
	}
}

func TestFormatCompact(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{0, "0"},
		{500, "500"},
		{1200, "1200"},
		{45000, "45000"},
		{100000, "100.00K"},
		{300230, "300.23K"},
		{1250000, "1.250M"},
		{500000000, "500.000M"},
		{-1250000, "-1.250M"},
	}

	for _, tt := range tests {
		actual := formatCompact(tt.input)
		if actual != tt.expected {
			t.Errorf("formatCompact(%d): expected %q, got %q", tt.input, tt.expected, actual)
		}
	}
}

func TestAnalyzePrices_TaxAndBuffer(t *testing.T) {
	// Setup test data
	metadata := map[int]ItemMetadata{
		1: {ID: 1, Name: "Cheap Item", Limit: 1000},
		2: {ID: 2, Name: "Standard Item", Limit: 1000},
		3: {ID: 3, Name: "Expensive Item", Limit: 10},
		4: {ID: 4, Name: "Narrow Spread Item", Limit: 100},
		5: {ID: 5, Name: "Twisted bow", Limit: 8}, // Sink item
	}

	// 1. Cheap Item (HighMod < 50, so tax should be 0)
	// Raw spread = 49 - 39 = 10. Buffer = max(5, 1) = 5.
	// HighMod = 49 - 5 = 44. LowMod = 39 + 5 = 44.
	// HighMod <= LowMod, so cheap item should be filtered out.
	// Let's adjust prices so HighMod is 49 and LowMod is 40.
	// Raw: High = 55, Low = 34. Spread = 21. Buffer = max(5, 2) = 5.
	// HighMod = 50. LowMod = 39.
	// HighMod >= 50, so tax is floor(50 * 0.02) = 1. Profit = 50 - 1 - 39 = 10.
	// If HighMod was 49 (High = 54, Low = 34, Buffer = 5, HighMod = 49, LowMod = 39):
	// Tax should be 0 because HighMod < 50. Profit = 49 - 0 - 39 = 10.

	hCheapHigh := int64(54)
	hCheapLow := int64(34)

	// 2. Standard Item
	// High = 1000, Low = 800. Spread = 200. Buffer = max(5, 20) = 20.
	// HighMod = 980. LowMod = 820.
	// Tax = floor(980 * 0.02) = 19.
	// Profit per item = 980 - 19 - 820 = 141.
	hStdHigh := int64(1000)
	hStdLow := int64(800)

	// 3. Expensive Item (Hits 5M tax cap)
	// High = 350,000,000. Low = 340,000,000. Spread = 10,000,000. Buffer = max(5, 1,000,000) = 1,000,000.
	// HighMod = 349,000,000. LowMod = 341,000,000.
	// Tax = floor(349,000,000 * 0.02) = 6,980,000 -> capped at 5,000,000.
	// Profit per item = 349,000,000 - 5,000,000 - 341,000,000 = 3,000,000.
	hExpHigh := int64(350_000_000)
	hExpLow := int64(340_000_000)

	// 4. Narrow Spread Item (Filtered out)
	// High = 100, Low = 95. Spread = 5. Buffer = max(5, 0) = 5.
	// HighMod = 95. LowMod = 100.
	// HighMod <= LowMod, so filtered out.
	hNarrowHigh := int64(100)
	hNarrowLow := int64(95)

	// 5. Twisted bow (Sink Item)
	// High = 1,200,000,000. Low = 1,190,000,000. Spread = 10,000,000. Buffer = 1,000,000.
	// HighMod = 1,199,000,000. LowMod = 1,191,000,000.
	// Tax = 5,000,000 (capped). Profit = 1,199M - 5M - 1,191M = 3,000,000.
	hTbowHigh := int64(1_200_000_000)
	hTbowLow := int64(1_190_000_000)

	prices := map[string]LatestPrice{
		"1": {High: &hCheapHigh, Low: &hCheapLow},
		"2": {High: &hStdHigh, Low: &hStdLow},
		"3": {High: &hExpHigh, Low: &hExpLow},
		"4": {High: &hNarrowHigh, Low: &hNarrowLow},
		"5": {High: &hTbowHigh, Low: &hTbowLow},
	}

	// High volumes to ignore volume penalty (VolumeFactor ~ 1.0)
	volumes := map[string]HourlyVolume{
		"1": {HighPriceVolume: 10000, LowPriceVolume: 10000},
		"2": {HighPriceVolume: 10000, LowPriceVolume: 10000},
		"3": {HighPriceVolume: 10000, LowPriceVolume: 10000},
		"4": {HighPriceVolume: 10000, LowPriceVolume: 10000},
		"5": {HighPriceVolume: 10000, LowPriceVolume: 10000},
	}

	// High capital threshold to ignore capital penalty (CapitalFactor ~ 1.0)
	capitalThreshold := int64(1_000_000_000_000) // 1 Trillion gp
	volThreshold := int64(1)
	nudges := make(map[int]float64)

	report := AnalyzePrices(prices, volumes, metadata, capitalThreshold, volThreshold, nudges, nil, nil, nil)

	// We expect 4 items (Cheap Item, Standard Item, Expensive Item, Twisted Bow). Narrow Spread Item is filtered.
	if len(report) != 4 {
		t.Fatalf("Expected 4 items in report, got %d", len(report))
	}

	// Let's create a lookup map of the results
	resMap := make(map[int]ReportItem)
	for _, item := range report {
		resMap[item.ID] = item
	}

	// Verify Cheap Item (ID 1)
	cheapItem, ok := resMap[1]
	if !ok {
		t.Error("Cheap Item not found in report")
	} else {
		if cheapItem.HighMod != 49 {
			t.Errorf("Cheap Item HighMod: expected 49, got %d", cheapItem.HighMod)
		}
		if cheapItem.LowMod != 39 {
			t.Errorf("Cheap Item LowMod: expected 39, got %d", cheapItem.LowMod)
		}
		if cheapItem.Tax != 0 {
			t.Errorf("Cheap Item Tax: expected 0, got %d (rounding rule failed)", cheapItem.Tax)
		}
		if cheapItem.ProfitPerItem != 10 {
			t.Errorf("Cheap Item ProfitPerItem: expected 10, got %d", cheapItem.ProfitPerItem)
		}
	}

	// Verify Standard Item (ID 2)
	stdItem, ok := resMap[2]
	if !ok {
		t.Error("Standard Item not found in report")
	} else {
		if stdItem.HighMod != 980 || stdItem.LowMod != 820 {
			t.Errorf("Standard Item HighMod/LowMod: expected 980/820, got %d/%d", stdItem.HighMod, stdItem.LowMod)
		}
		if stdItem.Tax != 19 {
			t.Errorf("Standard Item Tax: expected 19, got %d", stdItem.Tax)
		}
		if stdItem.ProfitPerItem != 141 {
			t.Errorf("Standard Item ProfitPerItem: expected 141, got %d", stdItem.ProfitPerItem)
		}
	}

	// Verify Expensive Item (ID 3)
	expItem, ok := resMap[3]
	if !ok {
		t.Error("Expensive Item not found in report")
	} else {
		if expItem.Tax != 5_000_000 {
			t.Errorf("Expensive Item Tax: expected 5,000,000 (cap), got %d", expItem.Tax)
		}
		if expItem.ProfitPerItem != 3_000_000 {
			t.Errorf("Expensive Item ProfitPerItem: expected 3,000,000, got %d", expItem.ProfitPerItem)
		}
	}

	// Verify Twisted Bow (ID 5) - should be identified as a Sink Item
	tbowItem, ok := resMap[5]
	if !ok {
		t.Error("Twisted Bow not found in report")
	} else {
		if !tbowItem.IsSink {
			t.Error("Twisted Bow: expected IsSink to be true, got false")
		}
	}
}

func TestAnalyzePrices_Heuristics(t *testing.T) {
	// Standard metadata for two identical items
	metadata := map[int]ItemMetadata{
		1: {ID: 1, Name: "Item A", Limit: 100},
		2: {ID: 2, Name: "Item B", Limit: 100},
	}

	hHigh := int64(1000)
	hLow := int64(800)

	prices := map[string]LatestPrice{
		"1": {High: &hHigh, Low: &hLow},
		"2": {High: &hHigh, Low: &hLow},
	}

	// Test 1: Volume Penalty Heuristic
	// Item A has high volume (100) equal to its limit (100), Item B has low volume (20) relative to its limit (100)
	volumes := map[string]HourlyVolume{
		"1": {HighPriceVolume: 50, LowPriceVolume: 50}, // Volume = 100
		"2": {HighPriceVolume: 10, LowPriceVolume: 10}, // Volume = 20
	}

	// Large capital threshold to isolate volume effect
	capThreshold := int64(1_000_000_000)
	volThreshold := int64(10)
	nudges := make(map[int]float64)

	report := AnalyzePrices(prices, volumes, metadata, capThreshold, volThreshold, nudges, nil, nil, nil)

	if len(report) != 2 {
		t.Fatalf("Expected 2 items in report, got %d", len(report))
	}

	// Item 1 (high volume) should be ranked higher than Item 2 (low volume)
	if report[0].ID != 1 || report[1].ID != 2 {
		t.Errorf("Volume heuristic sorting failed. Expected order: [1, 2], got: [%d, %d]", report[0].ID, report[1].ID)
	}

	// Verify volume factors
	// Item A: Volume (100) >= Limit (100) -> Factor = 1.0
	// Item B: Volume (20) < Limit (100) -> Ratio = 0.2 -> Penalty = 1.30 * (0.8/0.9)^2 = 1.0271 (capped to 0.0) -> Factor = 0.0
	factorA := report[0].Score / float64(report[0].PotentialProfit)
	factorB := report[1].Score / float64(report[1].PotentialProfit)
	if factorA <= factorB {
		t.Errorf("Expected volume factor for Item A (%f) to be greater than Item B (%f)", factorA, factorB)
	}

	// Test 2: Capital Penalty Heuristic
	// We will create two items with identical potential profits, but different capital requirements
	// Item C: Low price = 10, Limit = 1000 -> Capital Required = 10,000 gp. Potential Profit = 5 * 1000 = 5000 gp.
	// Item D: Low price = 10,000, Limit = 1 -> Capital Required = 10,000 gp. Potential Profit = 5000 * 1 = 5000 gp.
	// Wait, to keep potential profit the same, let's configure them:
	metadataCap := map[int]ItemMetadata{
		3: {ID: 3, Name: "Low Capital Item", Limit: 1000},
		4: {ID: 4, Name: "High Capital Item", Limit: 1},
	}

	// High Capital Item requires 11,000 gp capital for 1 item.
	// Low Capital Item requires 10,000 gp capital for 1000 items.
	// If capital threshold is small (e.g., 5,000 gp), the Capital Penalty will favor the one requiring less capital.
	// Let's write standard prices that produce identical potential profit:
	// Let's verify:
	// Item 3: buy = 100, sell = 160. spread = 60. buffer = 6. highMod = 154. lowMod = 106. Tax = 3. profit_per_ea = 45. limit = 100.
	// Capital required = 10,600. Potential profit = 4,500.
	// Item 4: buy = 10,000, sell = 16,000. spread = 6000. buffer = 600. highMod = 15400. lowMod = 10600. Tax = 308. profit_per_ea = 4492. limit = 1.
	// Capital required = 10,600. Potential profit = 4,492.
	// Wait, capital required is identical! That doesn't test capital penalty differences.
	// Let's change the limits!
	// Item 3 (Low Cap): buy = 100, sell = 160. limit = 100. Capital = 10,600. Profit = 4,500.
	// Item 4 (High Cap): buy = 10,000, sell = 16,000. limit = 10. Capital = 106,000. Profit = 44,920.
	// Raw potential profit of Item 4 is almost 10 times higher.
	// But if K_cap is very small, say 5,000 gp, then the Capital Penalty for Item 4 is:
	// 5,000 / (5,000 + 106,000) = 0.045 -> Score = 44,920 * 0.045 = 2023.
	// Capital Penalty for Item 3 is:
	// 5,000 / (5,000 + 10,600) = 0.320 -> Score = 4,500 * 0.320 = 1442.
	// Wait, Item 4 is still higher.
	// Let's make K_cap even smaller, or make the capital requirement difference even larger!
	// Item 4: limit = 100 -> Capital = 1,060,000. Profit = 449,200.
	// Capital penalty = 5,000 / (5,000 + 1,060,000) = 0.00469. Score = 449,200 * 0.00469 = 2108.
	// What if we set K_cap to 1,000 gp?
	// Item 3: 1,000 / (1,000 + 10,600) = 0.0862. Score = 4,500 * 0.0862 = 387.
	// Item 4: 1,000 / (1,000 + 1,060,000) = 0.000942. Score = 449,200 * 0.000942 = 423.
	// Let's just assert that the Score of Item 4 is heavily suppressed compared to its raw Potential Profit due to the Capital Penalty.
	// Specifically, without capital penalty (K_cap = 10,000,000), Item 4 is ranked 1st by far.
	// With capital penalty (K_cap = 1,000), Item 4's score is scaled down by a factor of ~1000, while Item 3 is scaled down by a factor of ~11.
	// Let's write a direct test for the Capital Penalty Factor!
	// CapitalRequired = 100,000. K_cap = 100,000. CapitalFactor = 100K / (100K + 100K) = 0.5.
	// CapitalRequired = 900,000. K_cap = 100,000. CapitalFactor = 100K / (100K + 900K) = 0.1.
	// This math is extremely easy to verify. Let's write tests verifying that the calculated scores match the formula!

	h3High := int64(160)
	h3Low := int64(100)
	h4High := int64(16000)
	h4Low := int64(10000)

	pricesCap := map[string]LatestPrice{
		"3": {High: &h3High, Low: &h3Low},
		"4": {High: &h4High, Low: &h4Low},
	}
	volumesCap := map[string]HourlyVolume{
		"3": {HighPriceVolume: 10000, LowPriceVolume: 10000},
		"4": {HighPriceVolume: 10000, LowPriceVolume: 10000},
	}

	capThresholdTest := int64(10000) // K_cap = 10,000 gp
	volThresholdTest := int64(1)

	reportCap := AnalyzePrices(pricesCap, volumesCap, metadataCap, capThresholdTest, volThresholdTest, nudges, nil, nil, nil)

	// Item 3: CapitalReq = 106 * 1000 = 106,000 gp. Potential Profit = 45 * 1000 = 45,000.
	// CapitalFactor = 10,000 / (10,000 + 106,000) = 0.08620689
	// VolumeFactor = 20000 / (20000 + 1) = 0.99995
	// Expected Score = 45,000 * 0.08620689 * 0.99995 = 3879.11

	// Item 4: CapitalReq = 10,600 * 1 = 10,600 gp. Potential Profit = 4,492 * 1 = 4,492.
	// CapitalFactor = 10,000 / (10,000 + 10,600) = 0.48543689
	// VolumeFactor = 20000 / (20000 + 1) = 0.99995
	// Expected Score = 4,492 * 0.48543689 * 0.99995 = 2180.47

	resMapCap := make(map[int]ReportItem)
	for _, item := range reportCap {
		resMapCap[item.ID] = item
	}

	item3 := resMapCap[3]
	expectedScore3 := 45000.0 * (0.5 + 0.5*(10000.0/(10000.0+106000.0)))
	if math.Abs(item3.Score-expectedScore3) > 0.001 {
		t.Errorf("Item 3 score: expected %f, got %f", expectedScore3, item3.Score)
	}

	item4 := resMapCap[4]
	expectedScore4 := 4492.0 * (0.5 + 0.5*(10000.0/(10000.0+10600.0)))
	if math.Abs(item4.Score-expectedScore4) > 0.001 {
		t.Errorf("Item 4 score: expected %f, got %f", expectedScore4, item4.Score)
	}
}

func TestAnalyzePrices_Nudge(t *testing.T) {
	metadata := map[int]ItemMetadata{
		1: {ID: 1, Name: "Standard Item", Limit: 1000},
	}

	hHigh := int64(1000)
	hLow := int64(800)

	prices := map[string]LatestPrice{
		"1": {High: &hHigh, Low: &hLow},
	}
	volumes := map[string]HourlyVolume{
		"1": {HighPriceVolume: 10000, LowPriceVolume: 10000},
	}

	nudges := map[int]float64{
		1: 1.5, // 50% boost from successful history
	}

	report := AnalyzePrices(prices, volumes, metadata, 1000000000, 1, nudges, nil, nil, nil)

	if len(report) != 1 {
		t.Fatalf("Expected 1 item in report, got %d", len(report))
	}

	item := report[0]
	if item.NudgeMultiplier != 1.5 {
		t.Errorf("Expected NudgeMultiplier to be 1.5, got %f", item.NudgeMultiplier)
	}

	// Verify score is multiplied by 1.5
	// Raw potential profit = 141 * 1000 = 141,000
	// Without nudge, Score ~ 141,000 (since factors are ~1.0)
	// With nudge, Score should be ~ 141,000 * 1.5 = 211,500
	expectedScore := 141000.0 * 1.5
	if math.Abs(item.Score-expectedScore) > 1000.0 {
		t.Errorf("Expected score to be around %f, got %f", expectedScore, item.Score)
	}
}

func TestAnalyzePrices_TrendPenalties(t *testing.T) {
	// Setup metadata: 5 items
	// Item 1: Stable/Rising (no penalty)
	// Item 2: Down 1h (0.80 penalty)
	// Item 3: Down 24h (0.90 penalty)
	// Item 4: Down 30d (0.95 penalty)
	// Item 5: Down in all three (0.80 * 0.90 * 0.95 = 0.684 penalty)
	metadata := map[int]ItemMetadata{
		1: {ID: 1, Name: "Stable Item", Limit: 1000},
		2: {ID: 2, Name: "Down 1h Item", Limit: 1000},
		3: {ID: 3, Name: "Down 24h Item", Limit: 1000},
		4: {ID: 4, Name: "Down 30d Item", Limit: 1000},
		5: {ID: 5, Name: "Down All Item", Limit: 1000},
	}

	// For all items, Raw Prices are: High = 1000, Low = 800.
	// Average price is (1000+800)/2 = 900.
	// CurrentPrice = 900.
	hHigh := int64(1000)
	hLow := int64(800)
	prices := map[string]LatestPrice{
		"1": {High: &hHigh, Low: &hLow},
		"2": {High: &hHigh, Low: &hLow},
		"3": {High: &hHigh, Low: &hLow},
		"4": {High: &hHigh, Low: &hLow},
		"5": {High: &hHigh, Low: &hLow},
	}

	volumes := map[string]HourlyVolume{
		"1": {HighPriceVolume: 10000, LowPriceVolume: 10000},
		"2": {HighPriceVolume: 10000, LowPriceVolume: 10000},
		"3": {HighPriceVolume: 10000, LowPriceVolume: 10000},
		"4": {HighPriceVolume: 10000, LowPriceVolume: 10000},
		"5": {HighPriceVolume: 10000, LowPriceVolume: 10000},
	}

	// Helper to build HourlyVolume with given average price
	hVol := func(avg int64) HourlyVolume {
		return HourlyVolume{
			AvgHighPrice:    &avg,
			AvgLowPrice:     &avg,
			HighPriceVolume: 100,
			LowPriceVolume:  100,
		}
	}

	// 1h ago: Item 2 and 5 are down (historical avg 1000 > currentPrice 900)
	hist1h := map[string]HourlyVolume{
		"1": hVol(900),  // stable
		"2": hVol(1000), // down
		"3": hVol(900),  // stable
		"4": hVol(900),  // stable
		"5": hVol(1000), // down
	}

	// 24h ago: Item 3 and 5 are down
	hist24h := map[string]HourlyVolume{
		"1": hVol(900),
		"2": hVol(900),
		"3": hVol(1000),
		"4": hVol(900),
		"5": hVol(1000),
	}

	// 30d ago: Item 4 and 5 are down
	hist30d := map[string]HourlyVolume{
		"1": hVol(900),
		"2": hVol(900),
		"3": hVol(900),
		"4": hVol(1000),
		"5": hVol(1000),
	}

	nudges := make(map[int]float64)

	// Run analyzer
	report := AnalyzePrices(prices, volumes, metadata, 1000000000, 1, nudges, hist1h, hist24h, hist30d)

	if len(report) != 5 {
		t.Fatalf("Expected 5 items in report, got %d", len(report))
	}

	// Map results for verification
	res := make(map[int]ReportItem)
	for _, item := range report {
		res[item.ID] = item
	}

	// Verify Item 1 (Stable): TrendMultiplier = 1.0, Indicators = ["↗"]
	if res[1].TrendMultiplier != 1.0 || len(res[1].TrendIndicators) != 1 || res[1].TrendIndicators[0] != "↗" {
		t.Errorf("Item 1 stable failed: TrendMultiplier=%f, Indicators=%v", res[1].TrendMultiplier, res[1].TrendIndicators)
	}

	// Verify Item 2 (Down 1h): TrendMultiplier = 0.80, Indicators = ["↓1h"]
	if math.Abs(res[2].TrendMultiplier-0.80) > 1e-9 || len(res[2].TrendIndicators) != 1 || res[2].TrendIndicators[0] != "↓1h" {
		t.Errorf("Item 2 down 1h failed: TrendMultiplier=%f, Indicators=%v", res[2].TrendMultiplier, res[2].TrendIndicators)
	}

	// Verify Item 3 (Down 24h): TrendMultiplier = 0.90, Indicators = ["↓24h"]
	if math.Abs(res[3].TrendMultiplier-0.90) > 1e-9 || len(res[3].TrendIndicators) != 1 || res[3].TrendIndicators[0] != "↓24h" {
		t.Errorf("Item 3 down 24h failed: TrendMultiplier=%f, Indicators=%v", res[3].TrendMultiplier, res[3].TrendIndicators)
	}

	// Verify Item 4 (Down 30d): TrendMultiplier = 0.95, Indicators = ["↓30d"]
	if math.Abs(res[4].TrendMultiplier-0.95) > 1e-9 || len(res[4].TrendIndicators) != 1 || res[4].TrendIndicators[0] != "↓30d" {
		t.Errorf("Item 4 down 30d failed: TrendMultiplier=%f, Indicators=%v", res[4].TrendMultiplier, res[4].TrendIndicators)
	}

	// Verify Item 5 (Down All): TrendMultiplier = 0.80 * 0.90 * 0.95 = 0.684, Indicators = ["↓1h", "↓24h", "↓30d"]
	expectedAll := 0.80 * 0.90 * 0.95
	if math.Abs(res[5].TrendMultiplier-expectedAll) > 1e-9 || len(res[5].TrendIndicators) != 3 ||
		res[5].TrendIndicators[0] != "↓1h" || res[5].TrendIndicators[1] != "↓24h" || res[5].TrendIndicators[2] != "↓30d" {
		t.Errorf("Item 5 down all failed: TrendMultiplier=%f, Indicators=%v", res[5].TrendMultiplier, res[5].TrendIndicators)
	}

	// Verify sorting order: Item 1 (Score factor 1.0) > Item 4 (0.95) > Item 3 (0.90) > Item 2 (0.80) > Item 5 (0.684)
	if report[0].ID != 1 || report[1].ID != 4 || report[2].ID != 3 || report[3].ID != 2 || report[4].ID != 5 {
		t.Errorf("Trend penalty sorting failed. Expected order: [1, 4, 3, 2, 5], got: [%d, %d, %d, %d, %d]",
			report[0].ID, report[1].ID, report[2].ID, report[3].ID, report[4].ID)
	}
}

func TestAnalyzePrices_AbsoluteVolumeFilter(t *testing.T) {
	// Setup metadata: 4 items
	// Buy Limit is 10, so volume ratio filters are bypassed (since volume > 0.1 * limit)
	metadata := map[int]ItemMetadata{
		1: {ID: 1, Name: "Below 10 Vol Item", Limit: 10},
		2: {ID: 2, Name: "At 10 Vol Item", Limit: 10},
		3: {ID: 3, Name: "At 55 Vol Item", Limit: 10},
		4: {ID: 4, Name: "At 100 Vol Item", Limit: 10},
	}

	hHigh := int64(1000)
	hLow := int64(800)
	prices := map[string]LatestPrice{
		"1": {High: &hHigh, Low: &hLow},
		"2": {High: &hHigh, Low: &hLow},
		"3": {High: &hHigh, Low: &hLow},
		"4": {High: &hHigh, Low: &hLow},
	}

	// Volumes:
	// Item 1: Volume = 5 (filtered out!)
	// Item 2: Volume = 10 (filtered out!)
	// Item 3: Volume = 55 (Factor = 1.0 - 1.30 * 0.5^2 = 0.675)
	// Item 4: Volume = 100 (Factor = 1.0)
	volumes := map[string]HourlyVolume{
		"1": {HighPriceVolume: 2, LowPriceVolume: 3},   // Vol = 5
		"2": {HighPriceVolume: 5, LowPriceVolume: 5},   // Vol = 10
		"3": {HighPriceVolume: 25, LowPriceVolume: 30}, // Vol = 55
		"4": {HighPriceVolume: 50, LowPriceVolume: 50}, // Vol = 100
	}

	nudges := make(map[int]float64)

	// Run analyzer
	report := AnalyzePrices(prices, volumes, metadata, 1000000000, 1, nudges, nil, nil, nil)

	// We expect only 2 items (Item 3 and 4) in the report. Item 1 and 2 are filtered out.
	if len(report) != 2 {
		t.Fatalf("Expected 2 items in report, got %d", len(report))
	}

	// Map results
	res := make(map[int]ReportItem)
	for _, item := range report {
		res[item.ID] = item
	}

	// Verify Item 4 has no absolute volume penalty (Volume factor ~ 1.0)
	// Potential profit = 141 * 10 = 1410
	if math.Abs(res[4].Score-1410.0) > 10.0 {
		t.Errorf("Item 4 score failed: expected ~1410, got %f", res[4].Score)
	}

	// Verify Item 3 has a 32.5% absolute volume penalty (Score ~ 1410 * 0.675 = 951.75)
	expectedScore3 := 1410.0 * 0.675
	if math.Abs(res[3].Score-expectedScore3) > 10.0 {
		t.Errorf("Item 3 score failed: expected ~%f, got %f", expectedScore3, res[3].Score)
	}

	// Verify sorting order: Item 4 (Score ~1410) > Item 3 (Score ~1057)
	if report[0].ID != 4 || report[1].ID != 3 {
		t.Errorf("Absolute volume sorting failed. Expected order: [4, 3], got: [%d, %d]", report[0].ID, report[1].ID)
	}
}

func TestLoadNudges_FlipsAndFailedBuys(t *testing.T) {
	// 1. Create a temporary local storage directory
	tempDir, err := os.MkdirTemp("", "ge-analyzer-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Save original working directory and restore it
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	defer os.Chdir(oldWD)

	// Change to temporary directory so relative file operations occur inside it
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Failed to change working directory: %v", err)
	}

	// Save original store and restore it after test
	oldStore := Store
	defer func() { Store = oldStore }()

	// Set global store to LocalStorage
	Store = &LocalStorage{}

	// 2. Create mock directories
	if err := os.MkdirAll("flips", 0755); err != nil {
		t.Fatalf("Failed to create flips dir: %v", err)
	}
	if err := os.MkdirAll("failed_buys", 0755); err != nil {
		t.Fatalf("Failed to create failed_buys dir: %v", err)
	}

	now := time.Now()

	// 3. Write a successful flip for Item 1 (good flip: profit > 0)
	// Price: buy = 100, sell = 120. Profit = 120 - 2 (tax) - 100 = 18 > 0.
	// Direction = +0.10.
	// Timestamp = now (age = 0, weight = 1.0)
	flip1 := FlipRecord{
		ItemID:    1,
		Quantity:  100,
		BuyPrice:  100,
		SellPrice: 120,
		Timestamp: now,
	}
	if err := saveJSONTest("flips", "flip_1", now.Unix(), flip1); err != nil {
		t.Fatalf("Failed to save flip1: %v", err)
	}

	// 4. Write a failed buy for Item 1
	// Target = 1000, Bought = 0 (100% failure).
	// Direction = -0.40 * 1.0 = -0.40.
	// Timestamp = now (age = 0, weight = 1.0)
	failed1 := FailedBuyRecord{
		ItemID:    1,
		TargetQty: 1000,
		BoughtQty: 0,
		BuyPrice:  100,
		Timestamp: now,
	}
	if err := saveJSONTest("failed_buys", "failed_buy_1", now.Unix(), failed1); err != nil {
		t.Fatalf("Failed to save failed1: %v", err)
	}

	// 5. Write a decayed failed buy for Item 2
	// Target = 1000, Bought = 500 (50% failure).
	// Base direction = -0.40 * 0.5 = -0.20.
	// Timestamp = now - 3 days (age = 3 days = 1 half-life, weight = 0.5)
	// Expected direction = -0.20 * 0.5 = -0.10.
	failed2 := FailedBuyRecord{
		ItemID:    2,
		TargetQty: 1000,
		BoughtQty: 500,
		BuyPrice:  100,
		Timestamp: now.Add(-3 * 24 * time.Hour),
	}
	if err := saveJSONTest("failed_buys", "failed_buy_2", now.Add(-3*24*time.Hour).Unix(), failed2); err != nil {
		t.Fatalf("Failed to save failed2: %v", err)
	}

	// 6. Call LoadNudges()
	multipliers, err := LoadNudges()
	if err != nil {
		t.Fatalf("loadNudges failed: %v", err)
	}

	// 7. Verify multipliers
	// Item 1:
	// Net Nudge = flip nudge (+0.10) + failed buy nudge (-0.40) = -0.30
	// Expected Multiplier = 1.0 - 0.30 = 0.70
	mult1, ok := multipliers[1]
	if !ok {
		t.Errorf("Expected multiplier for Item 1, but found none")
	} else if math.Abs(mult1-0.70) > 0.01 {
		t.Errorf("Item 1 multiplier: expected 0.70, got %f", mult1)
	}

	// Item 2:
	// Net Nudge = failed buy nudge decayed (-0.10) = -0.10
	// Expected Multiplier = 1.0 - 0.10 = 0.90
	mult2, ok := multipliers[2]
	if !ok {
		t.Errorf("Expected multiplier for Item 2, but found none")
	} else if math.Abs(mult2-0.90) > 0.01 {
		t.Errorf("Item 2 multiplier: expected 0.90, got %f", mult2)
	}
}

// Helper to write JSON files in testing without using saveJSON (which might lock or write to production dirs)
func saveJSONTest(dir, prefix string, ts int64, data interface{}) error {
	path := fmt.Sprintf("%s/%s_%d.json", dir, prefix, ts)
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return json.NewEncoder(file).Encode(data)
}

func TestBackupAndRestore(t *testing.T) {
	// 1. Create temporary directory
	tempDir, err := os.MkdirTemp("", "ge-analyzer-backup-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Save original working directory and restore it
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	defer os.Chdir(oldWD)

	// Change to temporary directory to isolate storage operations
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Failed to change working directory: %v", err)
	}

	// Save original store and restore it after test
	oldStore := Store
	defer func() { Store = oldStore }()

	// Set global store to LocalStorage
	Store = &LocalStorage{}

	// 2. Create mock directories and write some test files
	if err := os.MkdirAll("flips", 0755); err != nil {
		t.Fatalf("Failed to create flips dir: %v", err)
	}
	if err := os.MkdirAll("failed_buys", 0755); err != nil {
		t.Fatalf("Failed to create failed_buys dir: %v", err)
	}
	if err := os.MkdirAll("reports", 0755); err != nil {
		t.Fatalf("Failed to create reports dir: %v", err)
	}

	flipContent := []byte(`{"ItemID": 1, "Quantity": 100}`)
	failedContent := []byte(`{"ItemID": 2, "TargetQty": 500}`)
	reportContent := []byte("# Test Report\n- Item 1\n- Item 2")

	if err := Store.WriteRaw("flips/flip_1.json", flipContent); err != nil {
		t.Fatalf("Failed to write flip: %v", err)
	}
	if err := Store.WriteRaw("failed_buys/failed_buy_2.json", failedContent); err != nil {
		t.Fatalf("Failed to write failed buy: %v", err)
	}
	if err := Store.WriteRaw("reports/report_latest.md", reportContent); err != nil {
		t.Fatalf("Failed to write report: %v", err)
	}

	// 3. Trigger backup
	backupJSON, err := BackupData()
	if err != nil {
		t.Fatalf("BackupData failed: %v", err)
	}

	// 4. Verify backup JSON contains all files and correct contents
	var payload BackupPayload
	if err := json.Unmarshal(backupJSON, &payload); err != nil {
		t.Fatalf("Failed to unmarshal backup JSON: %v", err)
	}

	if payload.Version != 1 {
		t.Errorf("Expected version 1, got %d", payload.Version)
	}

	if len(payload.Files) != 3 {
		t.Errorf("Expected 3 files in backup, got %d", len(payload.Files))
	}

	// Verify specific file presence
	for _, expectedPath := range []string{"flips/flip_1.json", "failed_buys/failed_buy_2.json", "reports/report_latest.md"} {
		if _, ok := payload.Files[expectedPath]; !ok {
			t.Errorf("Expected backup to contain file %s, but it was missing", expectedPath)
		}
	}

	// 5. Delete all files locally to simulate database loss
	if err := os.RemoveAll("flips"); err != nil {
		t.Fatalf("Failed to clean flips: %v", err)
	}
	if err := os.RemoveAll("failed_buys"); err != nil {
		t.Fatalf("Failed to clean failed_buys: %v", err)
	}
	if err := os.RemoveAll("reports"); err != nil {
		t.Fatalf("Failed to clean reports: %v", err)
	}

	// 6. Trigger restore
	if err := RestoreData(backupJSON); err != nil {
		t.Fatalf("RestoreData failed: %v", err)
	}

	// 7. Verify files are perfectly restored
	restoredFlip, err := Store.ReadRaw("flips/flip_1.json")
	if err != nil {
		t.Fatalf("Failed to read restored flip: %v", err)
	}
	if string(restoredFlip) != string(flipContent) {
		t.Errorf("Restored flip content mismatch. Expected %s, got %s", flipContent, restoredFlip)
	}

	restoredFailed, err := Store.ReadRaw("failed_buys/failed_buy_2.json")
	if err != nil {
		t.Fatalf("Failed to read restored failed buy: %v", err)
	}
	if string(restoredFailed) != string(failedContent) {
		t.Errorf("Restored failed buy content mismatch. Expected %s, got %s", failedContent, restoredFailed)
	}

	restoredReport, err := Store.ReadRaw("reports/report_latest.md")
	if err != nil {
		t.Fatalf("Failed to read restored report: %v", err)
	}
	if string(restoredReport) != string(reportContent) {
		t.Errorf("Restored report content mismatch. Expected %s, got %s", reportContent, restoredReport)
	}
}


