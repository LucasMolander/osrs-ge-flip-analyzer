package main

import (
	"fmt"
	"log"
	"os"
	"github.com/lucasmolander/osrs-ge-flip-analyzer/core"
	"github.com/lucasmolander/osrs-ge-flip-analyzer/web"
)

func main() {
	isServer := os.Getenv("PORT") != "" || (len(os.Args) >= 2 && os.Args[1] == "serve")

	var err error
	core.Store, err = core.NewStorage(isServer)
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}

	// Load config
	config, err := core.LoadConfig("config.json")
	if err != nil {
		log.Printf("Warning: failed to load config.json (%v), using defaults", err)
		config = core.DefaultRankingConfig()
	}

	// Check if PORT env var is set (standard for cloud environments like Cloud Run)
	port := os.Getenv("PORT")
	if port != "" {
		client := core.NewClient("")
		err := web.StartServer(port, client, 20000000, 10, 50, core.Store, config)
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

