package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/lucasmolander/osrs-ge-flip-analyzer/backend"
	"github.com/lucasmolander/osrs-ge-flip-analyzer/core"
	"github.com/lucasmolander/osrs-ge-flip-analyzer/web"
)

// formatCompact formats large numbers into human-readable strings like 1.5M, 2.3B.
func formatCompact(n int64) string {
	if n >= 1_000_000_000 {
		return fmt.Sprintf("%.1fB", float64(n)/1_000_000_000)
	}
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1_000 {
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	}
	return fmt.Sprintf("%d", n)
}

// formatCommas formats an int64 with comma separators.
func formatCommas(n int64) string {
	in := fmt.Sprintf("%d", n)
	numOfCommas := (len(in) - 1) / 3
	out := make([]byte, len(in)+numOfCommas)
	for i, j, k := len(in)-1, len(out)-1, 0; ; i, j = i-1, j-1 {
		out[j] = in[i]
		if i == 0 {
			break
		}
		if k++; k == 3 {
			j, k = j-1, 0
			out[j] = ','
		}
	}
	return string(out)
}

// formatPriceCompact formats price numbers with 3 decimals for values >= 100K.
func formatPriceCompact(n int64) string {
	absN := n
	sign := ""
	if n < 0 {
		absN = -n
		sign = "-"
	}
	if absN >= 1_000_000 {
		return fmt.Sprintf("%s%.3fM", sign, float64(absN)/1_000_000.0)
	}
	if absN >= 100_000 {
		return fmt.Sprintf("%s%.3fK", sign, float64(absN)/1_000.0)
	}
	return fmt.Sprintf("%s%s", sign, formatCommas(absN))
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
	fmt.Println("  record-failed-buy Log a failed buy order to penalize item rankings")
	fmt.Println("  backup          Serialize and export all persistent database files to a JSON file")
	fmt.Println("  restore         De-serialize a JSON backup file and restore the database")
	fmt.Println("  serve           Start the web server dashboard")
	fmt.Println("\nUse 'ge-analyzer <command> -help' for more information on a command.")
}

func handlePricesCommand() {
	cmd := flag.NewFlagSet("prices", flag.ExitOnError)
	ua := cmd.String("user-agent", "", "Custom User-Agent header for the OSRS Wiki API")
	cmd.Parse(os.Args[2:])

	client := backend.NewClient(*ua)
	timestamp := time.Now().Unix()

	fmt.Println("Fetching and saving latest prices and volumes...")
	if _, err := backend.DownloadPrices(context.Background(), client, timestamp); err != nil {
		log.Fatalf("Error downloading prices: %v", err)
	}
	fmt.Println("Prices and volumes downloaded successfully.")
}

func handleItemMetadataCommand() {
	cmd := flag.NewFlagSet("item-metadata", flag.ExitOnError)
	ua := cmd.String("user-agent", "", "Custom User-Agent header for the OSRS Wiki API")
	cmd.Parse(os.Args[2:])

	client := backend.NewClient(*ua)
	timestamp := time.Now().Unix()

	fmt.Println("Fetching and saving item metadata...")
	_, path, err := backend.DownloadMetadata(context.Background(), client, timestamp)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	fmt.Printf("Successfully saved item metadata map to: %s\n", path)
}

