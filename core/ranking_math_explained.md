# OSRS GE Flip Analyzer: Scoring Formula Explained

The core philosophy of this analyzer is to transform raw GE prices and volumes into a single, comprehensive `Score` that dictates the recommendation order. The formula takes into account raw profit, required capital, volume constraints, historical item success, and market momentum.

Every variable listed below can be manually fine-tuned within the `config.json` file located in the root directory.

---

## 1. Base Profit & ROI Scaling
At the core of the score is the raw profit potential, bounded by the capital you can actually afford, and multiplied by your Return on Investment (ROI) to penalize dangerous, low-margin flips.

```text
Profit_Per_Item = HighPrice - LowPrice - Tax
Affordable_Qty = Min(Item_Limit, Floor(K_cap / LowPrice))

Potential_Profit = Profit_Per_Item * Affordable_Qty
ROI_Multiplier = Profit_Per_Item / LowPrice
```

> **Note:** Tax is controlled by `tax_rate` (default `0.02` for 2%) and capped at `tax_cap` (default `5,000,000`). Items under 50 gp are not taxed.

---

## 2. Capital Penalty Factor
*Config Variables: `capital_penalty_base_weight`, `capital_penalty_scale_weight`*

Flipping expensive items ties up cash. The capital penalty reduces the score for items that require massive amounts of capital relative to your configured `--capital` threshold (K_cap).

```text
Required_Capital = LowPrice * Affordable_Qty
Penalty_Curve = K_cap / (K_cap + Required_Capital)

CapitalFactor = capital_penalty_base_weight + (capital_penalty_scale_weight * Penalty_Curve)
```

By default, both weights are `0.5`, meaning an item taking infinite capital will at worst have its score halved (0.5), while an item taking near-zero capital retains its full score (1.0).

---

## 3. Volume Penalty Factors
*Config Variables: `volume_ratio_penalty_max`, `absolute_volume_penalty_max`*

Low liquidity items are inherently riskier because they take longer to buy and sell. There are two distinct volume penalties applied simultaneously:

### 3a. Volume Ratio Penalty
If an item's projected 4-hour volume is lower than its GE limit, its score is reduced quadratically.

```text
Projected_4h_Volume = Hourly_Volume * 4.0
Ratio = Projected_4h_Volume / Item_Limit
Penalty_Curve = Max(0, (1.0 - Ratio) / 0.9)

VolumeRatioFactor = Max(0.01, 1.0 - (volume_ratio_penalty_max * Penalty_Curve^2))
```

> **Important:** If `Projected_4h_Volume` is <= 10% of `Item_Limit`, the item is **filtered out completely**. `volume_ratio_penalty_max` defaults to `1.30`.

### 3b. Absolute Volume Penalty
Regardless of the item limit, items with incredibly low absolute hourly volume (under 100 trades/hour) are penalized.

```text
Penalty_Curve = Max(0, (100.0 - Hourly_Volume) / 90.0)
AbsoluteVolumeFactor = Max(0.01, 1.0 - (absolute_volume_penalty_max * Penalty_Curve^2))
```

> **Important:** If `Hourly_Volume` is <= 10, the item is **filtered out completely**.

---

## 4. Historical Nudges
*Config Variables: `flip_half_life_hours`, `failed_sell_half_life_hours`, `flip_modifier_*`, `failed_sell_penalty`, `nudge_min`, `nudge_max`*

Every time you record a Manual Flip or a Failed Sell, it leaves a permanent, decaying "nudge" on that item's score multiplier. Nudges are calculated using an Exponential Moving Average (EMA) that decays over time based on their half-lives.

```text
Multiplier = 1.0 + EMA(Recent_Flips) + EMA(Recent_Failed_Sells)
```
- **Flips:** Added based on rating (`flip_modifier_meh` = `-0.10`, `flip_modifier_great` = `+0.20`, etc). Decays over `flip_half_life_hours`.
- **Failed Sells:** Subtracted heavily (`failed_sell_penalty` = `-0.40`). Represents an unprofitable dump when the margin collapses. Decays over `failed_sell_half_life_hours`.

> **Tip:** The final multiplier is clamped strictly between `nudge_min` (default `0.05`) and `nudge_max` (default `2.0`).

---

## 5. Market Momentum Modifiers
The analyzer detects crashing markets and temporary volatility spikes to adjust recommendations.

### 5a. Price Trend Modifiers
*Config Variables: `price_trend_penalty_1h`, `price_trend_penalty_24h`, `price_trend_penalty_30d`*

If the current **HighPrice** (sell ceiling) is strictly less than historical moving averages, the market is actively crashing, and penalties are chained:
```text
TrendMultiplier = 1.0
If High_Price < 1h_Avg:   TrendMultiplier *= price_trend_penalty_1h
If High_Price < 24h_Avg:  TrendMultiplier *= price_trend_penalty_24h
If High_Price < 30d_Avg:  TrendMultiplier *= price_trend_penalty_30d
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

---

## Final Formula
The final `Score` used to sort the tables is the product of everything:

```text
Score = PotentialProfit * CapitalFactor * VolumeRatioFactor * AbsoluteVolumeFactor * NudgeMultiplier * TrendMultiplier * SpikeMultiplier * ROI_Multiplier
```
