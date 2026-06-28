import re

with open("core/ranking_math_explained.md", "r") as f:
    text = f.read()

# 1. Final Formula
text = text.replace(
    "Score = PotentialProfit * VolumeRatioFactor * AbsoluteVolumeFactor * NudgeMultiplier * TrendMultiplier * SpikeMultiplier * ROI_Multiplier",
    "Score = PotentialProfit * Profit_Multiplier * ROI_Multiplier * VolumeRatioFactor * AbsoluteVolumeFactor * NudgeMultiplier * TrendMultiplier * SpikeMultiplier"
)

# 2. ROI Floor
text = text.replace(
    "ROI_Multiplier = Min(roi_reward_cap, Raw_ROI / target_roi)",
    "ROI_Multiplier = Max(0.50, Min(roi_reward_cap, Raw_ROI / target_roi))"
)

# 3. Remove Section 2
sec2 = """## 2. Affordable Capital Limit
*Config Variable: `base_capital`*

Flipping expensive items ties up cash. The analyzer limits the scored volume calculation to the amount you can realistically afford based on your configured `base_capital` threshold. 

If the capital required to purchase a full limit exceeds your `base_capital`, the volume used for scoring is clamped.

```text
Affordable_Limit = Min(Item_Limit, Floor(base_capital / LowPrice))
```

---

"""
text = text.replace(sec2, "")

# Bump sections 3-6 to 2-5
text = text.replace("## 3. Volume Penalty Factors", "## 2. Volume Penalty Factors")
text = text.replace("### 3a. Volume Ratio Penalty", "### 2a. Volume Ratio Penalty")
text = text.replace("### 3b. Absolute Volume Penalty", "### 2b. Absolute Volume Penalty")
text = text.replace("## 4. Historical Nudges", "## 3. Historical Nudges")
text = text.replace("## 5. Market Momentum Modifiers", "## 4. Market Momentum Modifiers")

text = text.replace("### 5a. Stale Price Penalty (Liquidity-Adjusted)", "### 4a. Stale Price Penalty (Liquidity-Adjusted)")
text = text.replace("### 5b. Price Trend Modifiers", "### 4b. Price Trend Modifiers")

# 5. Typo: Duplicate Section Numbers. Bump down rest of 5 (now 4)
text = text.replace("### 5b. Volume Spike Modifiers", "### 4c. Volume Spike Modifiers")
text = text.replace("### 5c. Price Outlier Penalties (Rolling Z-Scores)", "### 4d. Price Outlier Penalties (Rolling Z-Scores)")
text = text.replace("### 5d. Sustained Volatility Penalties (High & Low Price Jitter)", "### 4e. Sustained Volatility Penalties (High & Low Price Jitter)")
text = text.replace("### 5e. Spread Jitter & Spike Penalties", "### 4f. Spread Jitter & Spike Penalties")
text = text.replace("### 5f. Recent Worst Spread (Margin Reversion Penalty)", "### 4g. Recent Worst Spread (Margin Reversion Penalty)")

text = text.replace("## 6. UI Abbreviations Legend", "## 5. UI Abbreviations Legend")

# 4. The 20x Too-Strict Volume Filter (Section 3a & 3b -> 2a & 2b)
old_3a_code = """Projected_4h_Volume = Hourly_Volume * 4.0
Player_Capture_Volume = Projected_4h_Volume * 0.05
Ratio = Player_Capture_Volume / Item_Limit

if Ratio <= volume_ratio_filter_threshold: 
    Drop_Item() // Hard filter, item does not get scored

Penalty_Curve = Max(0, (1.0 - Ratio) / (1.0 - volume_ratio_filter_threshold))
VolumeRatioFactor = Max(0.01, 1.0 - (volume_ratio_penalty_max * Penalty_Curve^2))"""

new_2a_code = """Projected_4h_Volume = Hourly_Volume * 4.0
Global_Ratio = Projected_4h_Volume / Item_Limit

if Global_Ratio <= volume_ratio_filter_threshold: 
    VolumeRatioFactor = 0.001 // Massive penalty to sink dead items
else:
    Penalty_Curve = Max(0, (1.0 - Global_Ratio) / (1.0 - volume_ratio_filter_threshold))
    VolumeRatioFactor = Max(0.01, 1.0 - (volume_ratio_penalty_max * Penalty_Curve^2))"""

text = text.replace(old_3a_code, new_2a_code)

old_3b_code = """Penalty_Curve = Max(0, (100.0 - Hourly_Volume) / 90.0)
AbsoluteVolumeFactor = Max(0.01, 1.0 - (absolute_volume_penalty_max * Penalty_Curve^2))"""
text = text.replace(old_3b_code, old_3b_code) # just in case

text = text.replace("Drop_Item() // Hard filter, item does not get scored", "VolumeRatioFactor = 0.001 // Massive penalty to sink dead items")
text = text.replace("filtered out completely: `Drop_Item()`.", "penalized heavily: `AbsoluteVolumeFactor = 0.001`.")

# 6. Z-Score Clamp Hallucination Contradiction
old_clamp = "the algorithm literally clamps the evaluated `HighPrice` down to its most recent high-price data point in the rolling window"
new_clamp = "the algorithm literally clamps the evaluated `HighPrice` down to `Mean + (Threshold * StdDev)`"
text = text.replace(old_clamp, new_clamp)

# 7. Legacy Ghost Artifact
legacy_ghost = """*Note: The analyzer also explicitly monitors data staleness. If an item is missing recent High or Low price data within the past 2 hours, it is penalized exponentially based on how long it has been since the last valid tick (a `STALE` or `GHOST` indicator will appear).*
2."""
text = text.replace(legacy_ghost, "2.")

with open("core/ranking_math_explained.md", "w") as f:
    f.write(text)
