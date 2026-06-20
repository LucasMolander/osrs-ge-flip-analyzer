package core

import (
	"fmt"
	"time"
	"math"
	"sync"
)

func SaveJSON(dir, prefix string, timestamp int64, data interface{}) (string, error) {
	path := fmt.Sprintf("%s/%s_%d.json", dir, prefix, timestamp)
	return Store.Write(path, data)
}

func DownloadPrices(client *OSRSClient, timestamp int64) error {
	// 0. Check if we already have recent data
	_, latestTs, err := Store.FindLatestFile("prices", "prices")
	if err == nil && (timestamp-latestTs) < 60 {
		fmt.Printf("Data from the past minute (%d) already exists. Skipping download.\n", latestTs)
		return nil
	}

	prices, err := client.FetchLatestPrices()
	if err != nil {
		return fmt.Errorf("fetching latest prices: %w", err)
	}
	_, err = SaveJSON("prices", "prices", timestamp, prices)
	if err != nil {
		return fmt.Errorf("saving prices: %w", err)
	}

	_, volumes, err := client.FetchHourlyVolumes()
	if err != nil {
		return fmt.Errorf("fetching hourly volumes: %w", err)
	}
	_, err = SaveJSON("prices", "volumes", timestamp, volumes)
	if err != nil {
		return fmt.Errorf("saving volumes: %w", err)
	}

	_, volumes5m, err := client.Fetch5mVolumes()
	if err != nil {
		return fmt.Errorf("fetching 5m volumes: %w", err)
	}
	_, err = SaveJSON("prices", "volumes_5m", timestamp, volumes5m)
	if err != nil {
		return fmt.Errorf("saving 5m volumes: %w", err)
	}

	_, volumes24hAvg, err := client.Fetch24hVolumes()
	if err != nil {
		return fmt.Errorf("fetching 24h volumes avg: %w", err)
	}
	_, err = SaveJSON("prices", "volumes_24h_avg", timestamp, volumes24hAvg)
	if err != nil {
		return fmt.Errorf("saving 24h volumes avg: %w", err)
	}

	// Align timestamp to the hourly boundary (multiples of 3600)
	alignedNow := (timestamp / 3600) * 3600

	// 1. Fetch 1 hour ago
	t1h := alignedNow - 3600
	hist1h, err := client.FetchHistoricalPrices(t1h)
	if err != nil {
		return fmt.Errorf("fetching historical prices (1h ago): %w", err)
	}
	_, err = SaveJSON("prices", "prices_1h", timestamp, hist1h)
	if err != nil {
		return fmt.Errorf("saving 1h historical prices: %w", err)
	}

	// 2. Fetch 24 hours ago
	t24h := alignedNow - 86400
	hist24h, err := client.FetchHistoricalPrices(t24h)
	if err != nil {
		return fmt.Errorf("fetching historical prices (24h ago): %w", err)
	}
	_, err = SaveJSON("prices", "prices_24h", timestamp, hist24h)
	if err != nil {
		return fmt.Errorf("saving 24h historical prices: %w", err)
	}

	// 3. Fetch 30 days ago
	t30d := alignedNow - 2592000
	hist30d, err := client.FetchHistoricalPrices(t30d)
	if err != nil {
		return fmt.Errorf("fetching historical prices (30d ago): %w", err)
	}
	_, err = SaveJSON("prices", "prices_30d", timestamp, hist30d)
	if err != nil {
		return fmt.Errorf("saving 30d historical prices: %w", err)
	}
	// 4. Fetch 24 continuous 5m ticks for Outlier Math
	aligned5m := (timestamp / 300) * 300
	rolling24 := make([]map[string]HourlyVolume, 24)
	var wg sync.WaitGroup
	var errMu sync.Mutex
	var rollingErr error

	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			// i=0 means 5 minutes ago, i=23 means 120 minutes ago (we exclude the current tick)
			tTick := aligned5m - int64((idx+1)*300)
			tickData, err := client.FetchHistorical5m(tTick)
			if err != nil {
				errMu.Lock()
				rollingErr = err
				errMu.Unlock()
				return
			}
			rolling24[idx] = tickData
		}(i)
	}
	wg.Wait()

	if rollingErr != nil {
		return fmt.Errorf("fetching rolling 5m ticks: %w", rollingErr)
	}

	_, err = SaveJSON("prices", "prices_rolling_24", timestamp, rolling24)
	if err != nil {
		return fmt.Errorf("saving rolling 24 ticks: %w", err)
	}

	return nil
}

