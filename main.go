package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		printGeneralUsage()
		os.Exit(1)
	}

	subcommand := os.Args[1]
	switch subcommand {
	case "prices":
		handlePricesCommand()
	case "item-metadata":
		handleItemMetadataCommand()
	case "report":
		handleReportCommand()
	case "record-flip":
		handleRecordFlipCommand()
	case "help", "-h", "--help":
		printGeneralUsage()
	default:
		fmt.Printf("Error: unknown command '%s'\n\n", subcommand)
		printGeneralUsage()
		os.Exit(1)
	}
}

func printGeneralUsage() {
	fmt.Println("OSRS Grand Exchange Flip Analyzer")
	fmt.Println("Usage:")
	fmt.Println("  ge-analyzer <command> [arguments]")
	fmt.Println("\nCommands:")
	fmt.Println("  prices          Fetch and save latest prices and hourly trading volumes")
	fmt.Println("  item-metadata   Fetch and save static item metadata map (buy limits, etc.)")
	fmt.Println("  report          Generate a ranked list of the best flips")
	fmt.Println("  record-flip     Log a completed transaction to nudge future reports")
	fmt.Println("\nUse 'ge-analyzer <command> -help' for more information on a command.")
}

// saveJSON is a helper to marshal data and write it to a timestamped file.
func saveJSON(dir, prefix string, timestamp int64, data interface{}) (string, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory %s: %w", dir, err)
	}
	filename := fmt.Sprintf("%s/%s_%d.json", dir, prefix, timestamp)
	file, err := os.Create(filename)
	if err != nil {
		return "", fmt.Errorf("failed to create file %s: %w", filename, err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		return "", fmt.Errorf("failed to encode JSON to %s: %w", filename, err)
	}
	return filename, nil
}

// handlePricesCommand fetches and saves the latest prices and hourly trading volumes.
func handlePricesCommand() {
	cmd := flag.NewFlagSet("prices", flag.ExitOnError)
	ua := cmd.String("user-agent", "", "Custom User-Agent header for the OSRS Wiki API")
	cmd.Parse(os.Args[2:])

	client := NewClient(*ua)
	timestamp := time.Now().Unix()

	fmt.Println("Fetching and saving latest prices and volumes...")
	if err := downloadPrices(client, timestamp); err != nil {
		log.Fatalf("Error downloading prices: %v", err)
	}
	fmt.Println("Prices and volumes downloaded successfully.")
}

// downloadPrices fetches prices and volumes and saves them.
func downloadPrices(client *OSRSClient, timestamp int64) error {
	prices, err := client.FetchLatestPrices()
	if err != nil {
		return fmt.Errorf("fetching latest prices: %w", err)
	}
	_, err = saveJSON("prices", "prices", timestamp, prices)
	if err != nil {
		return fmt.Errorf("saving prices: %w", err)
	}

	_, volumes, err := client.FetchHourlyVolumes()
	if err != nil {
		return fmt.Errorf("fetching hourly volumes: %w", err)
	}
	_, err = saveJSON("prices", "volumes", timestamp, volumes)
	if err != nil {
		return fmt.Errorf("saving volumes: %w", err)
	}
	return nil
}

// handleItemMetadataCommand fetches item mappings and saves them as a map keyed by Item ID.
func handleItemMetadataCommand() {
	cmd := flag.NewFlagSet("item-metadata", flag.ExitOnError)
	ua := cmd.String("user-agent", "", "Custom User-Agent header for the OSRS Wiki API")
	cmd.Parse(os.Args[2:])

	client := NewClient(*ua)
	timestamp := time.Now().Unix()

	fmt.Println("Fetching and saving item metadata...")
	_, path, err := downloadMetadata(client, timestamp)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	fmt.Printf("Successfully saved item metadata map to: %s\n", path)
}

// downloadMetadata fetches mapping and saves it as a map.
func downloadMetadata(client *OSRSClient, timestamp int64) (map[int]ItemMetadata, string, error) {
	items, err := client.FetchItemMapping()
	if err != nil {
		return nil, "", fmt.Errorf("fetching item mapping: %w", err)
	}

	// Build a map of ID -> ItemMetadata as requested
	metadataMap := make(map[int]ItemMetadata)
	for _, item := range items {
		metadataMap[item.ID] = item
	}

	metadataFile, err := saveJSON("item_data", "item_metadata", timestamp, metadataMap)
	if err != nil {
		return nil, "", fmt.Errorf("saving item metadata: %w", err)
	}
	return metadataMap, metadataFile, nil
}

