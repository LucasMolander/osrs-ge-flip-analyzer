package backend

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

// BackupPayload represents the complete serialized backup structure.
type BackupPayload struct {
	Version   int               `json:"version"`
	Timestamp time.Time         `json:"timestamp"`
	Files     map[string]string `json:"files"` // maps platform-independent relative path to base64 content
}

// BackupData serializes all persistent data files into a JSON payload.
func BackupData() ([]byte, error) {
	payload := BackupPayload{
		Version:   1,
		Timestamp: time.Now().UTC(),
		Files:     make(map[string]string),
	}

	dirs := []string{"flips", "failed_sells", "prices", "reports"}

	for _, dir := range dirs {
		files, err := Store.ListDir(dir)
		if err != nil {
			// Directory might not exist yet if no operations have occurred.
			// This is normal, so we just skip it.
			continue
		}

		for _, file := range files {
			// Construct platform-independent relative path
			relPath := dir + "/" + file

			raw, err := Store.ReadRaw(relPath)
			if err != nil {
				return nil, fmt.Errorf("failed to read file %s: %w", relPath, err)
			}

			// Encode raw content to base64
			payload.Files[relPath] = base64.StdEncoding.EncodeToString(raw)
		}
	}

	backupJSON, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal backup JSON: %w", err)
	}

	return backupJSON, nil
}

// RestoreData extracts and writes all files from a backup JSON payload.
func RestoreData(backupJSON []byte) error {
	var payload BackupPayload
	if err := json.Unmarshal(backupJSON, &payload); err != nil {
		return fmt.Errorf("failed to unmarshal backup JSON: %w", err)
	}

	if payload.Version != 1 {
		return fmt.Errorf("unsupported backup version: %d", payload.Version)
	}

	for relPath, encoded := range payload.Files {
		raw, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return fmt.Errorf("failed to decode base64 for file %s: %w", relPath, err)
		}

		// Write raw content to storage
		if err := Store.WriteRaw(relPath, raw); err != nil {
			return fmt.Errorf("failed to write restored file %s: %w", relPath, err)
		}
	}

	return nil
}
