package dungeon

import "context"

// ContinuationControls describe service-owned eligibility, not command wiring.
type ContinuationControls struct {
	CanMove   bool `json:"can_move"`
	CanFight  bool `json:"can_fight"`
	CanEscape bool `json:"can_escape"`
}

// Any living member may move; combat and Escape stay with the leader.
func (exp ActiveExpedition) Controls(actor string) ContinuationControls {
	canMove := exp.Status == StatusExploring && exp.isAlive(actor) && !exp.Encounter.blocking() && exp.Round < exp.RoundLimit
	if exp.CharacterID != actor {
		return ContinuationControls{CanMove: canMove}
	}
	switch exp.Status {
	case StatusExploring:
		return ContinuationControls{CanMove: canMove, CanFight: exp.Encounter.hostile(), CanEscape: true}
	case StatusEscaped, StatusWipedOut:
		// Both operations retry the frozen outcome without fighting or changing it.
		return ContinuationControls{CanFight: true, CanEscape: true}
	default:
		return ContinuationControls{}
	}
}

// ExpeditionObservation excludes the private roster inventory and event/reward buffers.
type ExpeditionObservation struct {
	ID         string               `json:"id"`
	LeaderID   string               `json:"leader_id"`
	PartyID    string               `json:"party_id,omitempty"`
	DungeonID  string               `json:"dungeon_id"`
	Revision   int                  `json:"revision"`
	Status     ExpeditionStatus     `json:"status"`
	Round      int                  `json:"round,omitempty"`
	RoundLimit int                  `json:"round_limit,omitempty"`
	Floor      int                  `json:"floor,omitempty"`
	Map        *MapView             `json:"map"`
	Directions []Direction          `json:"directions"`
	Controls   ContinuationControls `json:"controls"`
}

// ObserveExpedition reads one authoritative run, including terminal settlement buffers.
// It never advances combat, settles rewards, or repairs member references.
func (s *Service) ObserveExpedition(ctx context.Context, actor string) (*ExpeditionObservation, error) {
	exp, err := s.GetActiveExpedition(ctx, actor)
	if err != nil || exp == nil {
		return nil, err
	}
	view := &ExpeditionObservation{ID: exp.ID, LeaderID: exp.CharacterID, PartyID: exp.PartyID, DungeonID: exp.DungeonID, Revision: exp.Revision, Status: exp.Status, Directions: []Direction{}, Controls: exp.Controls(actor)}
	if exp.Status != StatusExploring {
		return view, nil
	}
	m, err := s.viewMap(ctx, actor, exp)
	if err != nil {
		return nil, err
	}
	view.Map = &m
	_, floor, err := s.activeFloor(exp)
	if err != nil {
		return nil, err
	}
	// A pitfall to a floor with a lower limit can leave the round above it.
	view.Round, view.RoundLimit, view.Floor = exp.Round, exp.RoundLimit, floor.FloorNumber
	// Legacy registers all four directions; walls are rejected on use, so
	// the list cannot reveal a hidden passage.
	if view.Controls.CanMove {
		view.Directions = []Direction{DirectionNorth, DirectionSouth, DirectionEast, DirectionWest}
	}
	return view, nil
}
