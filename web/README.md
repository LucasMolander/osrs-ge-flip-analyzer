# `web`

This directory handles the backend HTTP server and routing logic for the API layer of the GE Flip Analyzer. It acts as the bridge between the `core` analysis pipelines and the frontend UI.

## Files

- **`server.go`**: Initializes the `http.ServeMux` and configures the routing for all API endpoints. It also implements basic authentication middleware and static file serving for the `frontend` sub-directory.
- **`handlers.go`**: Contains the core REST API endpoint handlers (e.g., handling `POST /api/report`). This handles parsing the frontend's JSON payloads (such as user-local configs and flip history) and triggering the `core.RunAnalysis` pipeline.
- **`handlers_extra.go`**: Contains auxiliary endpoint handlers for tasks like fetching the system default config, triggering metadata syncs, and manual data updates.

## Subdirectories
- **`frontend/`**: The static HTML, CSS, and JS files served by this backend to present the web application interface.
