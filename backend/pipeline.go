package backend

import (
	"context"
	"fmt"
	"runtime/pprof"
	"sync"
	"time"

	"github.com/lucasmolander/osrs-ge-flip-analyzer/core"
)

func SaveJSON(dir, prefix string, timestamp int64, data interface{}) (string, error) {
	t := time.Unix(timestamp, 0).UTC()
	// Structure: dir/yyyy/mm/dd/prefix_timestamp.json
	path := fmt.Sprintf("%s/%04d/%02d/%02d/%s_%d.json", dir, t.Year(), int(t.Month()), t.Day(), prefix, timestamp)
	return Store.Write(path, data)
}

func DownloadPrices(ctx context.Context, client *OSRSClient, timestamp int64) (bool, error) {
	defer core.GlobalProfiler.Stop("DownloadPrices", core.GlobalProfiler.Start("DownloadPrices"))

	// 0. Check if we already have recent data
	_, latestTs, err := Store.FindLatestFile("prices", "prices")
	if err == nil && (timestamp-latestTs) < 60 {
		fmt.Printf("Data from the past minute (%d) already exists. Skipping download.\n", latestTs)
		return false, nil
	}

	prices, err := client.FetchLatestPrices(ctx)
	if err != nil {
		return false, fmt.Errorf("fetching latest prices: %w", err)
	}
	_, err = SaveJSON("prices", "prices", timestamp, prices)
	if err != nil {
		return false, fmt.Errorf("saving prices: %w", err)
	}

	_, volumes, err := client.FetchHourlyVolumes(ctx)
	if err != nil {
		return false, fmt.Errorf("fetching hourly volumes: %w", err)
	}
	_, err = SaveJSON("prices", "volumes", timestamp, volumes)
	if err != nil {
		return false, fmt.Errorf("saving volumes: %w", err)
	}

	_, volumes5m, err := client.Fetch5mVolumes(ctx)
	if err != nil {
		return false, fmt.Errorf("fetching 5m volumes: %w", err)
	}
	_, err = SaveJSON("prices", "volumes_5m", timestamp, volumes5m)
	if err != nil {
		return false, fmt.Errorf("saving 5m volumes: %w", err)
	}

	_, volumes24hAvg, err := client.Fetch24hVolumes(ctx)
	if err != nil {
		return false, fmt.Errorf("fetching 24h volumes avg: %w", err)
	}
	_, err = SaveJSON("prices", "volumes_24h_avg", timestamp, volumes24hAvg)
	if err != nil {
		return false, fmt.Errorf("saving 24h volumes avg: %w", err)
	}

	// Align timestamp to the hourly boundary (multiples of 3600)
	alignedNow := (timestamp / 3600) * 3600

	// 1. Fetch 1 hour ago
	t1h := alignedNow - 3600
	hist1h, err := client.FetchHistoricalPrices(ctx, t1h)
	if err != nil {
		return false, fmt.Errorf("fetching historical prices (1h ago): %w", err)
	}
	_, err = SaveJSON("prices", "prices_1h", timestamp, hist1h)
	if err != nil {
		return false, fmt.Errorf("saving 1h prices: %w", err)
	}

	// 2. Fetch 24 hours ago
	t24h := alignedNow - 86400
	hist24h, err := client.FetchHistoricalPrices(ctx, t24h)
	if err != nil {
		return false, fmt.Errorf("fetching historical prices (24h ago): %w", err)
	}
	_, err = SaveJSON("prices", "prices_24h", timestamp, hist24h)
	if err != nil {
		return false, fmt.Errorf("saving 24h prices: %w", err)
	}

	// 3. Fetch 30 days ago (for 30d volume spike analysis)
	t30d := alignedNow - (30 * 86400)
	hist30d, err := client.FetchHistoricalPrices(ctx, t30d)
	if err != nil {
		return false, fmt.Errorf("fetching historical prices (30d ago): %w", err)
	}
	_, err = SaveJSON("prices", "prices_30d", timestamp, hist30d)
	if err != nil {
		return false, fmt.Errorf("saving 30d prices: %w", err)
	}

	// 4. Fetch 24 continuous 5m ticks for Outlier Math
	aligned5m := (timestamp / 300) * 300
	rolling24 := make([]map[string]core.HourlyVolume, 24)
	var wg sync.WaitGroup
	var errMu sync.Mutex
	var rollingErr error

	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			// i=0 means 5 minutes ago, i=23 means 120 minutes ago (we exclude the current tick)
			tTick := aligned5m - int64((idx+1)*300)
			tickData, err := client.FetchHistorical5m(ctx, tTick)
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
		return false, fmt.Errorf("fetching rolling 5m ticks: %w", rollingErr)
	}

	_, err = SaveJSON("prices", "prices_rolling_24", timestamp, rolling24)
	if err != nil {
		return false, fmt.Errorf("saving rolling 24 ticks: %w", err)
	}

	return true, nil
}