// handleReportCommand runs the analyzer and outputs a ranked list of flips.
func handleReportCommand() {
	cmd := flag.NewFlagSet("report", flag.ExitOnError)
	skipDownload := cmd.Bool("skip-download", false, "Skip downloading new price data and use the latest local files")
	capital := cmd.Int64("capital", 20000000, "Reference capital K_cap in gp (used for capital penalty)")
	volThreshold := cmd.Int64("volume-threshold", 10, "Reference volume K_vol in trades/hour (used for volume penalty)")
	limit := cmd.Int("limit", 50, "Maximum number of top items to show in report")
	ua := cmd.String("user-agent", "", "Custom User-Agent header for the OSRS Wiki API")
	cmd.Parse(os.Args[2:])

	client := NewClient(*ua)
	runTs := time.Now().Unix()

	// 1. Download unless skipped
	if !*skipDownload {
		fmt.Println("Downloading latest price and volume data...")
		if err := downloadPrices(client, runTs); err != nil {
			log.Fatalf("Error downloading prices: %v", err)
		}
	}

	// 2. Locate latest price and volume files
	pricesPath, _, err := FindLatestFile("prices", "prices")
	if err != nil {
		log.Fatalf("Error locating latest prices file: %v. Try running without -skip-download.", err)
	}
	volumesPath, _, err := FindLatestFile("prices", "volumes")
	if err != nil {
		log.Fatalf("Error locating latest volumes file: %v. Try running without -skip-download.", err)
	}

	// 3. Locate or fetch latest metadata file
	var metadata map[int]ItemMetadata
	metadataPath, _, err := FindLatestFile("item_data", "item_metadata")
	if err != nil {
		fmt.Println("No cached item metadata found. Fetching it now...")
		metadata, metadataPath, err = downloadMetadata(client, runTs)
		if err != nil {
			log.Fatalf("Error fetching item metadata: %v", err)
		}
		fmt.Printf("Metadata cached at: %s\n", metadataPath)
	} else {
		// Load from file
		metadata = make(map[int]ItemMetadata)
		if err := loadJSON(metadataPath, &metadata); err != nil {
			log.Fatalf("Error loading item metadata from %s: %v", metadataPath, err)
		}
	}

	// 4. Load prices and volumes
	var prices map[string]LatestPrice
	if err := loadJSON(pricesPath, &prices); err != nil {
		log.Fatalf("Error loading prices from %s: %v", pricesPath, err)
	}

	var volumes map[string]HourlyVolume
	if err := loadJSON(volumesPath, &volumes); err != nil {
		log.Fatalf("Error loading volumes from %s: %v", volumesPath, err)
	}

	// 5. Load historical nudges (Phase 5)
	nudges, err := loadNudges()
	if err != nil {
		log.Fatalf("Error loading historical nudges: %v", err)
	}

	// 6. Run analysis
	fmt.Println("Analyzing prices and generating report...")
	reportItems := AnalyzePrices(prices, volumes, metadata, *capital, *volThreshold, nudges)

	// 7. Save reports
	reportJSONFile, err := saveJSON("reports", "report", runTs, reportItems)
	if err != nil {
		log.Fatalf("Error saving JSON report: %v", err)
	}

	mdReport := GenerateMarkdownReport(reportItems, runTs, *capital, *volThreshold, *limit)
	reportMDFile := fmt.Sprintf("reports/report_%d.md", runTs)
	if err := os.WriteFile(reportMDFile, []byte(mdReport), 0644); err != nil {
		log.Fatalf("Error saving Markdown report: %v", err)
	}

	fmt.Printf("\n=== Analysis Complete ===\n")
	fmt.Printf("JSON Report:     %s\n", reportJSONFile)
	fmt.Printf("Markdown Report: %s\n\n", reportMDFile)

	// Print quick preview of top 10 recommended flips
	displayPreview(reportItems, 10)
}

func loadJSON(path string, target interface{}) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return json.NewDecoder(file).Decode(target)
}

func displayPreview(items []ReportItem, count int) {
	if len(items) == 0 {
		fmt.Println("No profitable flips found.")
		return
	}
	displayLimit := count
	if len(items) < displayLimit {
		displayLimit = len(items)
	}
	fmt.Printf("Top %d Recommended Flips Preview:\n", displayLimit)
	fmt.Printf("%-4s %-35s %-10s %-12s %-10s %-12s %-6s\n", "Rank", "Item Name", "Score", "Pot. Profit", "Profit/ea", "Capital Req", "ROI")
	for i := 0; i < displayLimit; i++ {
		item := items[i]
		name := item.Name
		if item.IsSink {
			name += " [SINK]"
		}
		if len(name) > 35 {
			name = name[:32] + "..."
		}
		fmt.Printf("%-4d %-35s %-10.1f %-12s %-10s %-12s %.2f%%\n",
			i+1,
			name,
			item.Score,
			formatCompact(item.PotentialProfit),
			formatCompact(item.ProfitPerItem),
			formatCompact(item.CapitalRequired),
			item.ROI,
		)
	}
}

