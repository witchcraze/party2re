package ranking

import (
	"errors"
	"time"
)

// LegendCategory identifies the Hall of Fame category (comp_*).
type LegendCategory string

const (
	LegendCategoryJobMastery     LegendCategory = "comp_job"
	LegendCategoryMonsterMastery LegendCategory = "comp_mon"
	LegendCategoryWeaponMastery  LegendCategory = "comp_wea"
	LegendCategoryArmorMastery   LegendCategory = "comp_arm"
	LegendCategoryItemMastery    LegendCategory = "comp_ite"
	LegendCategoryAlchemyMastery LegendCategory = "comp_alc"
)

var (
	ErrInvalidLegendCategory = errors.New("invalid legend category")
)

// LegendCategoryInfo holds metadata and induction count for a Hall of Fame category.
type LegendCategoryInfo struct {
	Category    LegendCategory `json:"category"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	TotalCount  int            `json:"total_count"`
}

// LegendEntry represents a single player inducted into the Hall of Fame.
type LegendEntry struct {
	ID            int64          `json:"id"`
	Category      LegendCategory `json:"category"`
	CharacterID   string         `json:"character_id"`
	CharacterName string         `json:"character_name"`
	GuildName     string         `json:"guild_name,omitempty"`
	Color         string         `json:"color,omitempty"`
	Icon          string         `json:"icon,omitempty"`
	Message       string         `json:"message,omitempty"`
	InductedAt    time.Time      `json:"inducted_at"`
}

// LegendCategoryPage represents the response container for a specific Hall of Fame category.
type LegendCategoryPage struct {
	Category    LegendCategory `json:"category"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Entries     []LegendEntry  `json:"entries"`
	Total       int            `json:"total"`
}

// AllLegendCategories returns canonical legend category definitions.
func AllLegendCategories() []LegendCategoryInfo {
	return []LegendCategoryInfo{
		{
			Category:    LegendCategoryJobMastery,
			Title:       "ジョブマスター",
			Description: "All jobs mastered (ジョブコンプリート)",
		},
		{
			Category:    LegendCategoryMonsterMastery,
			Title:       "モンスターマスター",
			Description: "Monster book completed (モンスター図鑑コンプリート)",
		},
		{
			Category:    LegendCategoryWeaponMastery,
			Title:       "ウェポンキラー",
			Description: "Weapon encyclopedia completed (武器図鑑コンプリート)",
		},
		{
			Category:    LegendCategoryArmorMastery,
			Title:       "アーマーキング",
			Description: "Armor encyclopedia completed (防具図鑑コンプリート)",
		},
		{
			Category:    LegendCategoryItemMastery,
			Title:       "アイテムニスト",
			Description: "Item encyclopedia completed (アイテム図鑑コンプリート)",
		},
		{
			Category:    LegendCategoryAlchemyMastery,
			Title:       "アルケミスト",
			Description: "All alchemy recipes completed (錬金レシピコンプリート)",
		},
	}
}

// IsValidLegendCategory returns true if the specified category is valid.
func IsValidLegendCategory(cat LegendCategory) bool {
	switch cat {
	case LegendCategoryJobMastery,
		LegendCategoryMonsterMastery,
		LegendCategoryWeaponMastery,
		LegendCategoryArmorMastery,
		LegendCategoryItemMastery,
		LegendCategoryAlchemyMastery:
		return true
	default:
		return false
	}
}

// GetLegendCategoryInfo returns metadata for a specific category.
func GetLegendCategoryInfo(cat LegendCategory) (LegendCategoryInfo, bool) {
	for _, info := range AllLegendCategories() {
		if info.Category == cat {
			return info, true
		}
	}
	return LegendCategoryInfo{}, false
}
