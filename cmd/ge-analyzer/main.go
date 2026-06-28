package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"runtime"
	"time"

	"cloud.google.com/go/profiler"

	"github.com/lucasmolander/osrs-ge-flip-analyzer/backend"
	"github.com/lucasmolander/osrs-ge-flip-analyzer/core"
	"github.com/lucasmolander/osrs-ge-flip-analyzer/web"
)

func debugLog(msg string) {
	fmt.Println(msg)
	os.Stdout.Sync()
}

func main() {
	runtime.SetBlockProfileRate(1)
	runtime.SetMutexProfileFraction(1)

	debugLog("ge-analyzer-server starting...")
	isServer := os.Getenv("PORT") != "" || (len(os.Args) >= 2 && os.Args[1] == "serve")

	debugLog("Initializing storage...")
	var err error

	// Fast timeout for debugging
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = ctx

	debugLog("Calling NewStorage...")
	backend.Store, err = backend.NewStorage(isServer)
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}
	debugLog("Storage initialized.")

	debugLog("Loading config...")
	config, err := core.LoadConfig("config.json")
	if err != nil {
		debugLog(fmt.Sprintf("Warning: failed to load config.json (%v), using defaults", err))
		config = core.DefaultRankingConfig()
	}
	debugLog("Config loaded.")

	port := os.Getenv("PORT")
	if port != "" {
		debugLog(fmt.Sprintf("Detected PORT=%s, starting server...", port))

		// Initialize Google Cloud Profiler
		cfg := profiler.Config{
			Service:        "ge-analyzer-service",
			ServiceVersion: "1.0.0",
			DebugLogging:   true,
			ProjectID:      os.Getenv("GOOGLE_CLOUD_PROJECT"),
		}
		if err := profiler.Start(cfg); err != nil {
			log.Printf("Warning: failed to start Google Cloud Profiler: %v", err)
		} else {
			debugLog("Google Cloud Profiler started successfully.")
		}

		client := backend.NewClient("")
		err := web.StartServer(port, client, 20000000, 10, 50, backend.Store, config)
		if err != nil {
			log.Fatalf("Server failed to start: %v", err)
		}
		return
	}

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
	case "record-failed-sell":
		handleRecordFailedSellCommand()
	case "backup":
		handleBackupCommand()
	case "restore":
		handleRestoreCommand()
	case "serve":
		handleServeCommand()
	case "help", "-h", "--help":
		printGeneralUsage()
	default:
		fmt.Printf("Error: unknown command '%s'\n\n", subcommand)
		printGeneralUsage()
		os.Exit(1)
	}
}