func DownloadMetadata(client *OSRSClient, timestamp int64) (map[int]ItemMetadata, string, error) {
	items, err := client.FetchItemMapping()
	if err != nil {
		return nil, "", fmt.Errorf("fetching item mapping: %w", err)
	}

	// Build a map of ID -> ItemMetadata as requested
	metadataMap := make(map[int]ItemMetadata)
	for _, item := range items {
		metadataMap[item.ID] = item
	}

	metadataFile, err := SaveJSON("item_data", "item_metadata", timestamp, metadataMap)
	if err != nil {
		return nil, "", fmt.Errorf("saving item metadata: %w", err)
	}
	return metadataMap, metadataFile, nil
}

// RunAnalysis orchestrates the fetching of prices, parsing, scoring, and report generation.
func RunAnalysis(client *OSRSClient, capital, vol int64, limit int, forceDownload bool, filterName string, config *RankingConfig, flips []FlipRecord, failedSells []FailedSellRecord) ([]ReportItem, error) {
	runTs := time.Now().Unix()

	// 1. Download unless skipped
	if forceDownload {
		fmt.Println("Downloading latest price and volume data...")
		if err := DownloadPrices(client, runTs); err != nil {
			return nil, fmt.Errorf("error downloading prices: %w", err)
		}
	} else {
		// Verify we at least have one cached file of both prices and rolling_24.
		// If not, fetch anyway to prevent crashing on boot.
		_, _, err1 := Store.FindLatestFile("prices", "prices")
		_, _, err2 := Store.FindLatestFile("prices", "prices_rolling_24")
		if err1 != nil || err2 != nil {
			fmt.Println("Missing required cached prices (like rolling_24). Fetching them now as fallback...")
			if err := DownloadPrices(client, runTs); err != nil {
				return nil, fmt.Errorf("error downloading prices during fallback: %w", err)
			}
		}
	}

	// 2. Locate latest price and volume files
	pricesPath, _, err := Store.FindLatestFile("prices", "prices")
	if err != nil {
		return nil, fmt.Errorf("error locating latest prices file: %w", err)
	}
	volumesPath, _, err := Store.FindLatestFile("prices", "volumes")
	if err != nil {
		return nil, fmt.Errorf("error locating latest volumes file: %w", err)
	}
	volumes5mPath, _, err := Store.FindLatestFile("prices", "volumes_5m")
	if err != nil {
		return nil, fmt.Errorf("error locating latest 5m volumes file: %w", err)
	}
	volumes24hAvgPath, _, err := Store.FindLatestFile("prices", "volumes_24h_avg")
	if err != nil {
		return nil, fmt.Errorf("error locating latest 24h volumes avg file: %w", err)
	}
	prices1hPath, _, err := Store.FindLatestFile("prices", "prices_1h")
	if err != nil {
		return nil, fmt.Errorf("error locating latest 1h prices file: %w", err)
	}
	prices24hPath, _, err := Store.FindLatestFile("prices", "prices_24h")
	if err != nil {
		return nil, fmt.Errorf("error locating latest 24h prices file: %w", err)
	}
	prices30dPath, _, err := Store.FindLatestFile("prices", "prices_30d")
	if err != nil {
		return nil, fmt.Errorf("error locating latest 30d prices file: %w", err)
	}
	pricesRollingPath, _, err := Store.FindLatestFile("prices", "prices_rolling_24")
	if err != nil {
		return nil, fmt.Errorf("error locating latest rolling 24 prices file: %w", err)
	}

	// 3. Locate or fetch latest metadata file
	var metadata map[int]ItemMetadata
	metadataPath, _, err := Store.FindLatestFile("item_data", "item_metadata")
	if err != nil {
		fmt.Println("No cached item metadata found. Fetching it now...")
		metadata, metadataPath, err = DownloadMetadata(client, runTs)
		if err != nil {
			return nil, fmt.Errorf("error fetching item metadata: %w", err)
		}
		fmt.Printf("Metadata cached at: %s\n", metadataPath)
	} else {
		// Load from file
		metadata = make(map[int]ItemMetadata)
		if err := LoadJSON(metadataPath, &metadata); err != nil {
			return nil, fmt.Errorf("error loading item metadata from %s: %w", metadataPath, err)
		}
	}

	// 4. Load prices and volumes
	var prices map[string]LatestPrice
	if err := LoadJSON(pricesPath, &prices); err != nil {
		return nil, fmt.Errorf("error loading prices from %s: %w", pricesPath, err)
	}

	var volumes map[string]HourlyVolume
	if err := LoadJSON(volumesPath, &volumes); err != nil {
		return nil, fmt.Errorf("error loading volumes from %s: %w", volumesPath, err)
	}

	var vol5m map[string]HourlyVolume
	if err := LoadJSON(volumes5mPath, &vol5m); err != nil {
		return nil, fmt.Errorf("error loading 5m volumes from %s: %w", volumes5mPath, err)
	}

	var vol24h map[string]HourlyVolume
	if err := LoadJSON(volumes24hAvgPath, &vol24h); err != nil {
		return nil, fmt.Errorf("error loading 24h volumes from %s: %w", volumes24hAvgPath, err)
	}

	var hist1h map[string]HourlyVolume
	if err := LoadJSON(prices1hPath, &hist1h); err != nil {
		return nil, fmt.Errorf("error loading 1h prices from %s: %w", prices1hPath, err)
	}

	var hist24h map[string]HourlyVolume
	if err := LoadJSON(prices24hPath, &hist24h); err != nil {
		return nil, fmt.Errorf("error loading 24h prices from %s: %w", prices24hPath, err)
	}

	var hist30d map[string]HourlyVolume
	if err := LoadJSON(prices30dPath, &hist30d); err != nil {
		return nil, fmt.Errorf("error loading 30d prices from %s: %w", prices30dPath, err)
	}

	var rolling24 []map[string]HourlyVolume
	if err := LoadJSON(pricesRollingPath, &rolling24); err != nil {
		return nil, fmt.Errorf("error loading rolling 24 prices from %s: %w", pricesRollingPath, err)
	}

	// 5. Load historical nudges (calculated beforehand and passed in)
	nudges := CalculateNudges(config, flips, failedSells)

	// 6. Run analysis
	reportItems := AnalyzePrices(prices, volumes, metadata, nudges, hist1h, hist24h, hist30d, vol5m, vol24h, filterName, config, rolling24)

	// 7. Save reports
	_, err = SaveJSON("reports", "report", runTs, reportItems)
	if err != nil {
		return nil, fmt.Errorf("error saving JSON report: %w", err)
	}

	mdReport := GenerateMarkdownReport(reportItems, runTs, capital, vol, limit)
	reportMDFile := fmt.Sprintf("reports/report_%d.md", runTs)
	if err := Store.WriteRaw(reportMDFile, []byte(mdReport)); err != nil {
		return nil, fmt.Errorf("error saving Markdown report: %w", err)
	}

	return reportItems, nil
}

