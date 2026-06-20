package web

import (
	"fmt"
	"net/http"
	"os"

	"github.com/lucasmolander/osrs-ge-flip-analyzer/core"
)

// AppServer holds the dependencies for the web server handlers.
type AppServer struct {
	Client       *core.OSRSClient
	Capital      int64
	VolThreshold int64
	Limit        int
	Store        core.Storage
	Config       *core.RankingConfig
}

// StartServer initializes the HTTP handlers and starts listening on the given port.
func StartServer(port string, client *core.OSRSClient, capital, volThreshold int64, limit int, store core.Storage, config *core.RankingConfig) error {
	app := &AppServer{
		Client:       client,
		Capital:      capital,
		VolThreshold: volThreshold,
		Limit:        limit,
		Store:        store,
		Config:       config,
	}

	// API router wrapped with BasicAuth
	apiMux := http.NewServeMux()
	apiMux.HandleFunc("/api/report", app.apiReportHandler)
	apiMux.HandleFunc("/api/flips", app.apiRecordFlipHandler)
	apiMux.HandleFunc("/api/failed-buys", app.apiRecordFailedBuyHandler)
	apiMux.HandleFunc("/api/items", app.apiItemsHandler)
	apiMux.HandleFunc("/api/sync/prices", app.apiSyncPricesHandler)
	apiMux.HandleFunc("/api/sync/metadata", app.apiSyncMetadataHandler)
	apiMux.HandleFunc("/api/backup", app.apiBackupHandler)
	apiMux.HandleFunc("/api/restore", app.apiRestoreHandler)
	apiMux.HandleFunc("/api/history/flips", app.apiFlipsHistoryHandler)
	apiMux.HandleFunc("/api/history/failed-buys", app.apiFailedBuysHistoryHandler)

	// App-Level Authentication
	username := os.Getenv("AUTH_USERNAME")
	password := os.Getenv("AUTH_PASSWORD")

	if username == "" || password == "" {
		fmt.Printf("Error: AUTH_USERNAME or AUTH_PASSWORD environment variables are not set.\n")
		// Serve a static webpage indicating configuration error
		mux := http.NewServeMux()
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`
				<html>
				<head><title>Configuration Error</title></head>
				<body style="font-family: sans-serif; display: flex; justify-content: center; align-items: center; height: 100vh; background: #0f172a; color: white; margin: 0;">
					<div style="text-align: center; padding: 2rem; background: #1e293b; border-radius: 8px; box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.1), 0 2px 4px -1px rgba(0, 0, 0, 0.06);">
						<h1 style="color: #ef4444; margin-top: 0;">Configuration Error</h1>
						<p>Authentication credentials were not provided.</p>
						<p style="color: #94a3b8;">Please restart the application with <code>AUTH_USERNAME</code> and <code>AUTH_PASSWORD</code> environment variables set.</p>
					</div>
				</body>
				</html>
			`))
		})
		
		addr := fmt.Sprintf(":%s", port)
		fmt.Printf("Starting configuration error page on http://localhost:%s\n", port)
		return http.ListenAndServe(addr, mux)
	}

	authApiMux := BasicAuthMiddleware(apiMux, username, password)

	// Main router
	mux := http.NewServeMux()
	mux.Handle("/api/", authApiMux) // All /api/ routes are authenticated

	// Static File Server for the Vue 3 Frontend (Unauthenticated)
	fs := http.FileServer(http.Dir("./web/frontend"))
	mux.Handle("/", fs)

	addr := fmt.Sprintf(":%s", port)
	fmt.Printf("Starting web dashboard on http://localhost:%s\n", port)

	return http.ListenAndServe(addr, mux)
}

// BasicAuthMiddleware wraps an http.Handler with basic authentication.
func BasicAuthMiddleware(next http.Handler, username, password string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != username || pass != password {
			// Intentionally omitting WWW-Authenticate header to prevent native browser login popup.
			// This allows the Vue frontend to display a custom login page instead.
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
