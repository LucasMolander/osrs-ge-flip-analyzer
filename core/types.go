package core

import "time"

// LatestPrice represents the instant-buy and instant-sell price details from the /latest endpoint.
type LatestPrice struct {
	High     *int64 `json:"high"`     // Most recent instant buy price (could be null)
	HighTime *int64 `json:"highTime"` // Unix timestamp of last high transaction
	Low      *int64 `json:"low"`      // Most recent instant sell price (could be null)
	LowTime  *int64 `json:"lowTime"`  // Unix timestamp of last low transaction
}

// LatestPricesResponse represents the payload returned by the /latest endpoint.
type LatestPricesResponse struct {
	Data map[string]LatestPrice `json:"data"`
}

// HourlyVolume represents the 1-hour averages and trading volumes from the /1h endpoint.
type HourlyVolume struct {
	AvgHighPrice     *int64 `json:"avgHighPrice"`     // Volume-weighted average high price
	HighPriceVolume  int64  `json:"highPriceVolume"`  // Quantity traded at high price
	AvgLowPrice      *int64 `json:"avgLowPrice"`      // Volume-weighted average low price
	LowPriceVolume   int64  `json:"lowPriceVolume"`   // Quantity traded at low price
}

// HourlyVolumesResponse represents the payload returned by the /1h endpoint.
type HourlyVolumesResponse struct {
	Timestamp int64                   `json:"timestamp"`
	Data      map[string]HourlyVolume `json:"data"`
}

// ItemMetadata represents the static properties of an item from the /mapping endpoint.
type ItemMetadata struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Examine  string `json:"examine"`
	Members  bool   `json:"members"`
	LowAlch  int    `json:"lowalch"`
	HighAlch int    `json:"highalch"`
	Limit    int    `json:"limit"` // GE buy limit (could be 0 if unknown/untradeable)
	Value    int    `json:"value"`
	Icon     string `json:"icon"`
}

// FlipRecord represents a historical transaction logged by the user to nudge rankings.
type FlipRecord struct {
	ItemID    int       `json:"item_id"`
	ItemName  string    `json:"item_name"`
	Rating    string    `json:"rating"` // "Meh", "Mid", "Good", "Great"
	Timestamp time.Time `json:"timestamp"`
	Notes     string    `json:"notes,omitempty"`
}

// FailedSellRecord represents an unsuccessful or unprofitable sell order.
type FailedSellRecord struct {
	ItemID    int       `json:"item_id"`
	ItemName  string    `json:"item_name"`
	Timestamp time.Time `json:"timestamp"`
	Notes     string    `json:"notes,omitempty"`
}

// ReportRequest represents the incoming JSON payload for /api/report
type ReportRequest struct {
	Config      *RankingConfig     `json:"config,omitempty"`
	Flips       []FlipRecord       `json:"flips,omitempty"`
	FailedSells []FailedSellRecord `json:"failed_sells,omitempty"`
}

// ReportItem represents a completed analysis entry for an item, sorted and ranked.
type ReportItem struct {
	ID              int     `json:"id"`
	Name            string  `json:"name"`
	Icon            string  `json:"icon"`
	BuyLimit        int     `json:"buy_limit"`
	High            int64   `json:"high"`
	Low             int64   `json:"low"`
	HighMod         int64   `json:"high_mod"`
	LowMod          int64   `json:"low_mod"`
	Tax             int64   `json:"tax"`
	ProfitPerItem   int64   `json:"profit_per_item"`
	PotentialProfit int64   `json:"potential_profit"`
	CapitalRequired int64   `json:"capital_required"`
	ROI             float64 `json:"roi"`
	Volume          int64   `json:"volume"`
	Score           float64  `json:"score"`
	ProfitMultiplier float64 `json:"profit_multiplier"`
	NudgeMultiplier float64  `json:"nudge_multiplier"`
	TrendMultiplier float64  `json:"trend_multiplier"`
	PriceTrendIndicators  []string `json:"price_trend_indicators"`
	VolumeSpikeIndicators []string `json:"volume_spike_indicators"`
	IsSink          bool     `json:"is_sink"`
	GlobalRank      int      `json:"globalRank"`
}

// SinkItems is a set of items regulated via the OSRS item sink mechanism.
var SinkItems = map[string]bool{
	"Ancestral hat":                      true,
	"Ancestral robe top":                  true,
	"Ancestral robe bottom":               true,
	"Dinh's bulwark":                     true,
	"Elder maul":                         true,
	"Kodai wand":                         true,
	"Arcane prayer scroll":               true,
	"Dexterous prayer scroll":            true,
	"Twisted buckler":                    true,
	"Twisted bow":                        true,
	"Avernic defender hilt":              true,
	"Sanguinesti staff":                  true,
	"Scythe of vitur":                    true,
	"Elidinis' ward":                     true,
	"Lightbearer":                        true,
	"Masori mask (f)":                    true,
	"Masori body (f)":                    true,
	"Masori chaps (f)":                    true,
	"Osmumten's fang":                    true,
	"Tumeken's shadow":                   true,
	"Archers ring":                       true,
	"Berserker ring":                     true,
	"Armadyl helmet":                     true,
	"Armadyl chestplate":                 true,
	"Armadyl chainskirt":                 true,
	"Bandos chestplate":                  true,
	"Bandos tassets":                     true,
	"Bandos boots":                       false, // Explicitly excluded in GWD uniques
	"Ancient godsword":                   true,
	"Armadyl godsword":                   true,
	"Bandos godsword":                    true,
	"Saradomin godsword":                 true,
	"Zamorak godsword":                   true,
	"Torva full helm":                    true,
	"Torva platebody":                    true,
	"Torva platelegs":                    true,
	"Zamorakian spear":                   true,
	"Zaryte crossbow":                    true,
	"Zaryte vambraces":                   true,
	"Inquisitor's great helm":            true,
	"Inquisitor's hauberk":               true,
	"Inquisitor's plateskirt":            true,
	"Inquisitor's mace":                  true,
	"Nightmare staff":                    true,
	"Eldritch orb":                       true,
	"Harmonised orb":                     true,
	"Volatile orb":                       true,
	"Abyssal bludgeon":                   true,
	"Dark bow":                           true,
	"Dragon hunter lance":                true,
	"Smoke battlestaff":                  true,
	"Trident of the seas (uncharged)":    true,
	"Zenyte shard":                       true,
	"Uncut zenyte":                       true,
	"Zenyte":                             true,
	"Amulet of torture":                  true,
	"Necklace of anguish":                true,
	"Ring of suffering":                  true,
	"Tormented bracelet":                 true,
	"Arcane spirit shield":               true,
	"Spectral spirit shield":             true,
	"Burning claws":                      true,
	"Dragon pickaxe":                     true,
	"Dragon warhammer":                   true,
	"Toxic blowpipe (empty)":             true,
}
