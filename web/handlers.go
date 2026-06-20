package web

import (
	"encoding/json"
	"fmt"
	"net/http"
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
	config := app.Config
	
	if r.Method == http.MethodPost {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sendError(w, err, "Invalid JSON payload", http.StatusBadRequest)
			return
		}
		if req.Config != nil {
			config = req.Config
		}
	}

	// Always skip download during web requests to prevent API rate limiting / latency.
	// A background cron job should ideally update the prices.
	flips, err := core.RunAnalysis(app.Client, app.Capital, app.VolThreshold, app.Limit, false, "", config, req.Flips, req.FailedSells)
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