func DownloadMetadata(ctx context.Context, client *OSRSClient, timestamp int64) (map[int]core.ItemMetadata, string, error) {
	defer core.GlobalProfiler.Stop("DownloadMetadata", core.GlobalProfiler.Start("DownloadMetadata"))
	items, err := client.FetchItemMapping(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("fetching item mapping: %w", err)
	}

	// Build a map of ID -> core.ItemMetadata as requested
	metadataMap := make(map[int]core.ItemMetadata)
	for _, item := range items {
		metadataMap[item.ID] = item
	}

	metadataFile, err := SaveJSON("item_data", "item_metadata", timestamp, metadataMap)
	if err != nil {
		return nil, "", fmt.Errorf("saving item metadata: %w", err)
	}
	return metadataMap, metadataFile, nil
}

func loadMarketState(ctx context.Context, client *OSRSClient, runTs int64) (*core.MarketState, error) {
	// Locate latest price and volume files
	tFind := core.GlobalProfiler.Start("Storage_FindLatestFile")
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
	core.GlobalProfiler.Stop("Storage_FindLatestFile", tFind)

	// Locate or fetch latest metadata file
	tLoad := core.GlobalProfiler.Start("Storage_LoadJSONs")
	var metadata map[int]core.ItemMetadata
	metadataPath, _, err := Store.FindLatestFile("item_data", "item_metadata")
	if err != nil {
		fmt.Println("No cached item metadata found. Fetching it now...")
		metadata, metadataPath, err = DownloadMetadata(ctx, client, runTs)
		if err != nil {
			return nil, fmt.Errorf("error fetching item metadata: %w", err)
		}
		fmt.Printf("Metadata cached at: %s\n", metadataPath)
	} else {
		metadata = make(map[int]core.ItemMetadata)
		if err := LoadJSON(metadataPath, &metadata); err != nil {
			return nil, fmt.Errorf("error loading item metadata from %s: %w", metadataPath, err)
		}
	}

	// Load prices and volumes
	var prices map[string]core.LatestPrice
	if err := LoadJSON(pricesPath, &prices); err != nil {
		return nil, fmt.Errorf("error loading prices from %s: %w", pricesPath, err)
	}

	var volumes map[string]core.HourlyVolume
	if err := LoadJSON(volumesPath, &volumes); err != nil {
		return nil, fmt.Errorf("error loading volumes from %s: %w", volumesPath, err)
	}

	var vol5m map[string]core.HourlyVolume
	if err := LoadJSON(volumes5mPath, &vol5m); err != nil {
		return nil, fmt.Errorf("error loading 5m volumes from %s: %w", volumes5mPath, err)
	}

	var vol24h map[string]core.HourlyVolume
	if err := LoadJSON(volumes24hAvgPath, &vol24h); err != nil {
		return nil, fmt.Errorf("error loading 24h volumes from %s: %w", volumes24hAvgPath, err)
	}

	var hist1h map[string]core.HourlyVolume
	if err := LoadJSON(prices1hPath, &hist1h); err != nil {
		return nil, fmt.Errorf("error loading 1h prices from %s: %w", prices1hPath, err)
	}

	var hist24h map[string]core.HourlyVolume
	if err := LoadJSON(prices24hPath, &hist24h); err != nil {
		return nil, fmt.Errorf("error loading 24h prices from %s: %w", prices24hPath, err)
	}

	var hist30d map[string]core.HourlyVolume
	if err := LoadJSON(prices30dPath, &hist30d); err != nil {
		return nil, fmt.Errorf("error loading 30d prices from %s: %w", prices30dPath, err)
	}

	var rolling24 []map[string]core.HourlyVolume
	if err := LoadJSON(pricesRollingPath, &rolling24); err != nil {
		return nil, fmt.Errorf("error loading rolling 24 prices from %s: %w", pricesRollingPath, err)
	}
	core.GlobalProfiler.Stop("Storage_LoadJSONs", tLoad)

	return &core.MarketState{
		Timestamp: runTs,
		Prices:    prices,
		Volumes:   volumes,
		Vol5m:     vol5m,
		Vol24h:    vol24h,
		Hist1h:    hist1h,
		Hist24h:   hist24h,
		Hist30d:   hist30d,
		Rolling24: rolling24,
		Metadata:  metadata,
	}, nil
}

