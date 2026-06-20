package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"cloud.google.com/go/storage"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iterator"
)

var Store Storage

// Storage defines the interface for file-like operations.
type Storage interface {
	Read(path string, target interface{}) error
	ReadRaw(path string) ([]byte, error)
	Write(path string, data interface{}) (string, error)
	WriteRaw(path string, content []byte) error
	FindLatestFile(dir, prefix string) (string, int64, error)
	ListDir(dir string) ([]string, error)
}

// LocalStorage implements Storage using the local filesystem.
type LocalStorage struct{}

// ReadRaw reads and returns the raw content of a local file.
func (l *LocalStorage) ReadRaw(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// Read reads and decodes JSON from a local file.
func (l *LocalStorage) Read(path string, target interface{}) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return json.NewDecoder(file).Decode(target)
}

// Write encodes and writes JSON to a local file.
func (l *LocalStorage) Write(path string, data interface{}) (string, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory %s: %w", dir, err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		if os.IsExist(err) {
			// Race condition: file already exists, treat as success
			return path, nil
		}
		return "", fmt.Errorf("failed to create file %s: %w", path, err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		return "", fmt.Errorf("failed to encode JSON to %s: %w", path, err)
	}
	return path, nil
}

// WriteRaw writes raw bytes to a local file.
func (l *LocalStorage) WriteRaw(path string, content []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		if os.IsExist(err) {
			// Race condition: file already exists, treat as success
			return nil
		}
		return fmt.Errorf("failed to create file %s: %w", path, err)
	}
	defer file.Close()
	_, err = file.Write(content)
	return err
}

// FindLatestFile locates the latest file matching the pattern in the local directory.
func (l *LocalStorage) FindLatestFile(dir, prefix string) (string, int64, error) {
	return FindLatestFile(dir, prefix)
}

// ListDir lists all non-directory files in a local directory.
func (l *LocalStorage) ListDir(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var files []string
	for _, entry := range entries {
		if !entry.IsDir() {
			files = append(files, entry.Name())
		}
	}
	return files, nil
}

// GCSStorage implements Storage using Google Cloud Storage.
type GCSStorage struct {
	bucketName string
	client     *storage.Client
}

// NewGCSStorage instantiates a new GCS storage provider.
func NewGCSStorage(ctx context.Context, bucketName string) (*GCSStorage, error) {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCS client: %w", err)
	}
	return &GCSStorage{
		bucketName: bucketName,
		client:     client,
	}, nil
}

// Read reads and decodes JSON from a GCS object.
func (g *GCSStorage) Read(path string, target interface{}) error {
	ctx := context.Background()
	rc, err := g.client.Bucket(g.bucketName).Object(path).NewReader(ctx)
	if err != nil {
		return err
	}
	defer rc.Close()
	return json.NewDecoder(rc).Decode(target)
}

// ReadRaw reads and returns the raw content of a GCS object.
func (g *GCSStorage) ReadRaw(path string) ([]byte, error) {
	ctx := context.Background()
	rc, err := g.client.Bucket(g.bucketName).Object(path).NewReader(ctx)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// Write encodes and writes JSON to a GCS object.
func (g *GCSStorage) Write(path string, data interface{}) (string, error) {
	ctx := context.Background()
	wc := g.client.Bucket(g.bucketName).Object(path).If(storage.Conditions{DoesNotExist: true}).NewWriter(ctx)

	encoder := json.NewEncoder(wc)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		wc.Close()
		return "", fmt.Errorf("failed to encode JSON to GCS object %s: %w", path, err)
	}
	if err := wc.Close(); err != nil {
		if gErr, ok := err.(*googleapi.Error); ok && gErr.Code == 412 {
			// Precondition Failed: object already exists. Treat as success.
			return fmt.Sprintf("gs://%s/%s", g.bucketName, path), nil
		}
		return "", fmt.Errorf("failed to flush GCS object %s: %w", path, err)
	}
	return fmt.Sprintf("gs://%s/%s", g.bucketName, path), nil
}

// WriteRaw writes raw bytes to a GCS object.
func (g *GCSStorage) WriteRaw(path string, content []byte) error {
	ctx := context.Background()
	wc := g.client.Bucket(g.bucketName).Object(path).If(storage.Conditions{DoesNotExist: true}).NewWriter(ctx)

	if _, err := wc.Write(content); err != nil {
		wc.Close()
		return err
	}
	if err := wc.Close(); err != nil {
		if gErr, ok := err.(*googleapi.Error); ok && gErr.Code == 412 {
			// Precondition Failed: object already exists. Treat as success.
			return nil
		}
		return err
	}
	return nil
}

// FindLatestFile finds the latest JSON object matching prefix inside bucket/dir.
func (g *GCSStorage) FindLatestFile(dir, prefix string) (string, int64, error) {
	ctx := context.Background()
	queryPrefix := fmt.Sprintf("%s/%s_", dir, prefix)
	it := g.client.Bucket(g.bucketName).Objects(ctx, &storage.Query{Prefix: queryPrefix})

	var latestPath string
	var latestTime int64

	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return "", 0, fmt.Errorf("failed to list GCS objects: %w", err)
		}

		name := attrs.Name
		if len(name) > len(queryPrefix)+5 && name[len(name)-5:] == ".json" {
			var ts int64
			_, err := fmt.Sscanf(name[len(queryPrefix):len(name)-5], "%d", &ts)
			if err == nil && ts > latestTime {
				latestTime = ts
				latestPath = name
			}
		}
	}

	if latestPath == "" {
		return "", 0, fmt.Errorf("no GCS objects found matching gs://%s/%s*.json", g.bucketName, queryPrefix)
	}

	return latestPath, latestTime, nil
}

// ListDir lists all objects in GCS with the given directory prefix.
func (g *GCSStorage) ListDir(dir string) ([]string, error) {
	ctx := context.Background()
	prefix := dir
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	it := g.client.Bucket(g.bucketName).Objects(ctx, &storage.Query{
		Prefix:    prefix,
		Delimiter: "/",
	})

	var files []string
	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to list GCS objects under %s: %w", dir, err)
		}
		if !strings.HasSuffix(attrs.Name, "/") {
			filename := filepath.Base(attrs.Name)
			files = append(files, filename)
		}
	}
	return files, nil
}

// NewStorage returns a GCSStorage if running in server mode (and requires GCS_BUCKET to be set).
// Otherwise, it returns a LocalStorage for CLI mode.
func NewStorage(isServer bool) (Storage, error) {
	bucketName := os.Getenv("GCS_BUCKET")
	if bucketName != "" {
		ctx := context.Background()
		return NewGCSStorage(ctx, bucketName)
	}
	
	// CLI mode always uses local storage
	return &LocalStorage{}, nil
}