func handleReportCommand() {
	cmd := flag.NewFlagSet("report", flag.ExitOnError)
	skipDownload := cmd.Bool("skip-download", false, "Skip downloading new price data and use the latest local files")
	capital := cmd.Int64("capital", 20000000, "Reference capital K_cap in gp (used for capital penalty)")
	volThreshold := cmd.Int64("volume-threshold", 10, "Reference volume K_vol in trades/hour (used for volume penalty)")
	limit := cmd.Int("limit", 50, "Maximum number of top items to show in report")
	ua := cmd.String("user-agent", "", "Custom User-Agent header for the OSRS Wiki API")
	cmd.Parse(os.Args[2:])

	filterName := ""
	if cmd.NArg() > 0 {
		filterName = strings.ToLower(strings.Join(cmd.Args(), " "))
	}

	config, err := core.LoadConfig("config.json")
	if err != nil {
		log.Printf("Warning: failed to load config.json (%v), using defaults", err)
		config = core.DefaultRankingConfig()
	}

	client := backend.NewClient(*ua)
	reportItems, err := backend.RunAnalysis(context.Background(), client, *capital, *volThreshold, *limit, !*skipDownload, filterName, config, nil, nil)
	if err != nil {
		log.Fatalf("Analysis failed: %v", err)
	}

	_, ts, err := backend.Store.FindLatestFile("prices", "prices")
	if err != nil {
		log.Fatalf("Error locating latest prices file: %v", err)
	}

	fmt.Printf("\n=== Analysis Complete ===\n")
	fmt.Printf("JSON Report:     reports/report_%d.json\n", ts)
	fmt.Printf("Markdown Report: reports/report_%d.md\n\n", ts)

	// Print full table to stdout
	displayTable(reportItems, *limit)
}