func LoadJSON(path string, target interface{}) error {
	return Store.Read(path, target)
}

func CalculateNudges(config *RankingConfig, flips []FlipRecord, failedSells []FailedSellRecord) map[int]float64 {
	nudges := make(map[int]float64)
	netNudges := make(map[int]float64)

	now := time.Now()

	// 1. Process successful flips
	halfLifeFlips := time.Duration(config.FlipHalfLifeHours) * time.Hour
	lambdaFlips := 0.69314718056 / halfLifeFlips.Seconds()

	for _, record := range flips {
		itemID := record.ItemID
		age := now.Sub(record.Timestamp).Seconds()
		if age < 0 {
			age = 0
		}
		weight := math.Exp(-lambdaFlips * age)

		direction := 0.0
		switch record.Rating {
		case "Meh":
			direction = config.FlipModifierMeh
		case "Mid":
			direction = config.FlipModifierMid
		case "Good":
			direction = config.FlipModifierGood
		case "Great":
			direction = config.FlipModifierGreat
		}

		netNudges[itemID] += weight * direction
	}

	// 2. Process failed sells
	halfLifeFailed := time.Duration(config.FailedSellHalfLifeHours) * time.Hour
	lambdaFailed := 0.69314718056 / halfLifeFailed.Seconds()

	for _, record := range failedSells {
		itemID := record.ItemID
		age := now.Sub(record.Timestamp).Seconds()
		if age < 0 {
			age = 0
		}
		weight := math.Exp(-lambdaFailed * age)

		// Static heavy penalty per failed sell
		direction := config.FailedSellPenalty
		netNudges[itemID] += weight * direction
	}

	// Calculate Exponentially Decayed Sum
	for itemID, sum := range netNudges {

		multiplier := 1.0 + sum
		// Clamp multiplier between configured min and an absolute max of 3.0
		if multiplier < config.NudgeMin {
			multiplier = config.NudgeMin
		}
		if multiplier > 3.0 {
			multiplier = 3.0
		}
		nudges[itemID] = multiplier
	}

	return nudges
}

