# OSRS GE Flip Analyzer: Scoring Formula Explained

The core philosophy of this analyzer is to transform raw GE prices and volumes into a single, comprehensive `Score` that dictates the recommendation order. The formula takes into account raw profit, required capital, volume constraints, historical item success, and market momentum.

Every variable listed below can be manually fine-tuned within the `config.json` file located in the root directory.

---

## 1. Base Profit, Absolute GP Scaling, & ROI
At the core of the score is the raw profit potential, calculated on the maximum amount of the item you can realistically afford to flip. Because absolute GP is vastly more valuable than percentage margins, the formula heavily rewards flips that yield large total GP, while applying a smaller secondary reward for capital efficiency (ROI).

```text
Profit_Per_Item = HighPrice - LowPrice - Tax

if Profit_Per_Item <= 0:
    Drop_Item() // Drop the item immediately, skip all math

// Cap the volume strictly to what your bankroll can afford
Affordable_Limit = Min(Item_Limit, Floor(base_capital / LowPrice))
Potential_Profit = Profit_Per_Item * Affordable_Limit

```

### 1a. Total Profit Multiplier (The Heavyweight Metric)

*Config Variables: `target_profit_benchmark` (default `400000`), `profit_reward_cap` (default `5.0`)*

Because `Potential_Profit` is already the base value of the Final Score, applying a multiplier based on it creates a geometric curve. To safely make this the most dominant factor, we use a piecewise curve:

* **Below the benchmark:** We apply a linear multiplier. Because this multiplies against the base profit, it creates a **quadratic** penalty that ruthlessly crushes low-profit items.
* **Above the benchmark:** We apply a **logarithmic** reward. This aggressively boosts high-profit flips without causing integer overflows or mathematically drowning out our risk penalties (like volume or crashing markets).

```text
Profit_Ratio = Potential_Profit / target_profit_benchmark

if Profit_Ratio < 1.0:
    // Linear penalty (Results in a quadratic final score drop-off)
    Profit_Multiplier = Profit_Ratio 
else:
    // Logarithmic reward (Base 2)
    Profit_Multiplier = 1.0 + Log2(Profit_Ratio)

// Clamp to prevent complete zero-outs or runaway scaling on extreme outliers
Profit_Multiplier = Max(0.01, Min(profit_reward_cap, Profit_Multiplier))

```

### 1b. ROI Scaling (The Secondary Metric)

*Config Variables: `target_roi` (default `0.02`), `roi_reward_cap` (default `2.0`)*

While raw GP is king, we still lightly reward capital-efficient flips and penalize dangerous low-margin flips using a strictly bounded ROI multiplier.

```text
Raw_ROI = Profit_Per_Item / LowPrice
ROI_Multiplier = Max(0.50, Min(roi_reward_cap, Raw_ROI / target_roi)) 

```

> **Note:** The OSRS Tax is exactly `0.02` (2%) and capped at `tax_cap` (default `5,000,000`). Because OSRS rounds tax down, items strictly `< 50 gp` are not taxed.

---

## 2. Volume Penalty Factors
*Config Variables: `volume_ratio_penalty_max`, `absolute_volume_penalty_max`*

Low liquidity items are inherently riskier because they take longer to buy and sell. There are two distinct volume penalties applied simultaneously:

### 2a. Volume Ratio Penalty
If an item's projected 4-hour volume is lower than its GE limit, its score is reduced quadratically. We assume a safe market capture rate of 5%.

```text
Projected_4h_Volume = Hourly_Volume * 4.0
Global_Ratio = Projected_4h_Volume / Item_Limit

if Global_Ratio <= volume_ratio_filter_threshold: 
    VolumeRatioFactor = 0.001 // Massive penalty to sink dead items
else:
    Penalty_Curve = Max(0, (1.0 - Global_Ratio) / (1.0 - volume_ratio_filter_threshold))
    VolumeRatioFactor = Max(0.01, 1.0 - (volume_ratio_penalty_max * Penalty_Curve^2))
```

