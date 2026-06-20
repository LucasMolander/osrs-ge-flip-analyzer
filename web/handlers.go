package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/lucasmolander/osrs-ge-flip-analyzer/core"
)

// ErrorResponse represents a structured error with an optional stack trace.
type ErrorResponse struct {
	Error      string `json:"error"`
	StackTrace string `json:"stack_trace,omitempty"`
}

// sendError sends a structured JSON error response containing the error message and the current stack trace.
func sendError(w http.ResponseWriter, err error, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	errMsg := message
	if err != nil {
		errMsg = fmt.Sprintf("%s: %v", message, err)
	}

	resp := ErrorResponse{
		Error:      errMsg,
		StackTrace: string(debug.Stack()),
	}

	json.NewEncoder(w).Encode(resp)
}

// apiReportHandler triggers a fresh analysis using cached/downloaded data and returns the top flips.
func (app *AppServer) apiReportHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		sendError(w, nil, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Always skip download during web requests to prevent API rate limiting / latency.
	// A background cron job should ideally update the prices.
	flips, err := core.RunAnalysis(app.Client, app.Capital, app.VolThreshold, app.Limit, false, "")
	if err != nil {
		sendError(w, err, "Failed to generate report", http.StatusInternalServerError)
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
		sendError(w, nil, "Method not allowed", http.StatusMethodNotAllowed)
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
		sendError(w, err, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	if req.ItemID <= 0 || req.Quantity <= 0 || req.BuyPrice <= 0 || req.SellPrice <= 0 {
		sendError(w, nil, "item_id, quantity, buy_price, and sell_price must all be > 0", http.StatusBadRequest)
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
		sendError(w, err, "Failed to save flip record", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

// apiRecordFailedBuyHandler records an unsuccessful buy attempt.
func (app *AppServer) apiRecordFailedBuyHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		sendError(w, nil, "Method not allowed", http.StatusMethodNotAllowed)
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
		sendError(w, err, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	if req.ItemID <= 0 || req.TargetQty <= 0 || req.BuyPrice <= 0 {
		sendError(w, nil, "item_id, target_qty, and buy_price must be > 0", http.StatusBadRequest)
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
		sendError(w, err, "Failed to save failed buy record", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}
