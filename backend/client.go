package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/lucasmolander/osrs-ge-flip-analyzer/core"
)

const (
	baseURLLatest     = "https://prices.runescape.wiki/api/v1/osrs/latest"
	baseURL5m         = "https://prices.runescape.wiki/api/v1/osrs/5m"
	baseURL1h         = "https://prices.runescape.wiki/api/v1/osrs/1h"
	baseURL24h        = "https://prices.runescape.wiki/api/v1/osrs/24h"
	baseURLMapping    = "https://prices.runescape.wiki/api/v1/osrs/mapping"
	baseURLTimeSeries = "https://prices.runescape.wiki/api/v1/osrs/timeseries"
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
func (c *OSRSClient) FetchLatestPrices(ctx context.Context) (map[string]core.LatestPrice, error) {
	var response core.LatestPricesResponse
	if err := c.executeRequest(ctx, baseURLLatest, &response); err != nil {
		return nil, err
	}
	return response.Data, nil
}

// FetchHourlyVolumes retrieves the 1-hour trading volume and average prices.
// Returns the Unix timestamp of the 1-hour block and the map of item volumes.
func (c *OSRSClient) FetchHourlyVolumes(ctx context.Context) (int64, map[string]core.HourlyVolume, error) {
	var response core.HourlyVolumesResponse
	if err := c.executeRequest(ctx, baseURL1h, &response); err != nil {
		return 0, nil, err
	}
	return response.Timestamp, response.Data, nil
}

// Fetch5mVolumes retrieves the 5-minute trading volume and average prices.
// Returns the Unix timestamp of the 5-minute block and the map of item volumes.
func (c *OSRSClient) Fetch5mVolumes(ctx context.Context) (int64, map[string]core.HourlyVolume, error) {
	var response core.HourlyVolumesResponse
	if err := c.executeRequest(ctx, baseURL5m, &response); err != nil {
		return 0, nil, err
	}
	return response.Timestamp, response.Data, nil
}

// Fetch24hVolumes retrieves the 24-hour trading volume and average prices.
// Returns the Unix timestamp of the 24-hour block and the map of item volumes.
func (c *OSRSClient) Fetch24hVolumes(ctx context.Context) (int64, map[string]core.HourlyVolume, error) {
	var response core.HourlyVolumesResponse
	if err := c.executeRequest(ctx, baseURL24h, &response); err != nil {
		return 0, nil, err
	}
	return response.Timestamp, response.Data, nil
}

// FetchItemMapping retrieves the static item definitions/metadata (names, limits, alch values).
func (c *OSRSClient) FetchItemMapping(ctx context.Context) ([]core.ItemMetadata, error) {
	var response []core.ItemMetadata
	if err := c.executeRequest(ctx, baseURLMapping, &response); err != nil {
		return nil, err
	}
	return response, nil
}

// FetchHistoricalPrices retrieves the 1-hour average prices and volumes at a specific historical timestamp.
func (c *OSRSClient) FetchHistoricalPrices(ctx context.Context, timestamp int64) (map[string]core.HourlyVolume, error) {
	url := fmt.Sprintf("%s?timestamp=%d", baseURL1h, timestamp)
	var response core.HourlyVolumesResponse
	if err := c.executeRequest(ctx, url, &response); err != nil {
		return nil, err
	}
	return response.Data, nil
}

// FetchHistorical5m retrieves the 5-minute snapshot at a specific historical timestamp.
func (c *OSRSClient) FetchHistorical5m(ctx context.Context, timestamp int64) (map[string]core.HourlyVolume, error) {
	url := fmt.Sprintf("%s?timestamp=%d", baseURL5m, timestamp)
	var response core.HourlyVolumesResponse
	if err := c.executeRequest(ctx, url, &response); err != nil {
		return nil, err
	}
	return response.Data, nil
}

// FetchLatestPrice retrieves the latest price info for a specific item ID.
func (c *OSRSClient) FetchLatestPrice(ctx context.Context, id int) (map[string]core.LatestPrice, error) {
	url := fmt.Sprintf("%s?id=%d", baseURLLatest, id)
	var response core.LatestPricesResponse
	if err := c.executeRequest(ctx, url, &response); err != nil {
		return nil, err
	}
	return response.Data, nil
}

// FetchTimeSeries retrieves the high and low prices of an item at a given interval.
// Valid timesteps are "5m", "1h", "6h", and "24h".
func (c *OSRSClient) FetchTimeSeries(ctx context.Context, id int, timestep string) ([]core.TimeSeriesDataPoint, error) {
	url := fmt.Sprintf("%s?id=%d&timestep=%s", baseURLTimeSeries, id, timestep)
	var response core.TimeSeriesResponse
	if err := c.executeRequest(ctx, url, &response); err != nil {
		return nil, err
	}
	return response.Data, nil
}
