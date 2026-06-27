package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime/debug"

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
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		sendError(w, nil, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req core.ReportRequest
	configCopy := *app.Config
	req.Config = &configCopy

	if r.Method == http.MethodPost {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sendError(w, err, "Invalid JSON payload", http.StatusBadRequest)
			return
		}
	}
	
	// If the frontend sends `"config": null` before it finishes loading defaults,
	// the JSON decoder will overwrite req.Config with nil. Restore it.
	if req.Config == nil {
		configCopy2 := *app.Config
		req.Config = &configCopy2
	}
	
	config := req.Config

	// Don't force download on page load, just use the latest cached data. The background cron job keeps this fresh.
	flips, err := core.RunAnalysis(r.Context(), app.Client, app.Capital, app.VolThreshold, app.Limit, false, "", config, req.Flips, req.FailedSells)
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

// apiCronTickHandler is triggered by Cloud Scheduler every minute.
// It forces a download of new market prices and regenerates the database report.
func (app *AppServer) apiCronTickHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		sendError(w, nil, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Verify the secret header to ensure this request came from our Cloud Scheduler
	secret := os.Getenv("CRON_SECRET")
	if secret == "" || r.Header.Get("X-Cron-Secret") != secret {
		sendError(w, nil, "Unauthorized", http.StatusUnauthorized)
		return
	}

	fmt.Println("[Cron] Received 1-minute tick from Cloud Scheduler. Fetching market data...")

	// forceDownload = true to actually pull new data from OSRS Wiki APIs
	_, err := core.RunAnalysis(r.Context(), app.Client, app.Capital, app.VolThreshold, app.Limit, true, "", app.Config, nil, nil)
	if err != nil {
		fmt.Printf("[Cron] Error generating report: %v\n", err)
		sendError(w, err, "Failed to generate background report", http.StatusInternalServerError)
		return
	}

	fmt.Println("[Cron] Market data fetch and report generation complete.")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "ok"}`))
}
