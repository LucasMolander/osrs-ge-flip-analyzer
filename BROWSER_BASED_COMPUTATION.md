# Browser-Based Computation Feasibility

You raised a fantastic point: **Could the browser just do the math?**

Currently, the server fetches the data, calculates the nudges/scores/modifiers, and sends the processed JSON back. But modern JavaScript engines are incredibly fast, and Vue 3 is perfectly capable of rendering large computed lists.

Here is an analysis of moving the heavy lifting to the client:

## The Pros

1. **Zero Server CPU Cost for Analysis**:
   Right now, every time a user refreshes the report, the Go server spends CPU cycles iterating through thousands of items, calculating Z-scores, standard deviations, and running piecewise ranking algorithms. If this happens on the client, the server essentially just acts as a dumb file-server, serving static JSON from GCS.

2. **Instant UI Feedback**:
   If the math lives in the browser, users can tweak the configuration sliders (like Base Capital or Tax Rate) and see the table update **instantly** without needing a round-trip network request to `/api/report`. This would make the "Settings" tab infinitely more powerful.

3. **Massive Cost Savings**:
   Because the server would only need to run a 60-second cron job to download and cache JSON from the wiki, we could downscale the Cloud Run instance to the absolute minimum specs (or even replace the API entirely with a direct GCS public bucket or CDN).

## The Cons

1. **Payload Size (Bandwidth vs CPU)**:
   Right now, the server only sends back the **Top 50** items. 
   If the browser does the math, the server must send the *entire dataset* to the client.
   - Latest Prices (4,000+ items)
   - Volume Data (4,000+ items)
   - 24-hour rolling ticks (24 ticks * 4,000 items)
   - 30-day historical data
   
   This could amount to a **5-10 MB JSON payload** on the initial page load. However, we are already downloading this from GCS to the server anyway. Doing it from Server -> Client might add 1-2 seconds of load time, but can be mitigated heavily with gzip/brotli compression.

2. **Code Duplication & Maintenance**:
   We would have to rewrite `core/analyzer.go` entirely in JavaScript/TypeScript inside `app.js`. While doable, it splits the logic. If we ever want a Discord bot or CLI tool to generate reports, we wouldn't be able to use the shared Go logic unless we compile it to WebAssembly (Wasm).

## The WebAssembly (Wasm) Middle Ground

Because the backend is written in Go, we don't actually have to rewrite the math in JavaScript!
Go supports compiling directly to WebAssembly (`GOOS=js GOARCH=wasm go build -o main.wasm`).

We could:
1. Compile the `AnalyzePrices` function into a `.wasm` binary.
2. Have the browser download `main.wasm`.
3. When the user tweaks settings, Vue calls the Wasm function locally in the browser to re-sort and re-score the items in milliseconds.

## Conclusion & Recommendation

Moving computation to the browser is absolutely the right architectural move if you want a highly interactive UI where sliders instantly update the table. 

**Recommendation:**
For Phase 2, we should implement a hybrid approach:
- Keep the server running the cron job to fetch/merge the raw data from the OSRS Wiki into a single compressed `market_snapshot.json.gz` file.
- When the user loads the web app, they download this snapshot.
- The Vue frontend runs the scoring algorithm locally (either rewritten in JS or via Wasm).
- Cloud Run CPU usage drops to effectively zero.
