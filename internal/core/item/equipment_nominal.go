package item

import (
	"strconv"
	"strings"
)

func parseIndex(defID, prefix string) int {
	clean := strings.TrimPrefix(defID, prefix)
	no, err := strconv.Atoi(clean)
	if err != nil {
		return 0
	}
	return no
}

// GetWeaponNominalStats returns canonical nominal (attack power, weight) according to @weas in _data.cgi:394-500.
func GetWeaponNominalStats(defID string) (int, int) {
	if defID == "item-wea" {
		return 30, 20
	}
	if !strings.HasPrefix(defID, "weapon-") && !strings.HasPrefix(defID, "wea-") {
		return 0, 0
	}
	no := parseIndex(defID, "weapon-")
	if no <= 0 {
		no = parseIndex(defID, "wea-")
	}

	switch no {
	case 1:
		return 2, 0
	case 2:
		return 4, 2
	case 3:
		return 8, 6
	case 4:
		return 6, 5
	case 5:
		return 15, 5
	case 6:
		return 9, 7
	case 7:
		return 40, 12
	case 8:
		return 14, 9
	case 9:
		return 24, 10
	case 10:
		return 18, 15
	case 11:
		return 30, 25
	case 12:
		return 42, 4
	case 13:
		return 30, 20
	case 14:
		return 27, 25
	case 15:
		return 44, 24
	case 16:
		return 60, 20
	case 17:
		return 18, 16
	case 18:
		return 54, 34
	case 19:
		return 47, 50
	case 20:
		return 50, 38
	case 21:
		return 90, 32
	case 22:
		return 150, 50
	case 23:
		return 75, 45
	case 24:
		return 65, 70
	case 25:
		return 99, 66
	case 26:
		return 120, 45
	case 27:
		return 180, 60
	case 28:
		return -20, -40
	case 29:
		return 5, -40
	case 30:
		return 70, 20
	case 31:
		return 0, 50
	case 32:
		return 0, 40
	case 33:
		return 0, 40
	case 34:
		return 0, 70
	case 35:
		return 0, 50
	case 36:
		return 90, 100
	case 37:
		return 300, 70
	case 38:
		return 200, 55
	case 39:
		return 150, 80
	case 40:
		return 150, 75
	case 41:
		return 1, 30
	case 42:
		return 22, 12
	case 43:
		return 14, 11
	case 44:
		return 60, 15
	case 45:
		return 31, 19
	case 46:
		return 36, 10
	case 47:
		return 20, 30
	case 48:
		return 20, 55
	case 49:
		return 52, 26
	case 50:
		return 80, 39
	case 51:
		return 45, -40
	case 52:
		return -30, -60
	case 53:
		return 120, 60
	case 54:
		return 85, 10
	case 55:
		return 65, 45
	case 56:
		return 96, 50
	case 57:
		return 60, 20
	case 58:
		return 110, 55
	case 59:
		return 84, 40
	case 60:
		return 300, 85
	case 61:
		return 70, 55
	case 62:
		return 90, 65
	case 63:
		return 100, 80
	case 64:
		return 54, 16
	case 65:
		return 61, 11
	case 66:
		return 78, -30
	case 67:
		return 165, 80
	case 68:
		return 180, 0
	case 69:
		return 0, 40
	case 70:
		return 0, 50
	case 71:
		return 0, 60
	default:
		return 0, 0
	}
}

// GetArmorNominalStats returns canonical nominal (defense power, weight) according to @arms in _data.cgi:506-613.
func GetArmorNominalStats(defID string) (int, int) {
	if defID == "item-arm" {
		return 10, 6
	}
	no := parseIndex(defID, "armor-")
	if no <= 0 {
		no = parseIndex(defID, "shield-")
	}
	if no <= 0 {
		no = parseIndex(defID, "arm-")
	}
	if no <= 0 {
		no = parseIndex(defID, "item-")
	}

	if strings.HasPrefix(defID, "item-") {
		switch no {
		case 116: // ドクロの指輪
			return 30, 0
		case 117: // 金のロザリオ
			return 30, 0
		case 118: // 金の指輪
			return 15, 0
		case 119: // 金のブレスレット
			return 50, 0
		case 120: // はやてのリング
			return 0, -30
		case 121: // ほしふる腕輪
			return 0, -60
		case 122: // 力の指輪
			return 0, 0
		case 123: // ごうけつの腕輪
			return 0, 0
		case 124: // アルゴンリング
			return 0, -30
		case 240: // 覚醒の紅玉
			return -20, 20
		case 241: // 覚醒の蒼玉
			return 30, 20
		case 242: // 覚醒の翠玉
			return -20, -30
		}
	}

	switch no {
	case 1:
		return 3, 0
	case 2:
		return 5, 0
	case 3:
		return -2, -12
	case 4:
		return 12, 4
	case 5:
		return 11, 6
	case 6:
		return 10, 6
	case 7:
		return 24, 8
	case 8:
		return 20, 10
	case 9:
		return 30, 18
	case 10:
		return 20, 12
	case 11:
		return 43, 17
	case 12:
		return 34, 20
	case 13:
		return 90, 22
	case 14:
		return 0, -55
	case 15:
		return 30, 20
	case 16:
		return 52, 20
	case 17:
		return 30, 22
	case 18:
		return 45, 25
	case 19:
		return 73, 24
	case 20:
		return 140, 22
	case 21:
		return 62, 30
	case 22:
		return 40, 27
	case 23:
		return 25, -30
	case 24:
		return 70, 35
	case 25:
		return 180, 33
	case 26:
		return 76, 30
	case 27:
		return 50, 40
	case 28:
		return 40, -40
	case 29:
		return 90, 34
	case 30:
		return 70, 32
	case 31:
		return 220, 44
	case 32:
		return 10, -20
	case 33:
		return 77, -77
	case 34:
		return -30, -100
	case 35:
		return 50, 0
	case 36:
		return 100, 50
	case 37:
		return 50, 42
	case 38:
		return 300, 55
	case 39:
		return 150, 40
	case 40:
		return 150, 45
	case 41:
		return 1, 30
	case 42:
		return 32, 14
	case 43:
		return 55, 17
	case 44:
		return 35, 20
	case 45:
		return 75, 40
	case 46:
		return 92, 40
	case 47:
		return 95, 38
	case 48:
		return 20, -10
	case 49:
		return 40, -15
	case 50:
		return 90, 50
	case 51:
		return 160, 60
	case 52:
		return 165, 50
	case 53:
		return 180, 55
	case 54:
		return 25, 5
	case 55:
		return 25, 5
	default:
		return 0, 0
	}
}
