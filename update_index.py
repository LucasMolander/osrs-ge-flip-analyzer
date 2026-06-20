import re

with open('web/frontend/index.html', 'r') as f:
    html = f.read()

# 1. Remove Tax Rate and Tax Cap
html = re.sub(r'<div class="form-group">\s*<label>Tax Rate.*?</label>\s*<input type="number" step="0.001" v-model.number="localConfig.tax_rate">\s*</div>', '', html, flags=re.DOTALL)
html = re.sub(r'<div class="form-group">\s*<label>Tax Cap.*?</label>\s*<input type="number" v-model.number="localConfig.tax_cap">\s*</div>', '', html, flags=re.DOTALL)

# 2. Add missing fields: MinAbsoluteVolume, VolumeRatioFilterThreshold, NudgeMin, NudgeMax, VolumeRatioPenaltyMax, AbsoluteVolumePenaltyMax
missing_fields = """
                            <div class="form-group">
                                <label>Min Absolute Volume</label>
                                <input type="number" v-model.number="localConfig.min_absolute_volume">
                            </div>
                            <div class="form-group">
                                <label>Volume Ratio Filter</label>
                                <input type="number" step="0.01" v-model.number="localConfig.volume_ratio_filter_threshold">
                            </div>
"""

html = html.replace('<h3>Core Constraints & Taxation</h3>', '<h3>Core Constraints</h3>')
html = html.replace('<div class="form-group">\n                                <label>Base Capital (GP Threshold)</label>', missing_fields + '\n                            <div class="form-group">\n                                <label>Base Capital (GP Threshold)</label>')

missing_fields_nudges = """
                            <div class="form-group">
                                <label>Nudge Min</label>
                                <input type="number" step="0.01" v-model.number="localConfig.nudge_min">
                            </div>
                            <div class="form-group">
                                <label>Nudge Max</label>
                                <input type="number" step="0.01" v-model.number="localConfig.nudge_max">
                            </div>
"""
html = html.replace('<h3>Failed Sells Modifiers</h3>', '<h3>Failed Sells Modifiers & Nudge Limits</h3>\n' + missing_fields_nudges)

missing_fields_vol_pen = """
                            <div class="form-group">
                                <label>Volume Ratio Penalty Max</label>
                                <input type="number" step="0.01" v-model.number="localConfig.volume_ratio_penalty_max">
                            </div>
                            <div class="form-group">
                                <label>Absolute Volume Penalty Max</label>
                                <input type="number" step="0.01" v-model.number="localConfig.absolute_volume_penalty_max">
                            </div>
"""
html = html.replace('<h3>Base Score & Trend Penalties</h3>', '<h3>Base Score & Trend Penalties</h3>\n' + missing_fields_vol_pen)

# 3. Add CSS for grouping
css = """
        .form-row {
            display: flex;
            gap: 15px;
            margin-bottom: 15px;
            flex-wrap: wrap;
        }
        .form-row .form-group {
            flex: 1;
            min-width: 120px;
        }
"""
html = html.replace('</head>', f'    <style>{css}    </style>\n</head>')

# 4. Group Flip Modifiers
flip_modifiers_old = """                            <div class="form-group">
                                <label>Half Life (Hours)</label>
                                <input type="number" v-model.number="localConfig.flip_half_life_hours">
                            </div>
                            <div class="form-group">
                                <label>Modifier: Meh</label>
                                <input type="number" step="0.01" v-model.number="localConfig.flip_modifier_meh">
                            </div>
                            <div class="form-group">
                                <label>Modifier: Mid</label>
                                <input type="number" step="0.01" v-model.number="localConfig.flip_modifier_mid">
                            </div>
                            <div class="form-group">
                                <label>Modifier: Good</label>
                                <input type="number" step="0.01" v-model.number="localConfig.flip_modifier_good">
                            </div>
                            <div class="form-group">
                                <label>Modifier: Great</label>
                                <input type="number" step="0.01" v-model.number="localConfig.flip_modifier_great">
                            </div>"""

flip_modifiers_new = """                            <div class="form-row">
                                <div class="form-group">
                                    <label>Half Life (Hours)</label>
                                    <input type="number" v-model.number="localConfig.flip_half_life_hours">
                                </div>
                                <div class="form-group">
                                    <label>Modifier: Meh</label>
                                    <input type="number" step="0.01" v-model.number="localConfig.flip_modifier_meh">
                                </div>
                                <div class="form-group">
                                    <label>Modifier: Mid</label>
                                    <input type="number" step="0.01" v-model.number="localConfig.flip_modifier_mid">
                                </div>
                                <div class="form-group">
                                    <label>Modifier: Good</label>
                                    <input type="number" step="0.01" v-model.number="localConfig.flip_modifier_good">
                                </div>
                                <div class="form-group">
                                    <label>Modifier: Great</label>
                                    <input type="number" step="0.01" v-model.number="localConfig.flip_modifier_great">
                                </div>
                            </div>"""
html = html.replace(flip_modifiers_old, flip_modifiers_new)

# Group Spread Jitter
jitter_old = """                            <div class="form-group">
                                <label>Jitter High Threshold (%)</label>
                                <input type="number" step="0.01" v-model.number="localConfig.spread_jitter_high_threshold">
                            </div>
                            <div class="form-group">
                                <label>Jitter Low Threshold (%)</label>
                                <input type="number" step="0.01" v-model.number="localConfig.spread_jitter_low_threshold">
                            </div>
                            <div class="form-group">
                                <label>Jitter Penalty Multiplier</label>
                                <input type="number" step="0.1" v-model.number="localConfig.spread_jitter_penalty_multiplier">
                            </div>
                            <div class="form-group">
                                <label>Jitter Reward Multiplier</label>
                                <input type="number" step="0.01" v-model.number="localConfig.spread_jitter_reward_multiplier">
                            </div>
                            <div class="form-group">
                                <label>Spike Threshold (Ratio)</label>
                                <input type="number" step="0.1" v-model.number="localConfig.spread_spike_threshold">
                            </div>
                            <div class="form-group">
                                <label>Spike Penalty Multiplier</label>
                                <input type="number" step="0.1" v-model.number="localConfig.spread_spike_penalty_multiplier">
                            </div>"""

jitter_new = """                            <div class="form-row">
                                <div class="form-group">
                                    <label>Jitter High (%)</label>
                                    <input type="number" step="0.01" v-model.number="localConfig.spread_jitter_high_threshold">
                                </div>
                                <div class="form-group">
                                    <label>Jitter Low (%)</label>
                                    <input type="number" step="0.01" v-model.number="localConfig.spread_jitter_low_threshold">
                                </div>
                                <div class="form-group">
                                    <label>Penalty Mult</label>
                                    <input type="number" step="0.1" v-model.number="localConfig.spread_jitter_penalty_multiplier">
                                </div>
                                <div class="form-group">
                                    <label>Reward Mult</label>
                                    <input type="number" step="0.01" v-model.number="localConfig.spread_jitter_reward_multiplier">
                                </div>
                            </div>
                            <div class="form-row">
                                <div class="form-group">
                                    <label>Spike Threshold (Ratio)</label>
                                    <input type="number" step="0.1" v-model.number="localConfig.spread_spike_threshold">
                                </div>
                                <div class="form-group">
                                    <label>Spike Penalty Multiplier</label>
                                    <input type="number" step="0.1" v-model.number="localConfig.spread_spike_penalty_multiplier">
                                </div>
                            </div>"""
html = html.replace(jitter_old, jitter_new)

with open('web/frontend/index.html', 'w') as f:
    f.write(html)

