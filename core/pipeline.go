package core

import (
	"fmt"
	"time"
	"math"
)

func SaveJSON(dir, prefix string, timestamp int64, data interface{}) (string, error) {
	path := fmt.Sprintf("%s/%s_%d.json", dir, prefix, timestamp)
	return Store.Write(path, data)
}

func DownloadPrices(client *OSRSClient, timestamp int64) error {
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
func RunAnalysis(client *OSRSClient, capital, vol int64, limit int, forceDownload bool, filterName string, config *RankingConfig) ([]ReportItem, error) {
	runTs := time.Now().Unix()

	// 1. Download unless skipped
	if forceDownload {
		fmt.Println("Downloading latest price and volume data...")
		if err := DownloadPrices(client, runTs); err != nil {
			return nil, fmt.Errorf("error downloading prices: %w", err)
		}
	} else {
		// Verify we at least have one cached file, if not, fetch anyway to prevent crashing
		_, _, err := Store.FindLatestFile("prices", "prices")
		if err != nil {
			fmt.Println("No cached prices found. Fetching them now as fallback...")
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

	// 5. Load historical nudges
	nudges, err := LoadNudges(config)
	if err != nil {
		return nil, fmt.Errorf("error loading historical nudges: %w", err)
	}

	// 6. Run analysis
	fmt.Println("Analyzing prices and generating report...")
	reportItems := AnalyzePrices(prices, volumes, metadata, capital, vol, nudges, hist1h, hist24h, hist30d, vol5m, vol24h, filterName, config)

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

func LoadNudges(config *RankingConfig) (map[int]float64, error) {
	nudges := make(map[int]float64)
	netNudges := make(map[int]float64)

	now := time.Now()

	// 1. Process successful flips
	flipEntries, err := Store.ListDir("flips")
	if err == nil {
		halfLifeFlips := time.Duration(config.FlipHalfLifeHours) * time.Hour
		lambdaFlips := 0.69314718056 / halfLifeFlips.Seconds()

		for _, name := range flipEntries {
			if len(name) > 5 && name[:5] == "flip_" && name[len(name)-5:] == ".json" {
				var itemID int
				var ts int64
				_, err := fmt.Sscanf(name[5:len(name)-5], "%d_%d", &itemID, &ts)
				if err != nil {
					continue
				}

				path := fmt.Sprintf("flips/%s", name)
				var record FlipRecord
				if err := LoadJSON(path, &record); err != nil {
					continue
				}

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
		}
	}

	// 2. Process failed buys
	failedEntries, err := Store.ListDir("failed_buys")
	if err == nil {
		halfLifeFailed := time.Duration(config.FailedBuyHalfLifeHours) * time.Hour
		lambdaFailed := 0.69314718056 / halfLifeFailed.Seconds()

		for _, name := range failedEntries {
			if len(name) > 11 && name[:11] == "failed_buy_" && name[len(name)-5:] == ".json" {
				var itemID int
				var ts int64
				_, err := fmt.Sscanf(name[11:len(name)-5], "%d_%d", &itemID, &ts)
				if err != nil {
					continue
				}

				path := fmt.Sprintf("failed_buys/%s", name)
				var record FailedBuyRecord
				if err := LoadJSON(path, &record); err != nil {
					continue
				}

				age := now.Sub(record.Timestamp).Seconds()
				if age < 0 {
					age = 0
				}
				weight := math.Exp(-lambdaFailed * age)

				// Static heavy penalty per failed buy
				direction := config.FailedBuyPenalty
				netNudges[itemID] += weight * direction
			}
		}
	}

	// 3. Compile and clamp multipliers
	for itemID, netNudge := range netNudges {
		multiplier := 1.0 + netNudge
		// Clamp multiplier between configured min and max
		if multiplier < config.NudgeMin {
			multiplier = config.NudgeMin
		}
		if multiplier > config.NudgeMax {
			multiplier = config.NudgeMax
		}
		nudges[itemID] = multiplier
	}

	return nudges, nil
}

