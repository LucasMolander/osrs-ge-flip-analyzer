# `web/frontend`

This directory houses the static client-side web application (HTML, CSS, JS) that serves as the visual dashboard for the analyzer.

## Architecture

This frontend is a single-page application powered by **Vue 3** (via CDN). It is entirely stateless concerning the backend server: all user configuration and historical trade data are stored persistently in the browser's `localStorage` and sent over to the backend on-the-fly for analysis.

## Subdirectories & Files

- **`index.html`**: The main DOM structure and entry point. It contains the login screen, layout grids, modal overlays, and the data table.
- **`css/style.css`**: The stylesheet. It uses modern, dark-mode CSS variables, glassmorphism UI elements, and handles the responsive layout and table spacing.
- **`js/app.js`**: The Vue application logic. It manages state (e.g. `items`, `sortConfig`, `flipsHistory`), handles asynchronous fetching to the backend `/api/report` endpoint, and persists user overrides into `localStorage`. It also handles logic for exporting and importing `.json` profile backups.
