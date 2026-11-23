package casino

// RoomControls describes candidates from an authorized room snapshot. Commands
// still recheck admission, phase, balance and other prerequisites when executed.
type RoomControls struct {
	Role        string   `json:"role"`
	CanStart    bool     `json:"can_start"`
	Actions     []string `json:"actions"`
	KickTargets []string `json:"kick_targets"`
}

// coins is the actor's balance, which gates the High & Low call.
func (v RoomView) Controls(characterID string, coins int64) (RoomControls, error) {
	c := RoomControls{Role: "member", Actions: []string{}, KickTargets: []string{}}
	for _, m := range v.Members {
		if m.CharacterID != characterID {
			continue
		}
		if m.IsSpectator {
			c.Role = "spectator"
			return c, nil
		}
		if v.Room.LeaderCharacterID == characterID {
			c.Role = "leader"
			if v.Room.Round == 0 && v.Room.Status == RoomStatusWaiting {
				c.CanStart = true
				for _, target := range v.Members {
					if target.CharacterID != characterID {
						c.KickTargets = append(c.KickTargets, target.CharacterID)
					}
				}
			}
		}
		if v.Room.Round <= 0 || v.Room.Status != RoomStatusInProgress {
			return c, nil
		}
		// A mid-game entrant (待機中) sits out; Doppel keeps marks open until all select.
		if m.Action == "待機中" || (v.Room.GameType != GameTypeDoppel && m.Action != "") {
			return c, nil
		}
		switch v.Room.GameType {
		case GameTypeIndian:
			c.Actions = []string{string(ActionCall), string(ActionShowdown), string(ActionFold)}
		case GameTypeHighLow:
			if highLowCanCall(v.Room.CurrentBet, v.Room.MaxBet, coins) {
				c.Actions = append(c.Actions, string(HighLowActionCall))
			}
			c.Actions = append(c.Actions, string(HighLowActionHigh))
			if v.Room.ParticipantCount > 2 {
				c.Actions = append(c.Actions, string(HighLowActionLow))
			}
			c.Actions = append(c.Actions, string(HighLowActionFold))
		case GameTypeDoppel:
			players := v.Room.ParticipantCount
			for _, p := range v.Members {
				if !p.IsSpectator && p.Action == "待機中" {
					players--
				}
			}
			for _, mark := range AuthenticDoppelMarks[:min(players+1, len(AuthenticDoppelMarks))] {
				c.Actions = append(c.Actions, string(mark))
			}
		}
		return c, nil
	}
	return c, ErrRoomViewForbidden
}
