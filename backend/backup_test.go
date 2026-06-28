package backend

import (
"encoding/json"
"os"
"testing"
)
func TestBackupAndRestore(t *testing.T) {
	// 1. Create temporary directory
	tempDir, err := os.MkdirTemp("", "ge-analyzer-backup-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Save original working directory and restore it
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	defer os.Chdir(oldWD)

	// Change to temporary directory to isolate storage operations
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Failed to change working directory: %v", err)
	}

	// Save original store and restore it after test
	oldStore := Store
	defer func() { Store = oldStore }()

	// Set global store to LocalStorage
	Store = &LocalStorage{}

	// 2. Create mock directories and write some test files
	if err := os.MkdirAll("flips", 0755); err != nil {
		t.Fatalf("Failed to create flips dir: %v", err)
	}
	if err := os.MkdirAll("failed_sells", 0755); err != nil {
		t.Fatalf("Failed to create failed_sells dir: %v", err)
	}
	if err := os.MkdirAll("reports", 0755); err != nil {
		t.Fatalf("Failed to create reports dir: %v", err)
	}

	flipContent := []byte(`{"ItemID": 1, "Quantity": 100}`)
	failedContent := []byte(`{"ItemID": 2, "TargetQty": 500}`)
	reportContent := []byte("# Test Report\n- Item 1\n- Item 2")

	if err := Store.WriteRaw("flips/flip_1.json", flipContent); err != nil {
		t.Fatalf("Failed to write flip: %v", err)
	}
	if err := Store.WriteRaw("failed_sells/failed_sell_2.json", failedContent); err != nil {
		t.Fatalf("Failed to write failed buy: %v", err)
	}
	if err := Store.WriteRaw("reports/report_latest.md", reportContent); err != nil {
		t.Fatalf("Failed to write report: %v", err)
	}

	// 3. Trigger backup
	backupJSON, err := BackupData()
	if err != nil {
		t.Fatalf("BackupData failed: %v", err)
	}

	// 4. Verify backup JSON contains all files and correct contents
	var payload BackupPayload
	if err := json.Unmarshal(backupJSON, &payload); err != nil {
		t.Fatalf("Failed to unmarshal backup JSON: %v", err)
	}

	if payload.Version != 1 {
		t.Errorf("Expected version 1, got %d", payload.Version)
	}

	if len(payload.Files) != 3 {
		t.Errorf("Expected 3 files in backup, got %d", len(payload.Files))
	}

	// Verify specific file presence
	for _, expectedPath := range []string{"flips/flip_1.json", "failed_sells/failed_sell_2.json", "reports/report_latest.md"} {
		if _, ok := payload.Files[expectedPath]; !ok {
			t.Errorf("Expected backup to contain file %s, but it was missing", expectedPath)
		}
	}

	// 5. Delete all files locally to simulate database loss
	if err := os.RemoveAll("flips"); err != nil {
		t.Fatalf("Failed to clean flips: %v", err)
	}
	if err := os.RemoveAll("failed_sells"); err != nil {
		t.Fatalf("Failed to clean failed_sells: %v", err)
	}
	if err := os.RemoveAll("reports"); err != nil {
		t.Fatalf("Failed to clean reports: %v", err)
	}

	// 6. Trigger restore
	if err := RestoreData(backupJSON); err != nil {
		t.Fatalf("RestoreData failed: %v", err)
	}

	// 7. Verify files are perfectly restored
	restoredFlip, err := Store.ReadRaw("flips/flip_1.json")
	if err != nil {
		t.Fatalf("Failed to read restored flip: %v", err)
	}
	if string(restoredFlip) != string(flipContent) {
		t.Errorf("Restored flip content mismatch. Expected %s, got %s", flipContent, restoredFlip)
	}

	restoredFailed, err := Store.ReadRaw("failed_sells/failed_sell_2.json")
	if err != nil {
		t.Fatalf("Failed to read restored failed buy: %v", err)
	}
	if string(restoredFailed) != string(failedContent) {
		t.Errorf("Restored failed buy content mismatch. Expected %s, got %s", failedContent, restoredFailed)
	}

	restoredReport, err := Store.ReadRaw("reports/report_latest.md")
	if err != nil {
		t.Fatalf("Failed to read restored report: %v", err)
	}
	if string(restoredReport) != string(reportContent) {
		t.Errorf("Restored report content mismatch. Expected %s, got %s", reportContent, restoredReport)
	}
}
