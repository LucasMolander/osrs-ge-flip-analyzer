package web

import (
	"fmt"
	"net/http"

	"github.com/lucasmolander/osrs-ge-flip-analyzer/core"
)

// AppServer holds the dependencies for the web server handlers.
type AppServer struct {
	Client       *core.OSRSClient
	Capital      int64
	VolThreshold int64
	Limit        int
	Store        core.Storage
}

// StartServer initializes the HTTP handlers and starts listening on the given port.
func StartServer(port string, client *core.OSRSClient, capital, volThreshold int64, limit int, store core.Storage) error {
	app := &AppServer{
		Client:       client,
		Capital:      capital,
		VolThreshold: volThreshold,
		Limit:        limit,
		Store:        store,
	}

	mux := http.NewServeMux()

	// API Endpoints
	mux.HandleFunc("/api/report", app.apiReportHandler)
	mux.HandleFunc("/api/flips", app.apiRecordFlipHandler)
	mux.HandleFunc("/api/failed-buys", app.apiRecordFailedBuyHandler)

	// Static File Server for the Vue 3 Frontend
	fs := http.FileServer(http.Dir("./web/frontend"))
	mux.Handle("/", fs)

	addr := fmt.Sprintf(":%s", port)
	fmt.Printf("Starting web dashboard on http://localhost:%s\n", port)
	return http.ListenAndServe(addr, mux)
}
