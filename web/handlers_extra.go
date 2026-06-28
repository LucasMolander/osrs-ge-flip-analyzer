package web

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"sort"
	"time"

	"github.com/lucasmolander/osrs-ge-flip-analyzer/backend"
	"github.com/lucasmolander/osrs-ge-flip-analyzer/core"
)

// ItemDict represents a simplified item for the autocomplete dropdown
type ItemDict struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// apiItemsHandler returns a simplified list of items for the frontend autocomplete
func (app *AppServer) apiItemsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		sendError(w, nil, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	metadataPath, _, err := backend.Store.FindLatestFile("item_data", "item_metadata")
	if err != nil {
		sendError(w, err, "Item metadata not found. Please sync metadata first.", http.StatusNotFound)
		return
	}

	var metadata map[int]core.ItemMetadata
	if err := backend.LoadJSON(metadataPath, &metadata); err != nil {
		sendError(w, err, "Failed to load item metadata", http.StatusInternalServerError)
		return
	}

	var items []ItemDict
	for id, data := range metadata {
		items = append(items, ItemDict{
			ID:   id,
			Name: data.Name,
		})
	}

	// Sort alphabetically by name
	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items)
}

// apiSyncPricesHandler triggers a download of the latest prices
func (app *AppServer) apiSyncPricesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		sendError(w, nil, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	runTs := time.Now().Unix()
	fetched, err := backend.DownloadPrices(r.Context(), app.Client, runTs)
	if err != nil {
		sendError(w, err, "Failed to sync prices", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":           "success",
		"message":          "Prices synced successfully.",
		"fetched_new_data": fetched,
	})
}

// apiSyncMetadataHandler triggers a download of the latest item metadata
func (app *AppServer) apiSyncMetadataHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		sendError(w, nil, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	timestamp := time.Now().Unix()
	if _, _, err := backend.DownloadMetadata(r.Context(), app.Client, timestamp); err != nil {
		sendError(w, err, "Failed to sync metadata", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "Metadata synced successfully."})
}

// apiBackupHandler creates and downloads a full JSON backup of the system
func (app *AppServer) apiBackupHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		sendError(w, nil, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	backupJSON, err := backend.BackupData()
	if err != nil {
		sendError(w, err, "Failed to create backup", http.StatusInternalServerError)
		return
	}

	filename := fmt.Sprintf("ge_analyzer_backup_%d.json", time.Now().Unix())
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	w.Header().Set("Content-Type", "application/json")
	w.Write(backupJSON)
}

// apiRestoreHandler accepts a JSON backup file and restores the database
func (app *AppServer) apiRestoreHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		sendError(w, nil, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse multipart form, max 50 MB
	if err := r.ParseMultipartForm(50 << 20); err != nil {
		sendError(w, err, "Failed to parse form", http.StatusBadRequest)
		return
	}

	file, _, err := r.FormFile("backup_file")
	if err != nil {
		sendError(w, err, "Missing backup_file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	backupJSON, err := ioutil.ReadAll(file)
	if err != nil {
		sendError(w, err, "Failed to read uploaded file", http.StatusInternalServerError)
		return
	}

	if err := backend.RestoreData(backupJSON); err != nil {
		sendError(w, err, "Failed to restore data", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "Database restored successfully."})
}

// apiConfigDefaultHandler returns the system's default RankingConfig
func (app *AppServer) apiConfigDefaultHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		sendError(w, nil, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(core.DefaultRankingConfig())
}

// apiProfilingMetricsHandler returns the internal phase profiler's statistics.
func (app *AppServer) apiProfilingMetricsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		sendError(w, nil, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	metrics := core.GlobalProfiler.GetMetrics()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}