> **Important:** If `Ratio` is <= `volume_ratio_filter_threshold` (default 10%), the item is **filtered out completely**. `volume_ratio_penalty_max` defaults to `1.30`.

### 2b. Absolute Volume Penalty
Regardless of the item limit, items with incredibly low absolute hourly volume (under 100 trades/hour) are penalized.

```text
Penalty_Curve = Max(0, (100.0 - Hourly_Volume) / 90.0)
AbsoluteVolumeFactor = Max(0.01, 1.0 - (absolute_volume_penalty_max * Penalty_Curve^2))
```

> **Important:** If `Hourly_Volume` is <= `min_absolute_volume` (default 10), the item is **filtered out completely**: `Drop_Item()`.

---

## 3. Historical Nudges
*Config Variables: `flip_half_life_hours`, `failed_sell_half_life_hours`, `flip_modifier_*`, `failed_sell_penalty`, `nudge_min`, `nudge_max`*

Every time you record a Manual Flip or a Failed Sell, it leaves a permanent, decaying "nudge" on that item's score multiplier. Nudges are calculated using an Exponential Moving Average (EMA) that decays over time based on their half-lives.

```text
Multiplier = 1.0 + Sum(Flip_Value * Exp(-Hours_Since_Flip / Half_Life))
```
- **Flips:** Added based on rating (`flip_modifier_meh` = `-0.10`, `flip_modifier_great` = `+0.20`, etc). Decays over `flip_half_life_hours`.
- **Failed Sells:** Subtracted heavily (`failed_sell_penalty` = `-0.40`). Represents an unprofitable dump when the margin collapses. Decays over `failed_sell_half_life_hours`.

> **Tip:** The final multiplier is clamped strictly between `nudge_min` (default `0.05`) and `nudge_max` (default `2.0`).

---

## 4. Market Momentum Modifiers
The analyzer detects crashing markets and temporary volatility spikes to adjust recommendations.

### 4a. Stale Price Penalty (Liquidity-Adjusted)
*Config Variables: `stale_base_grace_minutes`, `stale_eti_multiplier`, `stale_max_grace_minutes`, `stale_extreme_penalty_multiplier`*

If an item's High or Low price has not updated recently, it indicates a dead or highly illiquid market. However, low-volume items naturally trade less frequently. We dynamically calculate an acceptable "stale threshold" based on the item's Expected Trade Interval (ETI).

```text
// 1. Calculate Expected Trade Interval (ETI) using 24-hour volume for accuracy
Avg_Hourly_Vol = Max((Volume_24h / 24.0), 1.0)
ETI = 60.0 / Avg_Hourly_Vol

// 2. Determine the dynamic limit, capped at a maximum ceiling
Dynamic_Limit_Mins = Min(stale_max_grace_minutes, stale_base_grace_minutes + (ETI * stale_eti_multiplier))
Dynamic_Limit_Seconds = Dynamic_Limit_Mins * 60

// 3. Apply extreme penalty if either price is older than the dynamic limit
If (Current_Time - HighTime) > Dynamic_Limit_Seconds OR (Current_Time - LowTime) > Dynamic_Limit_Seconds:
    TrendMultiplier *= stale_extreme_penalty_multiplier

```