// GenerateMarketState downloads prices (if forced), loads all data into a core.MarketState, and writes it as a gzipped JSON to GCS.
func GenerateMarketState(ctx context.Context, client *OSRSClient, forceDownload bool) error {
	defer core.GlobalProfiler.Stop("GenerateMarketState", core.GlobalProfiler.Start("GenerateMarketState"))
	runTs := time.Now().Unix()

	if forceDownload {
		fmt.Println("Downloading latest price and volume data...")
		if _, err := DownloadPrices(ctx, client, runTs); err != nil {
			return fmt.Errorf("error downloading prices: %w", err)
		}
	}

	state, err := loadMarketState(ctx, client, runTs)
	if err != nil {
		return err
	}

	// Write market_state_latest.json
	// This uses WriteGzip to upload with Content-Encoding: gzip.
	path := "market_state_latest.json"
	_, err = Store.WriteGzip(path, state)
	if err != nil {
		return fmt.Errorf("error saving compressed market state JSON: %w", err)
	}
	fmt.Printf("Successfully generated and saved %s\n", path)
	return nil
}

// RunAnalysis orchestrates the fetching of prices, parsing, scoring, and report generation.
func RunAnalysis(ctx context.Context, client *OSRSClient, capital, vol int64, limit int, forceDownload bool, filterName string, config *core.RankingConfig, flips []core.FlipRecord, failedSells []core.FailedSellRecord) ([]core.ReportItem, error) {
	defer core.GlobalProfiler.Stop("RunAnalysis", core.GlobalProfiler.Start("RunAnalysis"))
	runTs := time.Now().Unix()

	// 1. Download unless skipped
	if forceDownload {
		fmt.Println("Downloading latest price and volume data...")
		if _, err := DownloadPrices(ctx, client, runTs); err != nil {
			return nil, fmt.Errorf("error downloading prices: %w", err)
		}
	} else {
		_, _, err1 := Store.FindLatestFile("prices", "prices")
		_, _, err2 := Store.FindLatestFile("prices", "prices_rolling_24")
		if err1 != nil || err2 != nil {
			fmt.Println("Missing required cached prices (like rolling_24). Fetching them now as fallback...")
			if _, err := DownloadPrices(ctx, client, runTs); err != nil {
				return nil, fmt.Errorf("error downloading prices during fallback: %w", err)
			}
		}
	}

	state, err := loadMarketState(ctx, client, runTs)
	if err != nil {
		return nil, err
	}

	// 5. Load historical nudges (calculated beforehand and passed in)
	nudges := core.CalculateNudges(ctx, config, flips, failedSells)

	// 6. Run analysis
	tAnalyze := core.GlobalProfiler.Start("core.AnalyzePrices")
	var reportItems []core.ReportItem
	pprof.Do(ctx, pprof.Labels("phase", "RunAnalysis", "subphase", "core.AnalyzePrices"), func(ctx context.Context) {
		reportItems = core.AnalyzePrices(
			ctx,
			runTs,
			state.Prices,
			state.Volumes, state.Metadata, nudges, state.Hist1h, state.Hist24h, state.Hist30d, state.Vol5m, state.Vol24h, filterName, config, state.Rolling24)
	})
	core.GlobalProfiler.Stop("core.AnalyzePrices", tAnalyze)

	// 7. Save reports
	_, err = SaveJSON("reports", "report", runTs, reportItems)
	if err != nil {
		return nil, fmt.Errorf("error saving JSON report: %w", err)
	}

	mdReport := core.GenerateMarkdownReport(reportItems, runTs, capital, vol, limit)
	reportMDFile := fmt.Sprintf("reports/report_%d.md", runTs)
	if err := Store.WriteRaw(reportMDFile, []byte(mdReport)); err != nil {
		return nil, fmt.Errorf("error saving Markdown report: %w", err)
	}

	return reportItems, nil
}

func LoadJSON(path string, target interface{}) error {
	return Store.Read(path, target)
}
