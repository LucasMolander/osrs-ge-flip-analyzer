package main

import (
	"math"
	"testing"
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
		{1250000, "1.25M"},
		{500000000, "500.00M"},
		{-1250000, "-1.25M"},
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

	report := AnalyzePrices(prices, volumes, metadata, capitalThreshold, volThreshold, nudges)

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
	// Item A has high volume (100), Item B has low volume (2)
	volumes := map[string]HourlyVolume{
		"1": {HighPriceVolume: 50, LowPriceVolume: 50}, // Volume = 100
		"2": {HighPriceVolume: 1, LowPriceVolume: 1},   // Volume = 2
	}

	// Large capital threshold to isolate volume effect
	capThreshold := int64(1_000_000_000)
	volThreshold := int64(10) // K_vol = 10
	nudges := make(map[int]float64)

	report := AnalyzePrices(prices, volumes, metadata, capThreshold, volThreshold, nudges)

	if len(report) != 2 {
		t.Fatalf("Expected 2 items in report, got %d", len(report))
	}

	// Item 1 (high volume) should be ranked higher than Item 2 (low volume)
	if report[0].ID != 1 || report[1].ID != 2 {
		t.Errorf("Volume heuristic sorting failed. Expected order: [1, 2], got: [%d, %d]", report[0].ID, report[1].ID)
	}

	// Verify volume factors
	// Item A: 100 / (100 + 10) = 0.909
	// Item B: 2 / (2 + 10) = 0.167
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

	reportCap := AnalyzePrices(pricesCap, volumesCap, metadataCap, capThresholdTest, volThresholdTest, nudges)

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
	expectedScore3 := 45000.0 * (10000.0 / (10000.0 + 106000.0)) * (20000.0 / 20001.0)
	if math.Abs(item3.Score-expectedScore3) > 1.0 {
		t.Errorf("Item 3 score: expected %f, got %f", expectedScore3, item3.Score)
	}

	item4 := resMapCap[4]
	expectedScore4 := 4492.0 * (10000.0 / (10000.0 + 10600.0)) * (20000.0 / 20001.0)
	if math.Abs(item4.Score-expectedScore4) > 1.0 {
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

	report := AnalyzePrices(prices, volumes, metadata, 1000000000, 1, nudges)

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