*(Tagged as `STALE(>Xm)` in the UI, where X is the item's dynamically evaluated limit).*

### 4b. Price Trend Modifiers
*Config Variables: `price_trend_penalty_1h`, `price_trend_penalty_24h`*

If the current **HighPrice** (sell ceiling) is strictly less than historical moving averages, the market is actively crashing, and penalties are chained:
```text
TrendMultiplier = 1.0
If High_Price < 1h_Avg:   TrendMultiplier *= price_trend_penalty_1h
If High_Price < 24h_Avg:  TrendMultiplier *= price_trend_penalty_24h
```
*Note: We only check the HighPrice to ensure we don't accidentally penalize market dips where the LowPrice drops temporarily but the ceiling holds steady.*

### 4c. Volume Spike Modifiers
*Config Variables: `volume_spike_5m_multiplier`, `volume_spike_24h_multiplier`*

If volume surges uncharacteristically fast, it indicates heavy market momentum (e.g. panic dumps or FOMO pumps), which are extremely lucrative for flippers:
```text
SpikeMultiplier = 1.0
If 5m_Vol > (Expected_5m_Vol * 3):  SpikeMultiplier *= volume_spike_5m_multiplier (Default 1.50)
If 1h_Vol > (Expected_1h_Vol * 3):  SpikeMultiplier *= volume_spike_24h_multiplier (Default 1.20)
```

### 4d. Price Outlier Penalties (Rolling Z-Scores)
*Config Variables: `outlier_z_score_threshold`, `outlier_penalty_multiplier`*

The system continuously analyzes the last 2 hours of 5-minute ticks (24 data points). It calculates the Mean and Standard Deviation, enforcing a minimum volatility floor (`Max(Mean * 0.01, 2.0)`) to prevent division-by-zero panics on perfectly stable items.

```text
Z_Score = |Current_Price - Mean| / StdDev
```

The penalty is **asymmetric** to prevent profit hallucination vs genuine market crashes:
1. **High Price Spikes (Hallucination Clamp):** If the High price spikes above the Z-score threshold, the algorithm literally clamps the evaluated `HighPrice` down to `Mean + (Threshold * StdDev)` *before* calculating potential profit.

2. **Low Price Crashes (Falling Knife):** If the Low price drops below the Z-score threshold, we check if the High price is *also* dropping below its mean. 
    - If yes (a true crash), an exponential decay penalty is applied to the TrendMultiplier: `Exp(-(Z_Score - Threshold) * outlier_penalty_multiplier)`.
    - If no (the high price is stable), no penalty is applied. The item retains its "Golden Margin".

### 4e. Sustained Volatility Penalties (High & Low Price Jitter)
*Config Variables: `high_jitter_threshold`, `high_jitter_penalty_multiplier`, `low_jitter_threshold`, `low_jitter_penalty_multiplier`*

This penalty replaces the old midpoint jitter. It detects items whose individual High or Low prices are erratically whipsawing tick-to-tick. Because absolute GP volatility means very different things depending on the item's margin, the jitter is measured **strictly relative to the item's 24-hour average spread**.

```text
// 1. Determine the baseline spread
Avg_Spread_24h = hist24h.High - hist24h.Low
Safe_Denom = Max(Avg_Spread_24h, 2.0) // Floor strictly to prevent division-by-zero panics

// 2. For every step (Older to Newer) over the past 12 ticks:
High_Step_Var = Min(|High_New - High_Old| / Safe_Denom, 1.0)
Low_Step_Var = Min(|Low_New - Low_Old| / Safe_Denom, 1.0)

// 3. Average the steps
Avg_High_Jitter = Sum(High_Step_Var) / 11
Avg_Low_Jitter = Sum(Low_Step_Var) / 11

// 4. Apply independent penalties
If Avg_High_Jitter > high_jitter_threshold:
    Excess = Avg_High_Jitter - high_jitter_threshold
    TrendMultiplier *= Exp(-(Excess * high_jitter_penalty_multiplier))

// Note: isGoldenMargin ensures we don't penalize a dropping Low price that creates a highly profitable wide margin
If Avg_Low_Jitter > low_jitter_threshold AND !isGoldenMargin:
    Excess = Avg_Low_Jitter - low_jitter_threshold
    TrendMultiplier *= Exp(-(Excess * low_jitter_penalty_multiplier))

```

### 4f. Spread Jitter & Spike Penalties

*Config Variables: `spread_jitter_rel_threshold`, `spread_jitter_abs_threshold`, `spread_jitter_penalty_multiplier`, `spread_spike_threshold`, `spread_spike_penalty_multiplier`*

A wildly fluctuating spread means the profit margin is highly unstable. We measure Spread Jitter using both a Relative component (percentage change) and an Absolute component (raw GP change). To avoid penalizing safe micro-fluctuations on cheap items, the penalty only triggers if BOTH thresholds are broken simultaneously.

**Spread Jitter:**

```text
Spread_Old = High_Old - Low_Old
Spread_New = High_New - Low_New

// Relative Step
Safe_Spread = Max(Spread_Old, 2.0)
Rel_Step = Min(|Spread_New - Spread_Old| / Safe_Spread, 1.0)

// Absolute Step (Raw GP)
Abs_Step = |Spread_New - Spread_Old|

Avg_Rel_Jitter = Sum(Rel_Step) / 11
Avg_Abs_Jitter = Sum(Abs_Step) / 11

// The penalty only triggers if BOTH the relative and absolute jitter thresholds are exceeded
if Avg_Rel_Jitter > spread_jitter_rel_threshold AND Avg_Abs_Jitter > spread_jitter_abs_threshold:
    Excess = Avg_Rel_Jitter - spread_jitter_rel_threshold
    TrendMultiplier *= Exp(-(Excess * spread_jitter_penalty_multiplier))

```

**Spread Spike:**
If the current, real-time spread is massively larger than the average spread of the last hour, this usually indicates an artificial price hike or a temporary dry-up of supply.

```text
Mean_Spread = Average of last 12 Spreads
Spike_Ratio = Current_Spread / Max(Mean_Spread, 1.0)

// Penalty only triggers if the High Price is pumping, to protect "Golden Margins"
if Spike_Ratio > spread_spike_threshold AND High_Price > Mean_High_Price AND (Current_Spread - Mean_Spread) >= 5.0 AND !isGoldenMargin:
    Excess = Spike_Ratio - spread_spike_threshold
    TrendMultiplier *= Exp(-(Excess * spread_spike_penalty_multiplier))

```

### 4g. Recent Worst Spread (Margin Reversion Penalty)
*Config Variables: `worst_spread_penalty_min_gap`*

To protect against temporary margin illusions (where the current spread is artificially wide but is likely to snap back to a highly competitive tight margin), we evaluate the current spread against the worst (tightest) actual spread seen over the last hour.

```text
// 1. Find the tightest margin in the last 12 ticks
Worst_Spread = Min(High_i - Low_i) for i in past 1h

// 2. Calculate the ratio of the worst spread to the current spread
Ratio = Worst_Spread / Max(Current_Spread, 1.0)

// 3. Apply a quadratic penalty if the margin has widened unnaturally
// Bypassed if the expansion is under 5gp, or if it is a safe "Golden Margin"
If Ratio < 1.0 AND (Current_Spread - Worst_Spread) >= worst_spread_penalty_min_gap AND !isGoldenMargin:
    Penalty = Max(0.05, Ratio^2)
    TrendMultiplier *= Penalty

```

---

## 5. UI Abbreviations Legend
To save space on the UI data table, the price trend indicators are condensed into the following acronyms:
- **HC**: High Clamp (Outlier price safely capped)
- **KC**: Knife Crash (Outlier crashing price penalized)
- **GM**: Golden Margin (Outlier low price with stable high price, permitted)
- **HJP**: High Price Jitter Penalty
- **LJP**: Low Price Jitter Penalty
- **SJP**: Spread Jitter Penalty
- **SSP**: Spread Spike Penalty (Current spread vastly exceeds historical mean)
- **WSP**: Worst Spread Penalty
- **↓1h / ↓24h**: Moving average down-trend penalties
- **↑1h / ↑5m**: Short term volume spike rewards

---

## Final Formula
The final `Score` used to sort the tables is the product of everything:

```text
Score = PotentialProfit * Profit_Multiplier * ROI_Multiplier * VolumeRatioFactor * AbsoluteVolumeFactor * NudgeMultiplier * TrendMultiplier * SpikeMultiplier
```
