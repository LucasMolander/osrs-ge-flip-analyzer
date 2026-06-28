package main

import (
	"context"
	"fmt"
	"time"

	"github.com/lucasmolander/osrs-ge-flip-analyzer/backend"
)

func main() {
	client := backend.NewClient("osrs-ge-flip-analyzer-test")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	fmt.Println("--- Testing /mapping ---")
	mapping, err := client.FetchItemMapping(ctx)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else if len(mapping) > 0 {
		fmt.Printf("Success! First item: %+v\n", mapping[0])
	}

	fmt.Println("\n--- Testing /latest ---")
	latest, err := client.FetchLatestPrices(ctx)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else if len(latest) > 0 {
		var firstKey string
		for k := range latest {
			firstKey = k
			break
		}
		fmt.Printf("Success! Sample item %s: %+v\n", firstKey, latest[firstKey])
	}

	fmt.Println("\n--- Testing /5m ---")
	ts, m5, err := client.Fetch5mVolumes(ctx)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else if len(m5) > 0 {
		var firstKey string
		for k := range m5 {
			firstKey = k
			break
		}
		fmt.Printf("Success! TS: %d, Sample item %s: %+v\n", ts, firstKey, m5[firstKey])
	}

	fmt.Println("\n--- Testing /1h ---")
	ts1, h1, err := client.FetchHourlyVolumes(ctx)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else if len(h1) > 0 {
		var firstKey string
		for k := range h1 {
			firstKey = k
			break
		}
		fmt.Printf("Success! TS: %d, Sample item %s: %+v\n", ts1, firstKey, h1[firstKey])
	}

	fmt.Println("\n--- Testing /latest (with ID) ---")
	latestID, err := client.FetchLatestPrice(ctx, 4151) // Abyssal whip
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Printf("Success! Abyssal whip (4151): %+v\n", latestID["4151"])
	}

	fmt.Println("\n--- Testing /timeseries (5m for ID 4151) ---")
	tsData, err := client.FetchTimeSeries(ctx, 4151, "5m")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else if len(tsData) > 0 {
		fmt.Printf("Success! Total points: %d. Latest data point: %+v\n", len(tsData), tsData[len(tsData)-1])
	}
}
