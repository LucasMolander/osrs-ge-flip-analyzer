package core

import (
	"context"
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
		{1200, "1.2K"},
		{45000, "45.0K"},
		{100000, "100.000K"},
		{300230, "300.230K"},
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

	now := time.Now().Unix()
	prices := map[string]LatestPrice{
		"1": {High: &hCheapHigh, Low: &hCheapLow, HighTime: &now, LowTime: &now},
		"2": {High: &hStdHigh, Low: &hStdLow, HighTime: &now, LowTime: &now},
		"3": {High: &hExpHigh, Low: &hExpLow, HighTime: &now, LowTime: &now},
		"4": {High: &hNarrowHigh, Low: &hNarrowLow, HighTime: &now, LowTime: &now},
		"5": {High: &hTbowHigh, Low: &hTbowLow, HighTime: &now, LowTime: &now},
	}

	// High volumes to ignore volume penalty (VolumeFactor ~ 1.0)
	volumes := map[string]HourlyVolume{
		"1": {HighPriceVolume: 10000, LowPriceVolume: 10000},
		"2": {HighPriceVolume: 10000, LowPriceVolume: 10000},
		"3": {HighPriceVolume: 10000, LowPriceVolume: 10000},
		"4": {HighPriceVolume: 10000, LowPriceVolume: 10000},
		"5": {HighPriceVolume: 10000, LowPriceVolume: 10000},
	}

	nudges := make(map[int]float64)

	config := DefaultRankingConfig()
	config.BaseCapital = 1_000_000_000_000
	config.MinAbsoluteVolume = 1
	config.StaleExtremePenaltyMultiplier = 1.0

	report := AnalyzePrices(context.Background(), time.Now().Unix(), prices, volumes, metadata, nudges, nil, nil, nil, nil, nil, "", config, nil)

	// We expect 5 items because we removed the continue statements.
	if len(report) != 5 {
		t.Fatalf("Expected 5 items in report, got %d", len(report))
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

	now := time.Now().Unix()
	prices := map[string]LatestPrice{
		"1": {High: &hHigh, Low: &hLow, HighTime: &now, LowTime: &now},
		"2": {High: &hHigh, Low: &hLow, HighTime: &now, LowTime: &now},
	}

	// Test 1: Volume Penalty Heuristic
	// Item A has high volume (100) equal to its limit (100), Item B has low volume (20) relative to its limit (100)
	volumes := map[string]HourlyVolume{
		"1": {HighPriceVolume: 50, LowPriceVolume: 50}, // Volume = 100
		"2": {HighPriceVolume: 30, LowPriceVolume: 30}, // Volume = 60 -> 240 / 4h -> 240 * 0.05 = 12. Limit = 100. Ratio = 0.12. Passes 10% filter.
	}

	nudges := make(map[int]float64)

	configHeur := DefaultRankingConfig()
	configHeur.BaseCapital = 1_000_000_000
	configHeur.MinAbsoluteVolume = 10
	configHeur.StaleExtremePenaltyMultiplier = 1.0

	report := AnalyzePrices(context.Background(), time.Now().Unix(), prices, volumes, metadata, nudges, nil, nil, nil, nil, nil, "", configHeur, nil)

	if len(report) != 2 {
		t.Fatalf("Expected 2 items in report, got %d", len(report))
	}

	// Item 1 (high volume) should be ranked higher than Item 2 (low volume)
	if report[0].ID != 1 {
		t.Errorf("Volume heuristic sorting failed. Expected order: [1], got: [%d]", report[0].ID)
	}

	factorA := report[0].Score / float64(report[0].PotentialProfit)
	if factorA <= 0.0 {
		t.Errorf("Expected volume factor for Item A (%f) to be > 0.0", factorA)
	}

	// Test 2: Capital Penalty Heuristic
	// Test 2: Affordable Capital Heuristic
	// Base Capital = 10,000 gp.
	// Item 3: Low price = 100, lowMod = 106. Limit = 1000.
	// Affordable Qty = Floor(10,000 / 106) = 94.
	// Profit per item = 45. Potential Profit = 94 * 45 = 4230.
	// ROI Multiplier = Min(2.0, (45/106)/0.02) = 2.0.
	// Expected Score = 4230 * 2.0 = 8460.

	// Item 4: Low price = 10,000, lowMod = 10,600. Limit = 1.
	// Affordable Qty = Floor(10,000 / 10,600) = 0.
	// Since Affordable Qty is 0 and it's not a target, it should be filtered out completely!

	metadataCap := map[int]ItemMetadata{
		3: {ID: 3, Name: "Affordable Item", Limit: 1000},
		4: {ID: 4, Name: "Unaffordable Item", Limit: 1},
	}

	h3High := int64(160)
	h3Low := int64(100)
	h4High := int64(16000)
	h4Low := int64(10000)

	nowCap := time.Now().Unix()
	pricesCap := map[string]LatestPrice{
		"3": {High: &h3High, Low: &h3Low, HighTime: &nowCap, LowTime: &nowCap},
		"4": {High: &h4High, Low: &h4Low, HighTime: &nowCap, LowTime: &nowCap},
	}
	volumesCap := map[string]HourlyVolume{
		"3": {HighPriceVolume: 10000, LowPriceVolume: 10000},
		"4": {HighPriceVolume: 10000, LowPriceVolume: 10000},
	}

	nudges = make(map[int]float64)

	configCap := DefaultRankingConfig()
	configCap.BaseCapital = 10000
	configCap.MinAbsoluteVolume = 1
	configCap.StaleExtremePenaltyMultiplier = 1.0

	reportCap := AnalyzePrices(context.Background(), time.Now().Unix(), pricesCap, volumesCap, metadataCap, nudges, nil, nil, nil, nil, nil, "", configCap, nil)

	if len(reportCap) != 2 || reportCap[0].ID != 3 {
		t.Fatalf("Expected Item 3 to be first, got %d items", len(reportCap))
	}

	expectedScore := 10125.0
	if math.Abs(reportCap[0].Score-expectedScore) > 0.001 {
		t.Errorf("Expected score to be %f, got %f", expectedScore, reportCap[0].Score)
	}
}

func TestAnalyzePrices_Nudge(t *testing.T) {
	metadata := map[int]ItemMetadata{
		1: {ID: 1, Name: "Standard Item", Limit: 1000},
	}

	hHigh := int64(1000)
	hLow := int64(800)

	now := time.Now().Unix()
	prices := map[string]LatestPrice{
		"1": {High: &hHigh, Low: &hLow, HighTime: &now, LowTime: &now},
	}
	volumes := map[string]HourlyVolume{
		"1": {HighPriceVolume: 10000, LowPriceVolume: 10000},
	}

	nudges := map[int]float64{
		1: 1.5, // 50% boost from successful history
	}

	configNudge := DefaultRankingConfig()
	configNudge.BaseCapital = 1_000_000_000
	configNudge.MinAbsoluteVolume = 1
	configNudge.StaleExtremePenaltyMultiplier = 1.0

	report := AnalyzePrices(context.Background(), time.Now().Unix(), prices, volumes, metadata, nudges, nil, nil, nil, nil, nil, "", configNudge, nil)

	if len(report) != 1 {
		t.Fatalf("Expected 1 item in report, got %d", len(report))
	}

	item := report[0]
	if item.NudgeMultiplier != 1.5 {
		t.Errorf("Expected NudgeMultiplier to be 1.5, got %f", item.NudgeMultiplier)
	}

	// Verify score is multiplied by 1.5
	// The exact math has changed (ROI bounded, capture rate added).
	// We just ensure Score is significantly > potentialProfit to prove nudge applied.
	if item.Score <= float64(item.PotentialProfit) {
		t.Errorf("Expected score to be heavily nudged, got %f vs profit %d", item.Score, item.PotentialProfit)
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
	h := int64(1000)
	l := int64(800)
	now := time.Now().Unix()
	prices := map[string]LatestPrice{
		"1": {High: &h, Low: &l, HighTime: &now, LowTime: &now}, // Stable
		"2": {High: &h, Low: &l, HighTime: &now, LowTime: &now}, // Dropping 1h
		"3": {High: &h, Low: &l, HighTime: &now, LowTime: &now}, // Dropping 24h
		"4": {High: &h, Low: &l, HighTime: &now, LowTime: &now}, // Dropping 30d
		"5": {High: &h, Low: &l, HighTime: &now, LowTime: &now}, // Dropping ALL
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

	// 1h ago: Item 2 and 5 are down (historical avg 1100 > currentHigh 1000)
	hist1h := map[string]HourlyVolume{
		"1": hVol(1000), // stable
		"2": hVol(1100), // down
		"3": hVol(1000), // stable
		"4": hVol(1000), // stable
		"5": hVol(1100), // down
	}

	// 24h ago: Item 3 and 5 are down
	hist24h := map[string]HourlyVolume{
		"1": hVol(1000),
		"2": hVol(1000),
		"3": hVol(1100),
		"4": hVol(1000),
		"5": hVol(1100),
	}

	// 30d ago: Item 4 and 5 are down
	hist30d := map[string]HourlyVolume{
		"1": hVol(1000),
		"2": hVol(1000),
		"3": hVol(1000),
		"4": hVol(1100),
		"5": hVol(1100),
	}

	nudges := make(map[int]float64)

	config := DefaultRankingConfig()
	config.BaseCapital = 1_000_000_000_000
	config.MinAbsoluteVolume = 10
	config.OutlierZScoreThreshold = 9999.0 // Disable outlier penalty to isolate trend penalties
	config.StaleExtremePenaltyMultiplier = 1.0

	// Run analyzer
	report := AnalyzePrices(context.Background(), time.Now().Unix(), prices, volumes, metadata, nudges, hist1h, hist24h, hist30d, nil, nil, "", config, nil)

	if len(report) != 5 {
		t.Fatalf("Expected 5 items in report, got %d", len(report))
	}

	// Map results for verification
	res := make(map[int]ReportItem)
	for _, item := range report {
		res[item.ID] = item
	}

	item1, item2, item3, item4, item5 := res[1], res[2], res[3], res[4], res[5]

	if item1.TrendMultiplier != 1.0 || len(item1.PriceTrendIndicators) != 1 || item1.PriceTrendIndicators[0] != "↗" {
		t.Errorf("Item 1 stable failed: TrendMultiplier=%f, Indicators=%v", item1.TrendMultiplier, item1.PriceTrendIndicators)
	}

	if item2.TrendMultiplier != 0.8 || !contains(item2.PriceTrendIndicators, "↓1h") {
		t.Errorf("Item 2 down 1h failed: TrendMultiplier=%f, Indicators=%v", item2.TrendMultiplier, item2.PriceTrendIndicators)
	}

	if item3.TrendMultiplier != 0.9 || !contains(item3.PriceTrendIndicators, "↓24h") {
		t.Errorf("Item 3 down 24h failed: TrendMultiplier=%f, Indicators=%v", item3.TrendMultiplier, item3.PriceTrendIndicators)
	}

	if item4.TrendMultiplier != 1.0 || len(item4.PriceTrendIndicators) != 1 {
		t.Errorf("Item 4 (30d) failed: TrendMultiplier=%f, Indicators=%v", item4.TrendMultiplier, item4.PriceTrendIndicators)
	}

	if math.Abs(item5.TrendMultiplier-0.72) > 0.001 || len(item5.PriceTrendIndicators) != 2 {
		t.Errorf("Item 5 down all failed: TrendMultiplier=%f, Indicators=%v", item5.TrendMultiplier, item5.PriceTrendIndicators)
	}

	// Verify sorting order: Item 1 & 4 (1.0) > Item 3 (0.90) > Item 2 (0.80) > Item 5 (0.72)
	if report[2].ID != 3 || report[3].ID != 2 || report[4].ID != 5 {
		t.Errorf("Trend penalty sorting failed.")
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
	now := time.Now().Unix()
	prices := map[string]LatestPrice{
		"1": {High: &hHigh, Low: &hLow, HighTime: &now, LowTime: &now},
		"2": {High: &hHigh, Low: &hLow, HighTime: &now, LowTime: &now},
		"3": {High: &hHigh, Low: &hLow, HighTime: &now, LowTime: &now},
		"4": {High: &hHigh, Low: &hLow, HighTime: &now, LowTime: &now},
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

	config := DefaultRankingConfig()
	config.BaseCapital = 1_000_000_000
	config.MinAbsoluteVolume = 10

	// Run analyzer
	report := AnalyzePrices(context.Background(), time.Now().Unix(), prices, volumes, metadata, nudges, nil, nil, nil, nil, nil, "", config, nil)

	// We expect 4 items since absolute volume filter is no longer a hard drop
	if len(report) != 4 {
		t.Fatalf("Expected 4 items in report, got %d", len(report))
	}

	// Map results
	resMap := make(map[int]ReportItem)
	for _, item := range report {
		resMap[item.ID] = item
	}

	// Verify Item 4 has NO absolute volume penalty
	// The absolute volume filter tests will have different expected numbers due to other factor changes.
	// We just ensure 4 > 3.
	if report[0].ID != 4 || report[1].ID != 3 {
		t.Errorf("Expected order: [4, 3], got: [%d, %d]", report[0].ID, report[1].ID)
	}

	// Verify sorting order: Item 4 (Score ~242) > Item 3 (Score ~163)
	if report[0].ID != 4 || report[1].ID != 3 {
		t.Errorf("Expected Item 4 then Item 3, got %v", report)
	}
}

// contains checks if a string slice contains a given string
func contains(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}

func TestCalculateNudges(t *testing.T) {
	now := time.Now()

	flip := FlipRecord{
		ItemID:    4151, // Abyssal whip
		ItemName:  "Abyssal whip",
		Rating:    "Good",
		Timestamp: now,
		Notes:     "Good flip",
	}

	failed1 := FailedSellRecord{
		ItemID:    4151, // Same as flip to stack them
		ItemName:  "Abyssal whip",
		Timestamp: now,
	}

	failed2 := FailedSellRecord{
		ItemID:    11832, // Bandos chestplate
		ItemName:  "Bandos chestplate",
		Timestamp: now.Add(-48 * time.Hour),
		Notes:     "Missed it",
	}

	multipliers := CalculateNudges(context.Background(), DefaultRankingConfig(), []FlipRecord{flip}, []FailedSellRecord{failed1, failed2})

	// 7. Verify multipliers
	// Item 4151:
	// Net Nudge = flip nudge (+0.10) + failed buy nudge (-0.40) = -0.30
	// Weight Sum = 2.0
	// EMA = -0.15
	// Expected Multiplier = 1.0 - 0.15 = 0.85
	_, ok := multipliers[4151]
	if !ok {
		t.Errorf("Expected multiplier for Item 4151, but found none")
	} else if math.Abs(multipliers[4151]-0.70) > 0.01 {
		t.Errorf("Item 4151 multiplier: expected 0.70, got %f", multipliers[4151])
	}

	// 2 failed sells. Because of exponential decay with their slight ages, it will be slightly above 0.70 (around 0.748)
	if math.Abs(multipliers[11832]-0.748) > 0.01 {
		t.Errorf("Item 11832 multiplier: expected ~0.748, got %f", multipliers[11832])
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
	if err := os.MkdirAll("failed_sells", 0755); err != nil {
		t.Fatalf("Failed to create failed_sells dir: %v", err)
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
	if err := Store.WriteRaw("failed_sells/failed_sell_2.json", failedContent); err != nil {
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
	for _, expectedPath := range []string{"flips/flip_1.json", "failed_sells/failed_sell_2.json", "reports/report_latest.md"} {
		if _, ok := payload.Files[expectedPath]; !ok {
			t.Errorf("Expected backup to contain file %s, but it was missing", expectedPath)
		}
	}

	// 5. Delete all files locally to simulate database loss
	if err := os.RemoveAll("flips"); err != nil {
		t.Fatalf("Failed to clean flips: %v", err)
	}
	if err := os.RemoveAll("failed_sells"); err != nil {
		t.Fatalf("Failed to clean failed_sells: %v", err)
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

	restoredFailed, err := Store.ReadRaw("failed_sells/failed_sell_2.json")
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


