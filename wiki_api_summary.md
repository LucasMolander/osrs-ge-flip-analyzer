# OSRS Prices Wiki API Summary

The OSRS Prices Wiki API provides free, unauthenticated access to real-time and historical Grand Exchange pricing data. 

Here is a summary of the data and endpoints you now have access to via the `core.OSRSClient` library:

### 1. Item Mapping (`/mapping`)
**Client Method:** `FetchItemMapping(ctx)`
**Purpose:** Provides static definitions and metadata for all tradeable items.
**Data Returned:**
- `id` (int): The canonical item ID.
- `name` (string): The in-game name of the item.
- `examine` (string): The examine text.
- `members` (bool): Whether it is a members-only item.
- `lowalch` / `highalch` (int): Alchemy values.
- `limit` (int): The 4-hour GE buying limit.
- `value` (int): The base GE value of the item.
- `icon` (string): The filename of the item's icon image on the wiki.

### 2. Latest Prices (`/latest`)
**Client Method:** `FetchLatestPrices(ctx)` and `FetchLatestPrice(ctx, id)`
**Purpose:** Returns the absolute most recent instant-buy and instant-sell transactions recorded by RuneLite users.
**Data Returned:**
- `high` (*int64): The most recent instant-buy price (can be null if never bought).
- `highTime` (*int64): Unix timestamp of when this transaction occurred.
- `low` (*int64): The most recent instant-sell price (can be null if never sold).
- `lowTime` (*int64): Unix timestamp of when this transaction occurred.
*Note: You can pass a specific item ID to only fetch one item, saving bandwidth.*

### 3. Five-Minute Volumes (`/5m`)
**Client Method:** `Fetch5mVolumes(ctx)` and `FetchHistorical5m(ctx, timestamp)`
**Purpose:** Returns 5-minute averages of high/low prices and the exact trade volume within that 5-minute block.
**Data Returned:**
- `avgHighPrice` (*int64): Volume-weighted average high price during the 5 minutes.
- `highPriceVolume` (int64): Total quantity of the item instant-bought.
- `avgLowPrice` (*int64): Volume-weighted average low price during the 5 minutes.
- `lowPriceVolume` (int64): Total quantity of the item instant-sold.

### 4. One-Hour Volumes (`/1h`)
**Client Method:** `FetchHourlyVolumes(ctx)` and `FetchHistoricalPrices(ctx, timestamp)`
**Purpose:** Functions identically to `/5m`, but averages the prices and aggregates the trade volume over a 1-hour block.

### 5. Time-Series Data (`/timeseries`)
**Client Method:** `FetchTimeSeries(ctx, id, timestep)`
**Purpose:** Returns a historical chronological list of high/low prices and volumes for a *single specific item*.
**Data Returned:**
- Returns an array of up to 365 data points.
- Each data point contains: `timestamp`, `avgHighPrice`, `avgLowPrice`, `highPriceVolume`, and `lowPriceVolume`.
- `timestep` options: `"5m"`, `"1h"`, `"6h"`, `"24h"`. Using larger timesteps returns data going further back in time (e.g. 365 days for "24h").

---

### Implementation Notes
- I have added the missing `TimeSeriesDataPoint` and `TimeSeriesResponse` structs to `core/types.go`.
- I have added `FetchLatestPrice` and `FetchTimeSeries` methods to `core/client.go`.
- A successful unit test script was run locally to verify the exact schema structure of each response against live OSRS Wiki data.
