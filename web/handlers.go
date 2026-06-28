package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime/debug"

	"github.com/lucasmolander/osrs-ge-flip-analyzer/backend"
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
	flips, err := backend.RunAnalysis(r.Context(), app.Client, app.Capital, app.VolThreshold, app.Limit, false, "", config, req.Flips, req.FailedSells)
	if err != nil {
		sendError(w, err, "Failed to generate report", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(flips); err != nil {
		fmt.Printf("Error encoding JSON response: %v\n", err)
	}
}

// apiReportStatusHandler returns the timestamp of the latest cached prices file.
func (app *AppServer) apiReportStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		sendError(w, nil, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	_, ts, err := app.Store.FindLatestFile("prices", "prices")
	if err != nil {
		sendError(w, err, "Failed to get latest price timestamp", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int64{"lastUpdate": ts})
}

// apiMarketStateHandler redirects to the public GCS URL for the market state JSON.
func (app *AppServer) apiMarketStateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		sendError(w, nil, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	bucketName := os.Getenv("GCS_BUCKET")
	if bucketName == "" {
		// Fallback for local development
		w.Header().Set("Content-Type", "application/json")
		data, err := os.ReadFile("market_state_latest.json")
		if err != nil {
			sendError(w, err, "Market state not found", http.StatusNotFound)
			return
		}
		w.Write(data)
		return
	}

	redirectURL := fmt.Sprintf("https://storage.googleapis.com/%s/market_state_latest.json", bucketName)
	http.Redirect(w, r, redirectURL, http.StatusFound)
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
	err := backend.GenerateMarketState(r.Context(), app.Client, true)
	if err != nil {
		fmt.Printf("[Cron] Error generating market state: %v\n", err)
		sendError(w, err, "Failed to generate market state", http.StatusInternalServerError)
		return
	}

	fmt.Println("[Cron] Market data fetch and state generation complete.")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "ok"}`))
}
