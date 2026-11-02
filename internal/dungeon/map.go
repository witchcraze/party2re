package dungeon

import (
	"context"
	"fmt"
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
	case "job-09", "job-26", "job-27", "job-79", "9", "26", "27", "79", "thief", "ninja", "geomancer", "ranger", "盗賊", "忍者", "風水師", "レンジャー":
		return true
	default:
		return strings.Contains(j, "thief") || strings.Contains(j, "ninja") || strings.Contains(j, "geomancer") || strings.Contains(j, "ranger")
	}
}

func hasScopeGoggles(itemIDs []string) bool {
	for _, it := range itemIDs {
		lower := strings.ToLower(strings.TrimSpace(it))
		if lower == "item-197" || lower == "197" || lower == "scope_goggles" || strings.Contains(lower, "scope") || strings.Contains(lower, "スコープ") {
			return true
		}
	}
	return false
}

// viewMap projects one owned snapshot; bonuses belong to the viewing actor.
func (s *Service) viewMap(ctx context.Context, characterID string, exp *ActiveExpedition) (MapView, error) {
	dungeon, ok := s.dungeonMap[exp.DungeonID]
	if !ok {
		return MapView{}, ErrDungeonNotFound
	}

	floorIdx := exp.CurrentFloor - 1
	if floorIdx < 0 || floorIdx >= len(dungeon.Floors) {
		return MapView{}, ErrDungeonNotFound
	}
	floor := dungeon.Floors[floorIdx]
	if floor.Width <= 0 || floor.Height <= 0 || len(floor.Grid) != floor.Height || exp.PosX < 0 || exp.PosX >= floor.Width || exp.PosY < 0 || exp.PosY >= floor.Height {
		return MapView{}, fmt.Errorf("invalid dungeon map geometry")
	}
	for _, row := range floor.Grid {
		if len(row) != floor.Width {
			return MapView{}, fmt.Errorf("invalid dungeon map row")
		}
	}

	hasScoutJob := false
	scoutReason := ""
	hasGoggles := false
	goggleReason := ""
	actorFound := false
	needsInventory := true

	// Saved inventory belongs to this run; another member cannot lend visibility.
	for _, m := range exp.Members {
		if m.CharacterID != characterID {
			continue
		}
		actorFound = true
		hasScoutJob = isScoutingJob(m.JobID)
		hasGoggles = hasScopeGoggles(m.ItemIDs)
		needsInventory = len(m.ItemIDs) == 0
		scoutReason = "Scouting Job (" + m.JobID + "): " + m.Name
		goggleReason = "Item 197 (Scope Goggles): " + m.Name
	}
	if len(exp.Members) > 0 && !actorFound {
		return MapView{}, ErrExpeditionStateChanged
	}

	// Older solo buffers have no roster; their required sources must still succeed.
	if len(exp.Members) == 0 {
		char, err := s.characterRepo.FindByID(ctx, characterID)
		if err != nil {
			return MapView{}, err
		}
		hasScoutJob = isScoutingJob(char.JobID)
		scoutReason = "Scouting Job (" + char.JobID + "): " + char.Name
		goggleReason = "Item 197 (Scope Goggles): " + char.Name
	}
	// Empty item_ids also encodes older runs whose inventory was never read.
	if needsInventory {
		if s.invRepo == nil {
			return MapView{}, fmt.Errorf("dungeon inventory reader not configured")
		}
		inv, err := s.invRepo.FindByCharacterID(ctx, characterID)
		if err != nil {
			return MapView{}, err
		}
		for _, it := range inv.Items {
			if it.Quantity > 0 && hasScopeGoggles([]string{it.DefinitionID}) {
				hasGoggles = true
				break
			}
		}
	}

	radius := 1
	if hasScoutJob {
		radius++
	}
	if hasGoggles {
		radius++
	}

	bonusActive := hasScoutJob || hasGoggles
	bonusReason := ""
	if hasScoutJob && hasGoggles {
		bonusReason = scoutReason + " + " + goggleReason
	} else if hasScoutJob {
		bonusReason = scoutReason
	} else if hasGoggles {
		bonusReason = goggleReason
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
