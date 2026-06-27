package core

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const (
	baseURLLatest  = "https://prices.runescape.wiki/api/v1/osrs/latest"
	baseURL5m      = "https://prices.runescape.wiki/api/v1/osrs/5m"
	baseURL1h      = "https://prices.runescape.wiki/api/v1/osrs/1h"
	baseURL24h     = "https://prices.runescape.wiki/api/v1/osrs/24h"
	baseURLMapping = "https://prices.runescape.wiki/api/v1/osrs/mapping"
)

// OSRSClient handles all communications with the OSRS Wiki Grand Exchange APIs.
type OSRSClient struct {
	userAgent  string
	httpClient *http.Client
}

// NewClient returns a new OSRSClient instance with the given user agent.
func NewClient(userAgent string) *OSRSClient {
	if userAgent == "" {
		userAgent = "osrs-ge-flip-analyzer - @lucasmolander"
	}
	return &OSRSClient{
		userAgent: userAgent,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// executeRequest performs the HTTP request with the configured User-Agent and decodes the JSON response.
func (c *OSRSClient) executeRequest(ctx context.Context, url string, target interface{}) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set the mandatory User-Agent
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API returned non-200 status: %d (%s)", resp.StatusCode, resp.Status)
	}

	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("failed to decode JSON response: %w", err)
	}

	return nil
}

// FetchLatestPrices retrieves the latest high and low price info for all tradeable items.
func (c *OSRSClient) FetchLatestPrices(ctx context.Context) (map[string]LatestPrice, error) {
	var response LatestPricesResponse
	if err := c.executeRequest(ctx, baseURLLatest, &response); err != nil {
		return nil, err
	}
	return response.Data, nil
}

// FetchHourlyVolumes retrieves the 1-hour trading volume and average prices.
// Returns the Unix timestamp of the 1-hour block and the map of item volumes.
func (c *OSRSClient) FetchHourlyVolumes(ctx context.Context) (int64, map[string]HourlyVolume, error) {
	var response HourlyVolumesResponse
	if err := c.executeRequest(ctx, baseURL1h, &response); err != nil {
		return 0, nil, err
	}
	return response.Timestamp, response.Data, nil
}

// Fetch5mVolumes retrieves the 5-minute trading volume and average prices.
// Returns the Unix timestamp of the 5-minute block and the map of item volumes.
func (c *OSRSClient) Fetch5mVolumes(ctx context.Context) (int64, map[string]HourlyVolume, error) {
	var response HourlyVolumesResponse
	if err := c.executeRequest(ctx, baseURL5m, &response); err != nil {
		return 0, nil, err
	}
	return response.Timestamp, response.Data, nil
}

// Fetch24hVolumes retrieves the 24-hour trading volume and average prices.
// Returns the Unix timestamp of the 24-hour block and the map of item volumes.
func (c *OSRSClient) Fetch24hVolumes(ctx context.Context) (int64, map[string]HourlyVolume, error) {
	var response HourlyVolumesResponse
	if err := c.executeRequest(ctx, baseURL24h, &response); err != nil {
		return 0, nil, err
	}
	return response.Timestamp, response.Data, nil
}

// FetchItemMapping retrieves the static item definitions/metadata (names, limits, alch values).
func (c *OSRSClient) FetchItemMapping(ctx context.Context) ([]ItemMetadata, error) {
	var response []ItemMetadata
	if err := c.executeRequest(ctx, baseURLMapping, &response); err != nil {
		return nil, err
	}
	return response, nil
}

// FetchHistoricalPrices retrieves the 1-hour average prices and volumes at a specific historical timestamp.
func (c *OSRSClient) FetchHistoricalPrices(ctx context.Context, timestamp int64) (map[string]HourlyVolume, error) {
	url := fmt.Sprintf("%s?timestamp=%d", baseURL1h, timestamp)
	var response HourlyVolumesResponse
	if err := c.executeRequest(ctx, url, &response); err != nil {
		return nil, err
	}
	return response.Data, nil
}

// FetchHistorical5m retrieves the 5-minute snapshot at a specific historical timestamp.
func (c *OSRSClient) FetchHistorical5m(ctx context.Context, timestamp int64) (map[string]HourlyVolume, error) {
	url := fmt.Sprintf("%s?timestamp=%d", baseURL5m, timestamp)
	var response HourlyVolumesResponse
	if err := c.executeRequest(ctx, url, &response); err != nil {
		return nil, err
	}
	return response.Data, nil
}
