package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/lucasmolander/osrs-ge-flip-analyzer/core"
)

// apiReportHandler triggers a fresh analysis using cached/downloaded data and returns the top flips.
func (app *AppServer) apiReportHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Always skip download during web requests to prevent API rate limiting / latency.
	// A background cron job should ideally update the prices.
	flips, err := core.RunAnalysis(app.Client, app.Capital, app.VolThreshold, app.Limit, false)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to generate report: %v", err), http.StatusInternalServerError)
		return
	}

	// Limit to the top 50
	limit := app.Limit
	if len(flips) < limit {
		limit = len(flips)
	}
	topFlips := flips[:limit]

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(topFlips); err != nil {
		fmt.Printf("Error encoding JSON response: %v\n", err)
	}
}

// apiRecordFlipHandler records a successful flip.
func (app *AppServer) apiRecordFlipHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ItemID    int    `json:"item_id"`
		Quantity  int    `json:"quantity"`
		BuyPrice  int64  `json:"buy_price"`
		SellPrice int64  `json:"sell_price"`
		Note      string `json:"note"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	if req.ItemID <= 0 || req.Quantity <= 0 || req.BuyPrice <= 0 || req.SellPrice <= 0 {
		http.Error(w, "item_id, quantity, buy_price, and sell_price must all be > 0", http.StatusBadRequest)
		return
	}

	ts := time.Now().Unix()
	record := core.FlipRecord{
		ItemID:    req.ItemID,
		Quantity:  req.Quantity,
		BuyPrice:  req.BuyPrice,
		SellPrice: req.SellPrice,
		Timestamp: time.Unix(ts, 0),
		Notes:     req.Note,
	}

	prefix := fmt.Sprintf("flip_%d", req.ItemID)
	if _, err := core.SaveJSON("flips", prefix, ts, record); err != nil {
		http.Error(w, "Failed to save flip record", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

// apiRecordFailedBuyHandler records an unsuccessful buy attempt.
func (app *AppServer) apiRecordFailedBuyHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ItemID      int     `json:"item_id"`
		ItemName    string  `json:"item_name"`
		TargetQty   int     `json:"target_qty"`
		BoughtQty   int     `json:"bought_qty"`
		BuyPrice    int64   `json:"buy_price"`
		TimeSpent   string  `json:"time_spent"`
		ReportScore float64 `json:"report_score"`
		Note        string  `json:"note"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	if req.ItemID <= 0 || req.TargetQty <= 0 || req.BuyPrice <= 0 {
		http.Error(w, "item_id, target_qty, and buy_price must be > 0", http.StatusBadRequest)
		return
	}

	ts := time.Now().Unix()
	record := core.FailedBuyRecord{
		ItemID:      req.ItemID,
		ItemName:    req.ItemName,
		TargetQty:   req.TargetQty,
		BoughtQty:   req.BoughtQty,
		BuyPrice:    req.BuyPrice,
		TimeSpent:   req.TimeSpent,
		ReportScore: req.ReportScore,
		Timestamp:   time.Unix(ts, 0),
		Notes:       req.Note,
	}

	prefix := fmt.Sprintf("failed_buy_%d", req.ItemID)
	if _, err := core.SaveJSON("failed_buys", prefix, ts, record); err != nil {
		http.Error(w, "Failed to save failed buy record", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}