// loadNudges loads historical flip nudges from local files.
func loadNudges() (map[int]float64, error) {
	nudges := make(map[int]float64)

	entries, err := os.ReadDir("flips")
	if err != nil {
		if os.IsNotExist(err) {
			return nudges, nil // Flips directory doesn't exist yet, return empty nudges
		}
		return nil, fmt.Errorf("reading flips directory: %w", err)
	}

	// Group flips by ItemID
	flipsByItem := make(map[int][]FlipRecord)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		// Match file name like "flip_123_1718815709.json"
		if len(name) > 5 && name[:5] == "flip_" && name[len(name)-5:] == ".json" {
			var itemID int
			var ts int64
			_, err := fmt.Sscanf(name[5:len(name)-5], "%d_%d", &itemID, &ts)
			if err != nil {
				continue // Skip malformed filenames
			}

			path := fmt.Sprintf("flips/%s", name)
			var record FlipRecord
			if err := loadJSON(path, &record); err != nil {
				continue // Skip unparseable files
			}
			flipsByItem[itemID] = append(flipsByItem[itemID], record)
		}
	}

	now := time.Now()
	halfLife := 7 * 24 * time.Hour // 7 days half-life
	lambda := 0.69314718056 / halfLife.Seconds()

	for itemID, records := range flipsByItem {
		netNudge := 0.0
		for _, record := range records {
			age := now.Sub(record.Timestamp).Seconds()
			if age < 0 {
				age = 0
			}
			// Exponential decay weight
			weight := math.Exp(-lambda * age)

			// Compute if it was a good or bad flip
			// GE tax on selling: 2%, capped at 5M
			tax := int64(0)
			if record.SellPrice >= 50 {
				tax = int64(float64(record.SellPrice) * 0.02)
				if tax > 5_000_000 {
					tax = 5_000_000
				}
			}
			revenuePerItem := record.SellPrice - tax
			profitPerItem := revenuePerItem - record.BuyPrice

			direction := 0.10 // Good flip: +10%
			if profitPerItem <= 0 {
				direction = -0.20 // Bad flip: -20%
			}

			netNudge += weight * direction
		}

		multiplier := 1.0 + netNudge
		// Clamp multiplier between 0.1 and 2.0
		if multiplier < 0.1 {
			multiplier = 0.1
		}
		if multiplier > 2.0 {
			multiplier = 2.0
		}
		nudges[itemID] = multiplier
	}

	return nudges, nil
}

// handleRecordFlipCommand logs a completed flip to be used in nudging future scores.
func handleRecordFlipCommand() {
	cmd := flag.NewFlagSet("record-flip", flag.ExitOnError)
	id := cmd.Int("id", 0, "OSRS Item ID (required)")
	qty := cmd.Int("qty", 0, "Quantity traded (required)")
	buy := cmd.Int64("buy", 0, "Buy price per item in gp (required)")
	sell := cmd.Int64("sell", 0, "Sell price per item in gp (required)")
	note := cmd.String("note", "", "Optional note about the flip")
	timestamp := cmd.Int64("time", 0, "Optional Unix timestamp (defaults to current time)")
	cmd.Parse(os.Args[2:])

	if *id <= 0 || *qty <= 0 || *buy <= 0 || *sell <= 0 {
		fmt.Println("Error: --id, --qty, --buy, and --sell must all be greater than 0.")
		cmd.Usage()
		os.Exit(1)
	}

	var t time.Time
	var ts int64
	if *timestamp > 0 {
		ts = *timestamp
		t = time.Unix(ts, 0)
	} else {
		t = time.Now()
		ts = t.Unix()
	}

	record := FlipRecord{
		ItemID:    *id,
		Quantity:  *qty,
		BuyPrice:  *buy,
		SellPrice: *sell,
		Timestamp: t,
		Notes:     *note,
	}

	prefix := fmt.Sprintf("flip_%d", *id)
	path, err := saveJSON("flips", prefix, ts, record)
	if err != nil {
		log.Fatalf("Error saving flip record: %v", err)
	}

	fmt.Printf("Successfully logged flip for item %d to: %s\n", *id, path)
}
