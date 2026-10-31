package adventure

import "strings"

// SeasonToSubID maps a seasonal variant name to its legacy sub-identity (vs_monster.cgi:111-120).
// 1 = spring, 2 = summer, 3 = autumn, 4 = winter; 0 for unclassified or non-seasonal stages.
func SeasonToSubID(season string) int {
	switch strings.ToLower(strings.TrimSpace(season)) {
	case "spring":
		return 1
	case "summer":
		return 2
	case "autumn":
		return 3
	case "winter":
		return 4
	default:
		return 0
	}
}

// SubIDToSeason maps a legacy sub-identity integer (1..4) to its seasonal variant name.
func SubIDToSeason(subID int) string {
	switch subID {
	case 1:
		return "spring"
	case 2:
		return "summer"
	case 3:
		return "autumn"
	case 4:
		return "winter"
	default:
		return ""
	}
}
