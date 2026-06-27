# OSRS GE Flip Analyzer

A robust, data-driven Old School RuneScape Grand Exchange flipping analyzer. It pulls real-time market data from the OSRS Wiki API and mathematically ranks items to find the most profitable arbitrage opportunities (flips).

This repository is structured into distinct modules to separate the business logic, the web interface, and the deployment infrastructure.

## Directory Structure

- **`cmd/ge-analyzer/`**: Contains the `main.go` entry point and CLI commands. It handles booting up the application in either local-analysis mode or web-server mode.
- **`core/`**: The heart of the application. Contains the ranking algorithms, mathematical penalty curves, external API clients, and the data structs used across the app.
- **`web/`**: The backend HTTP server. Handles API routing, payload parsing, and authentication middleware.
  - **`web/frontend/`**: The Vue.js single-page application and static assets that power the visual dashboard.
- **`gcp/`**: Infrastructure as Code (Terraform) and bash scripts for deploying the application to Google Cloud Run.
- **`reports/`**: (Generated) Output directory where the application dumps `.json` and `.md` market reports when run in offline analysis mode.
- **`prices/` & `item_data/`**: (Generated) Caching directories for the local disk-based persistence store.
- **`flips/`**: (Generated) Local directory containing saved flip histories in JSON format.
- **`deploy/`**: Directory for Docker deployment files, primarily used for building the container image.
- **`reference/`**: Static reference data and scripts utilized by the pipeline to map item IDs to names and properties.
- **`config.json`**: The global configuration file establishing default penalty curves, parameters, and ranking weights.
- **`update_index.py`**: A Python script to pull new item mappings and metadata from the OSRS Wiki into the reference directory.
- **`scratch_app.js`**: Scratchpad for UI/Vue.js changes and testing.
- **`ge-analyzer`**: (Generated) The compiled Go binary executable.
- **`go.mod` & `go.sum`**: Go module definitions and dependency lock files.
- **`.gitignore`**: Global rules for files and folders to ignore in version control.
- **`LICENSE`**: The open-source license terms for this repository.
- **`out.log`**: (Generated) Output log from background processes or the web server.
