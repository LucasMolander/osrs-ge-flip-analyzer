# `cmd/ge-analyzer`

This directory contains the main entry points for the application. It provides the command-line interface (CLI) to run the analysis, start the web server, and manage local data.

## Files

- **`cli.go`**: Handles argument parsing and the execution of specific CLI commands (e.g. `analyze`, `serve`, `backup`, `restore`). It acts as the controller for invoking the core analysis pipelines and printing results to standard output.
- **`main.go`**: The root execution point of the Go binary. It initializes the file-based persistence store and delegates execution to the CLI parser in `cli.go`.
