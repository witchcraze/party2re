package dungeon

import (
	"context"
	"strings"
)

type MapView struct {
	Radius              int        `json:"radius"`
	CenterX             int        `json:"center_x"`
	CenterY             int        `json:"center_y"`
	Tiles               [][]string `json:"tiles"`
	Formatted           string     `json:"formatted"`
	ScoutingBonusActive bool       `json:"scouting_bonus_active"`
	BonusReason         string     `json:"bonus_reason,omitempty"`
}

func isScoutingJob(jobID string) bool {
	j := strings.ToLower(strings.TrimSpace(jobID))
	switch j {
	case "9", "26", "27", "79", "thief", "ninja", "geomancer", "ranger", "盗賊", "忍者", "風水師", "レンジャー":
		return true
	default:
		return strings.Contains(j, "thief") || strings.Contains(j, "ninja") || strings.Contains(j, "geomancer") || strings.Contains(j, "ranger")
	}
}

func hasScopeGoggles(itemIDs []string) bool {
	for _, it := range itemIDs {
		lower := strings.ToLower(strings.TrimSpace(it))
		if lower == "197" || lower == "scope_goggles" || strings.Contains(lower, "scope") || strings.Contains(lower, "スコープ") {
			return true
		}
	}
	return false
}

// ViewMap renders the surrounding dungeon grid (legacy vs_dungeon.cgi: @ちず).
// Base view radius is 1 (3x3 grid). If any party member is a Thief (9), Ninja (26),
// Geomancer (27), Ranger (79), or possesses Scope Goggles (Item 197), the view radius expands to 2 (5x5 grid).
func (s *Service) ViewMap(ctx context.Context, characterID string) (MapView, error) {
	if strings.TrimSpace(characterID) == "" {
		return MapView{}, ErrCharacterNotFound
	}

	exp, err := s.activeStore.GetActiveExpedition(ctx, characterID)
	if err != nil {
		return MapView{}, err
	}
	if exp == nil || exp.Status != StatusExploring {
		return MapView{}, ErrNoActiveExpedition
	}

	dungeon, ok := s.dungeonMap[exp.DungeonID]
	if !ok {
		return MapView{}, ErrDungeonNotFound
	}

	floorIdx := exp.CurrentFloor - 1
	if floorIdx < 0 || floorIdx >= len(dungeon.Floors) {
		return MapView{}, ErrDungeonNotFound
	}
	floor := dungeon.Floors[floorIdx]

	radius := 1
	bonusActive := false
	bonusReason := ""

	// Check party members for scouting job or scope goggles
	for _, m := range exp.Members {
		if isScoutingJob(m.JobID) {
			radius = 2
			bonusActive = true
			bonusReason = "Scouting Job (" + m.JobID + "): " + m.Name
			break
		}
		if hasScopeGoggles(m.ItemIDs) {
			radius = 2
			bonusActive = true
			bonusReason = "Item 197 (Scope Goggles): " + m.Name
			break
		}
	}

	// Fallback check if solo without members populated
	if !bonusActive && len(exp.Members) == 0 {
		if char, cErr := s.characterRepo.FindByID(ctx, characterID); cErr == nil {
			if isScoutingJob(char.JobID) {
				radius = 2
				bonusActive = true
				bonusReason = "Scouting Job (" + char.JobID + "): " + char.Name
			} else if s.invRepo != nil {
				if inv, iErr := s.invRepo.FindByCharacterID(ctx, characterID); iErr == nil {
					for _, it := range inv.Items {
						if hasScopeGoggles([]string{it.DefinitionID}) {
							radius = 2
							bonusActive = true
							bonusReason = "Item 197 (Scope Goggles): " + char.Name
							break
						}
					}
				}
			}
		}
	}

	dim := 2*radius + 1
	tiles := make([][]string, dim)
	var formattedLines []string

	for rowIdx, dy := 0, -radius; dy <= radius; dy, rowIdx = dy+1, rowIdx+1 {
		tiles[rowIdx] = make([]string, dim)
		var lineBuilder strings.Builder
		for colIdx, dx := 0, -radius; dx <= radius; dx, colIdx = dx+1, colIdx+1 {
			var symbol string
			if dx == 0 && dy == 0 {
				symbol = "●" // player/party location
			} else {
				targetY := exp.PosY + dy
				targetX := exp.PosX + dx
				if targetY < 0 || targetY >= floor.Height || targetX < 0 || targetX >= floor.Width {
					symbol = "■" // out of bounds / wall
				} else {
					tileChar := floor.Grid[targetY][targetX]
					if tileChar == '1' {
						symbol = "■" // wall
					} else {
						symbol = "□" // path
					}
				}
			}
			tiles[rowIdx][colIdx] = symbol
			lineBuilder.WriteString(symbol)
		}
		formattedLines = append(formattedLines, lineBuilder.String())
	}

	return MapView{
		Radius:              radius,
		CenterX:             exp.PosX,
		CenterY:             exp.PosY,
		Tiles:               tiles,
		Formatted:           strings.Join(formattedLines, "\n"),
		ScoutingBonusActive: bonusActive,
		BonusReason:         bonusReason,
	}, nil
}
