package dungeon

import "context"

// ContinuationControls describe service-owned eligibility, not command wiring.
type ContinuationControls struct {
	CanMove   bool `json:"can_move"`
	CanFight  bool `json:"can_fight"`
	CanEscape bool `json:"can_escape"`
}

func (exp ActiveExpedition) Controls(actor string) ContinuationControls {
	if exp.CharacterID != actor {
		return ContinuationControls{}
	}
	switch exp.Status {
	case StatusExploring:
		return ContinuationControls{CanMove: !exp.Encounter.blocking() && exp.TurnsRemaining > 0, CanFight: exp.Encounter.hostile(), CanEscape: true}
	case StatusCleared, StatusEscaped, StatusWipedOut:
		// Both operations retry the frozen outcome without fighting or changing it.
		return ContinuationControls{CanFight: true, CanEscape: true}
	default:
		return ContinuationControls{}
	}
}

// ExpeditionObservation excludes the private roster inventory and event/reward buffers.
type ExpeditionObservation struct {
	ID             string               `json:"id"`
	LeaderID       string               `json:"leader_id"`
	PartyID        string               `json:"party_id,omitempty"`
	DungeonID      string               `json:"dungeon_id"`
	Revision       int                  `json:"revision"`
	Status         ExpeditionStatus     `json:"status"`
	CurrentFloor   int                  `json:"current_floor"`
	TurnsRemaining int                  `json:"turns_remaining"`
	Round          int                  `json:"round,omitempty"`       // native maps only
	RoundLimit     int                  `json:"round_limit,omitempty"` // native maps only
	Map            *MapView             `json:"map"`
	Directions     []Direction          `json:"directions"`
	Controls       ContinuationControls `json:"controls"`
}

// ObserveExpedition reads one authoritative run, including terminal settlement buffers.
// It never advances combat, settles rewards, or repairs member references.
func (s *Service) ObserveExpedition(ctx context.Context, actor string) (*ExpeditionObservation, error) {
	exp, err := s.GetActiveExpedition(ctx, actor)
	if err != nil || exp == nil {
		return nil, err
	}
	view := &ExpeditionObservation{ID: exp.ID, LeaderID: exp.CharacterID, PartyID: exp.PartyID, DungeonID: exp.DungeonID, Revision: exp.Revision, Status: exp.Status, CurrentFloor: exp.CurrentFloor, TurnsRemaining: exp.TurnsRemaining, Directions: []Direction{}, Controls: exp.Controls(actor)}
	if exp.Status != StatusExploring {
		return view, nil
	}
	m, err := s.viewMap(ctx, actor, exp)
	if err != nil {
		return nil, err
	}
	view.Map = &m
	d, floor, err := s.activeFloor(exp)
	if err != nil {
		return nil, err
	}
	if d.native() {
		view.Round, view.RoundLimit = d.RoundLimit-exp.TurnsRemaining, d.RoundLimit
		// Legacy registers all four directions; walls are rejected on use, so
		// the list cannot reveal a hidden passage.
		if view.Controls.CanMove {
			view.Directions = []Direction{DirectionNorth, DirectionSouth, DirectionEast, DirectionWest}
		}
		return view, nil
	}
	if view.Controls.CanMove {
		for _, step := range []struct {
			dir  Direction
			x, y int
		}{{DirectionNorth, exp.PosX, exp.PosY - 1}, {DirectionSouth, exp.PosX, exp.PosY + 1}, {DirectionEast, exp.PosX + 1, exp.PosY}, {DirectionWest, exp.PosX - 1, exp.PosY}} {
			if step.x >= 0 && step.y >= 0 && step.x < floor.Width && step.y < floor.Height && floor.Grid[step.y][step.x] != '1' {
				view.Directions = append(view.Directions, step.dir)
			}
		}
	}
	return view, nil
}
