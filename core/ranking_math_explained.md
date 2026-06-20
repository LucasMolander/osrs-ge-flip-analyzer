# OSRS GE Flip Analyzer: Scoring Formula Explained

The core philosophy of this analyzer is to transform raw GE prices and volumes into a single, comprehensive `Score` that dictates the recommendation order. The formula takes into account raw profit, required capital, volume constraints, historical item success, and market momentum.

Every variable listed below can be manually fine-tuned within the `config.json` file located in the root directory.

---

## 1. Base Profit
At the core of the score is the raw profit potential of flipping a single limit of the item.

```text
Profit_Per_Item = HighPrice - LowPrice - Tax
Potential_Profit = Profit_Per_Item * Item_Limit
```

> **Note:** Tax is controlled by `tax_rate` (default `0.02` for 2%) and capped at `tax_cap` (default `5000000`). Items under 50 gp are not taxed.

---

## 2. Capital Penalty Factor
*Config Variables: `capital_penalty_base_weight`, `capital_penalty_scale_weight`*

Flipping expensive items ties up cash. The capital penalty reduces the score for items that require massive amounts of capital relative to your configured `--capital` threshold (K_cap).

```text
Required_Capital = LowPrice * Item_Limit
Penalty_Curve = K_cap / (K_cap + Required_Capital)

CapitalFactor = capital_penalty_base_weight + (capital_penalty_scale_weight * Penalty_Curve)
```

By default, both weights are `0.5`, meaning an item taking infinite capital will at worst have its score halved (0.5), while an item taking near-zero capital retains its full score (1.0).

---

## 3. Volume Penalty Factors
*Config Variables: `volume_ratio_penalty_max`, `absolute_volume_penalty_max`*

Low liquidity items are inherently riskier because they take longer to buy and sell. There are two distinct volume penalties applied simultaneously:

### 3a. Volume Ratio Penalty
If the item's hourly volume is lower than its GE limit, its score is reduced quadratically.

```text
Ratio = Hourly_Volume / Item_Limit
Penalty_Curve = (1.0 - Ratio) / 0.9

VolumeRatioFactor = 1.0 - (volume_ratio_penalty_max * Penalty_Curve^2)
```

> **Important:** If `Hourly_Volume` is <= 10% of `Item_Limit`, the item is **filtered out completely**. `volume_ratio_penalty_max` defaults to `1.30`, aggressively crushing the score of low-ratio items to 0.

### 3b. Absolute Volume Penalty
Regardless of the item limit, items with incredibly low absolute hourly volume (under 100 trades/hour) are penalized.

```text
Penalty_Curve = (100.0 - Hourly_Volume) / 90.0
AbsoluteVolumeFactor = 1.0 - (absolute_volume_penalty_max * Penalty_Curve^2)
```

> **Important:** If `Hourly_Volume` is <= 10, the item is **filtered out completely**.

---

## 4. Historical Nudges
*Config Variables: `flip_half_life_hours`, `failed_buy_half_life_hours`, `flip_modifier_*`, `failed_buy_penalty`, `nudge_min`, `nudge_max`*

Every time you record a Manual Flip or a Failed Buy, it leaves a permanent, decaying "nudge" on that item's score multiplier. Nudges start at `1.0` and decay back toward `1.0` over time using exponential decay based on their half-lives.

```text
Multiplier = 1.0 + Sum(Recent_Flips) + Sum(Recent_Failed_Buys)
```
- **Flips:** Added based on rating (`flip_modifier_meh` = `-0.10`, `flip_modifier_great` = `+0.20`, etc). Decays over `flip_half_life_hours`.
- **Failed Buys:** Subtracted permanently (`failed_buy_penalty` = `-0.40`). Decays over `failed_buy_half_life_hours`.

> **Tip:** The final sum of all historical nudges is clamped strictly between `nudge_min` (default `0.05`) and `nudge_max` (default `2.0`).

---

## 5. Market Momentum Modifiers
The analyzer detects crashing markets and temporary volatility spikes to protect you from buying into a falling knife.

### 5a. Price Trend Modifiers
*Config Variables: `price_trend_penalty_1h`, `price_trend_penalty_24h`, `price_trend_penalty_30d`*

If the current estimated price is *strictly less* than the historical moving averages, penalties are chained:
```text
TrendMultiplier = 1.0
If Current_Price < 1h_Avg:   TrendMultiplier *= price_trend_penalty_1h
If Current_Price < 24h_Avg:  TrendMultiplier *= price_trend_penalty_24h
If Current_Price < 30d_Avg:  TrendMultiplier *= price_trend_penalty_30d
```

### 5b. Volume Spike Modifiers
*Config Variables: `volume_spike_5m_multiplier`, `volume_spike_24h_multiplier`*

If volume surges uncharacteristically fast, it indicates a manipulated pump or panic dump:
```text
SpikeMultiplier = 1.0
If 5m_Vol > (Expected_5m_Vol * 3):  SpikeMultiplier *= volume_spike_5m_multiplier
If 1h_Vol > (Expected_1h_Vol * 3):  SpikeMultiplier *= volume_spike_24h_multiplier
```

---

## Final Formula
The final `Score` used to sort the tables is the product of everything:

```text
Score = PotentialProfit * CapitalFactor * VolumeRatioFactor * AbsoluteVolumeFactor * NudgeMultiplier * TrendMultiplier * SpikeMultiplier
```
