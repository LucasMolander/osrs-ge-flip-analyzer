package main

import (
	"context"
	"encoding/json"
	"fmt"
	"syscall/js"

	"github.com/lucasmolander/osrs-ge-flip-analyzer/core"
)

var (
	globalMarketState *core.MarketState
	globalNudges      map[int]float64
	globalConfig      *core.RankingConfig
)

func init() {
	globalNudges = make(map[int]float64)
	globalConfig = core.DefaultRankingConfig()
}

// GoLoadMarketData parses the market state JSON and stores it globally.
func GoLoadMarketData(this js.Value, p []js.Value) interface{} {
	if len(p) < 1 {
		return "Error: expected 1 argument (market JSON string)"
	}
	jsonStr := p[0].String()

	var state core.MarketState
	if err := json.Unmarshal([]byte(jsonStr), &state); err != nil {
		fmt.Println("Error unmarshaling market data:", err)
		return fmt.Sprintf("Error parsing market state: %v", err)
	}

	globalMarketState = &state
	fmt.Printf("Loaded market data successfully. %d prices, %d volumes\n", len(state.Prices), len(state.Volumes))
	return "OK"
}

// UserDataPayload represents the expected JSON shape from the UI for GoLoadUserData.
type UserDataPayload struct {
	Flips       []core.FlipRecord       `json:"flips"`
	FailedSells []core.FailedSellRecord `json:"failed_sells"`
	Config      *core.RankingConfig     `json:"config"`
}

// GoLoadUserData parses user flips, failed sells, and config to pre-compute nudges.
func GoLoadUserData(this js.Value, p []js.Value) interface{} {
	if len(p) < 1 {
		return "Error: expected 1 argument (user data JSON string)"
	}
	jsonStr := p[0].String()

	var payload UserDataPayload
	if err := json.Unmarshal([]byte(jsonStr), &payload); err != nil {
		fmt.Println("Error unmarshaling user data:", err)
		return fmt.Sprintf("Error parsing user data: %v", err)
	}

	cfg := payload.Config
	if cfg == nil {
		cfg = globalConfig
	} else {
		globalConfig = cfg
	}

	ctx := context.Background()
	globalNudges = core.CalculateNudges(ctx, cfg, payload.Flips, payload.FailedSells)

	fmt.Printf("Loaded user data. Computed nudges for %d items.\n", len(globalNudges))
	return "OK"
}

// GoCalculateScores calculates scores instantly using global state and a config.
func GoCalculateScores(this js.Value, p []js.Value) interface{} {
	if len(p) < 2 {
		return "[]"
	}
	configStr := p[0].String()
	filterStr := p[1].String()

	if globalMarketState == nil {
		fmt.Println("Error: Market data not loaded yet")
		return "[]"
	}

	var config core.RankingConfig
	if err := json.Unmarshal([]byte(configStr), &config); err != nil {
		fmt.Println("Error parsing config in CalculateScores:", err)
		// Fallback to global config if parse fails
		config = *globalConfig
	}

	ctx := context.Background()

	reportItems := core.AnalyzePrices(
		ctx,
		globalMarketState.Timestamp,
		globalMarketState.Prices,
		globalMarketState.Volumes,
		globalMarketState.Metadata,
		globalNudges,
		globalMarketState.Hist1h,
		globalMarketState.Hist24h,
		globalMarketState.Hist30d,
		globalMarketState.Vol5m,
		globalMarketState.Vol24h,
		filterStr,
		&config,
		globalMarketState.Rolling24,
	)

	jsonBytes, err := json.Marshal(reportItems)
	if err != nil {
		fmt.Println("Error marshaling report items:", err)
		return "[]"
	}

	return string(jsonBytes)
}

func main() {
	fmt.Println("Go WASM Initialized: OSRS GE Flip Analyzer Bridge")

	// Expose functions to JavaScript
	js.Global().Set("GoLoadMarketData", js.FuncOf(GoLoadMarketData))
	js.Global().Set("GoLoadUserData", js.FuncOf(GoLoadUserData))
	js.Global().Set("GoCalculateScores", js.FuncOf(GoCalculateScores))

	// Keep the WASM instance running forever
	select {}
}
