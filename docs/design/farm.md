# Farm & Plantation Cultivation Design

## Overview

The Farm and Plantation Feature Module (`internal/farm`) allows player characters to cultivate seeds into herbs, flowers, and rare alchemy/cooking ingredients across dedicated farm plots.

---

## Domain Rules & Mechanics

### Farm Plots

- Each player character possesses a dedicated farm with **4 individual plots** (indices `0` to `3`).
- **Plot Statuses**:
  - `EMPTY`: Plot is cleared and ready for planting.
  - `GROWING`: Crop is currently growing ($T_{\text{planted}} \le T_{\text{current}} < T_{\text{matures}}$).
  - `MATURE`: Crop is ready to harvest ($T_{\text{matures}} \le T_{\text{current}} < T_{\text{wither}}$).
  - `WITHERED`: Crop has withered due to neglect beyond grace period ($T_{\text{current}} \ge T_{\text{wither}}$).

---

## Seed & Crop Catalog

| Seed Type | Crop Name | Growth Duration | Grace Period | Base Yield | Reward Gold / Item |
| :--- | :--- | :---: | :---: | :---: | :---: |
| `seed_herb` | Medicinal Herb (薬草) | 5 minutes | 24 hours | 2 | 50 G / `item_medicinal_herb` |
| `seed_mandrake` | Mandrake Root (マンドラゴラ) | 15 minutes | 24 hours | 1 | 200 G / `item_mandrake_root` |
| `seed_moonlight` | Moonlight Flower (月光草) | 30 minutes | 24 hours | 1 | 500 G / `item_moonlight_flower` |
| `seed_golden` | Golden Apple (黄金の果実) | 60 minutes | 24 hours | 1 | 2,000 G / `item_golden_fruit` |

---

## Crop Care Operations

1. **Watering (`WaterPlot`)**:
   - Increases harvest yield by $+1$.
   - Allowed once per growth cycle.
2. **Fertilizing (`FertilizePlot`)**:
   - Halves the total remaining growth duration ($T_{\text{matures}} = T_{\text{planted}} + \frac{\text{Duration}}{2}$).
   - Allowed once per growth cycle.

---

## Harvesting & Clearing

- **Harvesting**:
  - Only allowed when the crop is in `MATURE` state.
  - Awards total yield ($\text{Yield} \times \text{RewardGold}$) and associated items.
  - Resets the plot back to `EMPTY`.
- **Clearing**:
  - Resets a `WITHERED` or abandoned plot back to `EMPTY`.

---

## Persistence & Transactions

- Farm plot states and character rewards are managed atomically in MariaDB (`farm_plots`).
- `UNIQUE (character_id, plot_index)` constraint ensures plot consistency per character.
