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
ROI_Multiplier = Min(roi_reward_cap, Raw_ROI / target_roi) 

```

> **Note:** The OSRS Tax is exactly `0.02` (2%) and capped at `tax_cap` (default `5,000,000`). Because OSRS rounds tax down, items strictly `< 50 gp` are not taxed.

---

## 2. Affordable Capital Limit
*Config Variable: `base_capital`*

Flipping expensive items ties up cash. The analyzer limits the scored volume calculation to the amount you can realistically afford based on your configured `base_capital` threshold. 

If the capital required to purchase a full limit exceeds your `base_capital`, the volume used for scoring is clamped.

```text
Affordable_Limit = Min(Item_Limit, Floor(base_capital / LowPrice))
```

---

## 3. Volume Penalty Factors
*Config Variables: `volume_ratio_penalty_max`, `absolute_volume_penalty_max`*

Low liquidity items are inherently riskier because they take longer to buy and sell. There are two distinct volume penalties applied simultaneously:

### 3a. Volume Ratio Penalty
If an item's projected 4-hour volume is lower than its GE limit, its score is reduced quadratically. We assume a safe market capture rate of 5%.

```text
Projected_4h_Volume = Hourly_Volume * 4.0
Player_Capture_Volume = Projected_4h_Volume * 0.05
Ratio = Player_Capture_Volume / Item_Limit

if Ratio <= volume_ratio_filter_threshold: 
    Drop_Item() // Hard filter, item does not get scored

Penalty_Curve = Max(0, (1.0 - Ratio) / (1.0 - volume_ratio_filter_threshold))
VolumeRatioFactor = Max(0.01, 1.0 - (volume_ratio_penalty_max * Penalty_Curve^2))
```

> **Important:** If `Ratio` is <= `volume_ratio_filter_threshold` (default 10%), the item is **filtered out completely**. `volume_ratio_penalty_max` defaults to `1.30`.

### 3b. Absolute Volume Penalty
Regardless of the item limit, items with incredibly low absolute hourly volume (under 100 trades/hour) are penalized.

```text
Penalty_Curve = Max(0, (100.0 - Hourly_Volume) / 90.0)
AbsoluteVolumeFactor = Max(0.01, 1.0 - (absolute_volume_penalty_max * Penalty_Curve^2))
```

> **Important:** If `Hourly_Volume` is <= `min_absolute_volume` (default 10), the item is **filtered out completely**: `Drop_Item()`.

---

## 4. Historical Nudges
*Config Variables: `flip_half_life_hours`, `failed_sell_half_life_hours`, `flip_modifier_*`, `failed_sell_penalty`, `nudge_min`, `nudge_max`*

Every time you record a Manual Flip or a Failed Sell, it leaves a permanent, decaying "nudge" on that item's score multiplier. Nudges are calculated using an Exponential Moving Average (EMA) that decays over time based on their half-lives.

```text
Multiplier = 1.0 + Sum(Flip_Value * Exp(-Hours_Since_Flip / Half_Life))
```
- **Flips:** Added based on rating (`flip_modifier_meh` = `-0.10`, `flip_modifier_great` = `+0.20`, etc). Decays over `flip_half_life_hours`.
- **Failed Sells:** Subtracted heavily (`failed_sell_penalty` = `-0.40`). Represents an unprofitable dump when the margin collapses. Decays over `failed_sell_half_life_hours`.

> **Tip:** The final multiplier is clamped strictly between `nudge_min` (default `0.05`) and `nudge_max` (default `2.0`).

---

## 5. Market Momentum Modifiers
The analyzer detects crashing markets and temporary volatility spikes to adjust recommendations.

### 5a. Stale Price Penalty
*Config Variables: `stale_price_threshold_minutes`, `stale_price_penalty_multiplier`*

If an item has not been traded recently, its High and Low prices become highly unreliable, often representing abandoned GE offers rather than the true active spread.

```text
Threshold_Seconds = stale_price_threshold_minutes * 60

if (Current_Time - HighTime) > Threshold_Seconds OR (Current_Time - LowTime) > Threshold_Seconds:
    TrendMultiplier *= stale_price_penalty_multiplier
```
*Note: This penalty defaults to a massive 90% score reduction (`0.10`) for any item where the High or Low price is older than 5 minutes.*

### 5b. Price Trend Modifiers
*Config Variables: `price_trend_penalty_1h`, `price_trend_penalty_24h`*

If the current **HighPrice** (sell ceiling) is strictly less than historical moving averages, the market is actively crashing, and penalties are chained:
```text
TrendMultiplier = 1.0
If High_Price < 1h_Avg:   TrendMultiplier *= price_trend_penalty_1h
If High_Price < 24h_Avg:  TrendMultiplier *= price_trend_penalty_24h
```
*Note: We only check the HighPrice to ensure we don't accidentally penalize market dips where the LowPrice drops temporarily but the ceiling holds steady.*

### 5b. Volume Spike Modifiers
*Config Variables: `volume_spike_5m_multiplier`, `volume_spike_24h_multiplier`*

If volume surges uncharacteristically fast, it indicates heavy market momentum (e.g. panic dumps or FOMO pumps), which are extremely lucrative for flippers:
```text
SpikeMultiplier = 1.0
If 5m_Vol > (Expected_5m_Vol * 3):  SpikeMultiplier *= volume_spike_5m_multiplier (Default 1.50)
If 1h_Vol > (Expected_1h_Vol * 3):  SpikeMultiplier *= volume_spike_24h_multiplier (Default 1.20)
```

### 5c. Price Outlier Penalties (Rolling Z-Scores)
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

### 5d. Sustained Volatility Penalty (Midpoint Jitter)
*Config Variables: `volatility_threshold_percent` (e.g., 0.05), `volatility_penalty_multiplier` (e.g., 0.80)*

This penalty detects items whose underlying value is erratically whipsawing tick-to-tick, avoiding highly unstable markets. It analyzes the last 12 ticks (1 hour).

For each tick, we calculate the Midpoint to ignore safe margin widening:
```text
Mid_Old = (High_Old + Low_Old) / 2.0
Mid_New = (High_New + Low_New) / 2.0
```

For each step (Older to Newer):
```text
// 1. Enforce a minimum denominator of 100 gp to prevent 1 gp changes on cheap items from triggering massive percentages
Safe_Denominator = Max(Mid_Old, 100.0)

