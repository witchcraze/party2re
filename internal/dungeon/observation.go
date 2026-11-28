package dungeon

import "context"

// ContinuationControls describe service-owned eligibility, not command wiring.
type ContinuationControls struct {
	CanMove   bool `json:"can_move"`
	CanFight  bool `json:"can_fight"`
	CanEscape bool `json:"can_escape"`
	CanMine   bool `json:"can_mine"`
}

// Any living member may move or mine; combat and Escape stay with the
// leader. An all-fallen party can only Escape.
func (exp ActiveExpedition) Controls(actor string) ContinuationControls {
	canMove := exp.Status == StatusExploring && exp.isAlive(actor) && !exp.Encounter.blocking() && exp.Round < exp.RoundLimit
	canMine := exp.canMine(actor)
	if exp.CharacterID != actor {
		return ContinuationControls{CanMove: canMove, CanMine: canMine}
	}
	switch exp.Status {
	case StatusExploring:
		return ContinuationControls{CanMove: canMove, CanFight: exp.Encounter.hostile() && exp.anyAlive(), CanEscape: true, CanMine: canMine}
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
	FloorName  string               `json:"floor_name,omitempty"`
	Map        *MapView             `json:"map"`
	Directions []Direction          `json:"directions"`
	Controls   ContinuationControls `json:"controls"`
	// ChannelTarget is shared by every member, as the legacy party log was.
	ChannelTarget *ChannelTarget `json:"channel_target,omitempty"`
}

// ChannelTarget is the channel combination the altar last disclosed (map13).
type ChannelTarget struct {
	Channels      []string `json:"channels"`
	RerolledSince bool     `json:"rerolled_since"`
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
	d, floor, err := s.activeFloor(exp)
	if err != nil {
		return nil, err
	}
	m, err := s.viewMap(ctx, actor, exp, floor)
	if err != nil {
		return nil, err
	}
	view.Map = &m
	if d.disclosure != nil {
		if view.ChannelTarget, err = d.disclosure(exp); err != nil {
			return nil, err
		}
	}
	// A pitfall to a floor with a lower limit can leave the round above it.
	view.Round, view.RoundLimit, view.Floor, view.FloorName = exp.Round, exp.RoundLimit, floor.floorNumber(), d.floorName(floor)
	// Legacy registers all four directions; walls are rejected on use, so
	// the list cannot reveal a hidden passage.
	if view.Controls.CanMove {
		view.Directions = []Direction{DirectionNorth, DirectionSouth, DirectionEast, DirectionWest}
	}
	return view, nil
}
