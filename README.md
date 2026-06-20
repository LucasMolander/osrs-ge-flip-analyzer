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