// 2. Divide by the OLDER tick to correctly calculate financial percent change
Step_Variation = |Mid_New - Mid_Old| / Safe_Denominator

// 3. Clamp the variation to max 10% per step. 
// This ensures a single massive outlier (handled by 5c) doesn't skew the average. We only want to penalize consistent jitter.
Capped_Step = Min(Step_Variation, 0.10) 
```

The average volatility is the sum of these capped steps divided by 11.

```text
Average_Volatility = Sum(Capped_Steps) / 11

If Average_Volatility > volatility_threshold_percent:
    // Calculate how far over the threshold we are
    Excess_Volatility = Average_Volatility - volatility_threshold_percent
    
    // 4. Use safe exponential decay to prevent penalty cliffs (scales smoothly downward from 1.0)
    Penalty = Exp(-(Excess_Volatility * volatility_penalty_multiplier))
    
    TrendMultiplier *= Penalty
```

### 5e. Spread Jitter & Spike Penalties
*Config Variables: `spread_jitter_high_threshold`, `spread_jitter_low_threshold`, `spread_jitter_penalty_multiplier`, `spread_jitter_reward_multiplier`, `spread_spike_threshold`, `spread_spike_penalty_multiplier`*

In addition to the midpoint jitter, we also track the reliability of the **Spread** (HighPrice - LowPrice) over the past hour (12 ticks). A wildly fluctuating spread means the profit margin is unstable due to heavy undercutting or low liquidity.

**Spread Jitter:**
We calculate the step-to-step variation of the spread, bounded by a 10gp floor to prevent micro-fluctuations on 1gp margins from triggering massive penalties. 

```text
Spread_Old = High_Old - Low_Old
Spread_New = High_New - Low_New

Safe_Spread = Max(Spread_Old, 10.0) 
Step_Variation = Min(|Spread_New - Spread_Old| / Safe_Spread, 0.50) // Capped at 50%
Average_Jitter = Sum(Step_Variation) / 11

if Average_Jitter > spread_jitter_high_threshold:
    Excess = Average_Jitter - spread_jitter_high_threshold
    TrendMultiplier *= Exp(-(Excess * spread_jitter_penalty_multiplier))

else if Average_Jitter < spread_jitter_low_threshold:
    // Reward for rock-solid reliable margins
    Scale = (spread_jitter_low_threshold - Average_Jitter) / Max(spread_jitter_low_threshold, 0.001)
    TrendMultiplier *= 1.0 + (spread_jitter_reward_multiplier - 1.0) * Scale
```

**Spread Spike:**
If the current, real-time spread is massively larger than the average spread of the last hour, this usually indicates an artificial price hike or a temporary dry-up of supply, which is highly risky to flip into.

```text
Mean_Spread = Average of last 12 Spreads
Spike_Ratio = Current_Spread / Max(Mean_Spread, 1.0)

if Spike_Ratio > spread_spike_threshold AND High_Price > Mean_High_Price AND (Current_Spread - Mean_Spread) >= 5.0:
    Excess = Spike_Ratio - spread_spike_threshold
    TrendMultiplier *= Exp(-(Excess * spread_spike_penalty_multiplier))
```

---

## 6. UI Abbreviations Legend
To save space on the UI data table, the price trend indicators are condensed into the following acronyms:
- **HC**: High Clamp (Outlier price safely capped)
- **KC**: Knife Crash (Outlier crashing price penalized)
- **GM**: Golden Margin (Outlier low price with stable high price, permitted)
- **JP**: Midpoint Jitter Penalty (Volatile midpoint)
- **SJP**: Spread Jitter Penalty (Volatile spread)
- **SS**: Solid Spread (Reward for stable spread)
- **SSP**: Spread Spike Penalty (Current spread vastly exceeds historical mean)
- **↓1h / ↓24h**: Moving average down-trend penalties
- **↑1h / ↑5m**: Short term volume spike rewards

---

## Final Formula
The final `Score` used to sort the tables is the product of everything:

```text
Score = PotentialProfit * VolumeRatioFactor * AbsoluteVolumeFactor * NudgeMultiplier * TrendMultiplier * SpikeMultiplier * ROI_Multiplier
```
