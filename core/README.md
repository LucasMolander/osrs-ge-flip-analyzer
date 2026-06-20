# `core`

This directory contains the central business logic and data processing algorithms for the OSRS GE Flip Analyzer. It is responsible for communicating with external APIs, managing local persistence, and evaluating the profitability and safety of market arbitrage opportunities.

## Files

- **`analyzer.go`**: Contains the core scoring algorithms (`AnalyzePrices`, `CalculateNudges`). It evaluates price spreads, trade volumes, historical trends, and user-local modifiers (flips and failed sells) to rank items by viability.
- **`analyzer_test.go`**: The unit testing suite for validating the math and logic of the core analyzer formulas (e.g. testing the Exponential Moving Average impact of recorded flips).
- **`backup.go`**: Handles the `BackupData` and `RestoreData` functions, serializing/deserializing the server's local `.json` storage files into ZIP archives or flat files for data portability.
- **`client.go`**: Implements the `WikiClient` used to interface directly with the Old School RuneScape Wiki API, fetching real-time 5-minute price ticks and 1-hour volume data.
- **`config.go`**: Defines the `RankingConfig` structure and loads default algorithm constants from `config.json`. These defaults can be overridden by the frontend.
- **`pipeline.go`**: The orchestration logic (`RunAnalysis`). It aggregates data from the Wiki API, feeds it into the scoring algorithm (`analyzer.go`), applies metadata filtering, and produces the final ranked report.
- **`ranking_math_explained.md`**: A detailed mathematical breakdown of the scoring algorithms and penalty curves implemented within the analyzer.
- **`storage.go`**: A local file-system wrapper (`FileStore`) that handles saving and retrieving timestamped JSON files for pricing data, reports, and metadata.
- **`types.go`**: Defines the foundational data models (`Item`, `PriceTick`, `ReportItem`, `ReportRequest`, etc.) used throughout the application.