func handleServeCommand() {
	cmd := flag.NewFlagSet("serve", flag.ExitOnError)
	port := cmd.String("port", "8080", "Port to run the web server on")
	capital := cmd.Int64("capital", 20000000, "Reference capital K_cap in gp (used for capital penalty)")
	volThreshold := cmd.Int64("volume-threshold", 10, "Reference volume K_vol in trades/hour (used for volume penalty)")
	limit := cmd.Int("limit", 50, "Maximum number of top items to show in report")
	ua := cmd.String("user-agent", "", "Custom User-Agent header for the OSRS Wiki API")
	cmd.Parse(os.Args[2:])

	config, err := core.LoadConfig("config.json")
	if err != nil {
		log.Printf("Warning: failed to load config.json (%v), using defaults", err)
		config = core.DefaultRankingConfig()
	}

	client := backend.NewClient(*ua)
	err = web.StartServer(*port, client, *capital, *volThreshold, *limit, backend.Store, config)
	if err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func handleRecordFlipCommand() {
	cmd := flag.NewFlagSet("record-flip", flag.ExitOnError)
	id := cmd.Int("id", 0, "OSRS Item ID (required)")
	name := cmd.String("name", "", "Item Name")
	rating := cmd.String("rating", "Good", "Rating: Meh, Mid, Good, Great (required)")
	note := cmd.String("note", "", "Optional note about the flip")
	timestamp := cmd.Int64("timestamp", 0, "Optional Unix timestamp (defaults to current time)")
	cmd.Parse(os.Args[2:])

	if *id <= 0 {
		fmt.Println("Error: --id must be greater than 0.")
		cmd.Usage()
		os.Exit(1)
	}

	validRatings := map[string]bool{"Meh": true, "Mid": true, "Good": true, "Great": true}
	if !validRatings[*rating] {
		fmt.Println("Error: --rating must be Meh, Mid, Good, or Great.")
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

	record := core.FlipRecord{
		ItemID:    *id,
		ItemName:  *name,
		Rating:    *rating,
		Timestamp: t,
		Notes:     *note,
	}

	prefix := fmt.Sprintf("flip_%d", *id)
	path, err := backend.SaveJSON("flips", prefix, ts, record)
	if err != nil {
		log.Fatalf("Error saving flip record: %v", err)
	}

	fmt.Printf("Successfully logged flip for item %d to: %s\n", *id, path)
}

func handleRecordFailedSellCommand() {
	cmd := flag.NewFlagSet("record-failed-buy", flag.ExitOnError)
	id := cmd.Int("id", 0, "OSRS Item ID (required)")
	name := cmd.String("name", "", "Optional item name")
	note := cmd.String("note", "", "Optional note about the failed buy")
	timestamp := cmd.Int64("timestamp", 0, "Optional Unix timestamp (defaults to current time)")
	cmd.Parse(os.Args[2:])

	if *id <= 0 {
		fmt.Println("Error: --id must be greater than 0.")
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

	record := core.FailedSellRecord{
		ItemID:    *id,
		ItemName:  *name,
		Timestamp: t,
		Notes:     *note,
	}

	prefix := fmt.Sprintf("failed_sell_%d", *id)
	path, err := backend.SaveJSON("failed_sells", prefix, ts, record)
	if err != nil {
		log.Fatalf("Error saving failed buy record: %v", err)
	}

	fmt.Printf("Successfully logged failed buy for item %d to: %s\n", *id, path)
}

func handleBackupCommand() {
	cmd := flag.NewFlagSet("backup", flag.ExitOnError)
	outputFile := cmd.String("output", "", "Path to save the JSON backup file (optional, defaults to backup_<timestamp>.json)")
	cmd.Parse(os.Args[2:])

	backupJSON, err := backend.BackupData()
	if err != nil {
		log.Fatalf("Error creating backup: %v", err)
	}

	outPath := *outputFile
	if outPath == "" {
		outPath = fmt.Sprintf("backup_%d.json", time.Now().Unix())
	}

	if err := os.WriteFile(outPath, backupJSON, 0644); err != nil {
		log.Fatalf("Error writing backup file %s: %v", outPath, err)
	}

	fmt.Printf("Successfully created backup and wrote to: %s\n", outPath)
}

func handleRestoreCommand() {
	cmd := flag.NewFlagSet("restore", flag.ExitOnError)
	inputFile := cmd.String("input", "", "Path to the JSON backup file to restore (required)")
	cmd.Parse(os.Args[2:])

	if *inputFile == "" {
		fmt.Println("Error: --input argument is required.")
		cmd.Usage()
		os.Exit(1)
	}

	backupJSON, err := os.ReadFile(*inputFile)
	if err != nil {
		log.Fatalf("Error reading backup file %s: %v", *inputFile, err)
	}

	fmt.Println("Restoring database from backup file...")
	if err := backend.RestoreData(backupJSON); err != nil {
		log.Fatalf("Error restoring backup: %v", err)
	}

	fmt.Println("Successfully restored all persistent files from backup.")
}

func displayTable(items []core.ReportItem, limit int) {
	if len(items) == 0 {
		fmt.Println("No profitable flips found.")
		return
	}
	displayLimit := limit
	if len(items) < displayLimit {
		displayLimit = len(items)
	}
	fmt.Printf("\nTop %d Recommended Flips:\n", displayLimit)
	fmt.Printf("%-4s %-30s %-10s %-10s %-9s %-11s %-15s %-18s %-6s %-7s %-8s %-10s\n",
		"Rank", "Item Name", "Score", "Pot.Profit", "Profit/ea", "Capital", "Raw Spread", "Adj Spread", "Limit", "ROI", "Vol (hr)", "Trend")

	for i := 0; i < displayLimit; i++ {
		item := items[i]
		name := item.Name
		if item.IsSink {
			name += " [SINK]"
		}
		if len(name) > 30 {
			name = name[:27] + "..."
		}

		rawSpreadStr := fmt.Sprintf("%s/%s", formatPriceCompact(item.Low), formatPriceCompact(item.High))
		adjSpreadStr := fmt.Sprintf("%s->%s", formatPriceCompact(item.LowMod), formatPriceCompact(item.HighMod))

		trendStr := ""
		if len(item.PriceTrendIndicators) > 0 {
			for idx, ind := range item.PriceTrendIndicators {
				if idx > 0 {
					trendStr += " "
				}
				trendStr += ind
			}
		}
		if len(item.VolumeSpikeIndicators) > 0 {
			for _, ind := range item.VolumeSpikeIndicators {
				if trendStr != "" {
					trendStr += " "
				}
				trendStr += "⚠️" + ind + "-Spike"
			}
		}

		roiStr := fmt.Sprintf("%.1f%%", item.ROI)

		fmt.Printf("%-4d %-30s %-10.1f %-10s %-9s %-11s %-15s %-18s %-6s %-7s %-8s %-10s\n",
			i+1,
			name,
			item.Score,
			formatCompact(item.PotentialProfit),
			formatCompact(item.ProfitPerItem),
			formatCompact(item.CapitalRequired),
			rawSpreadStr,
			adjSpreadStr,
			formatCompact(int64(item.BuyLimit)),
			roiStr,
			formatCompact(item.Volume),
			trendStr,
		)
	}
	fmt.Println()
}
