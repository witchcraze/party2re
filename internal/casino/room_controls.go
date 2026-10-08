package casino

// RoomControls describes candidates from an authorized room snapshot. Commands
// still recheck admission, phase, balance and other prerequisites when executed.
type RoomControls struct {
	Role        string   `json:"role"`
	CanStart    bool     `json:"can_start"`
	Actions     []string `json:"actions"`
	KickTargets []string `json:"kick_targets"`
}

func (v RoomView) Controls(characterID string) (RoomControls, error) {
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
		if v.Room.GameType != GameTypeDoppel && m.Action != "" && m.Action != "待機中" {
			return c, nil
		}
		switch v.Room.GameType {
		case GameTypeIndian:
			c.Actions = []string{string(ActionCall), string(ActionShowdown), string(ActionFold)}
		case GameTypeHighLow:
			c.Actions = []string{string(HighLowActionCall), string(HighLowActionHigh)}
			if v.Room.ParticipantCount > 2 {
				c.Actions = append(c.Actions, string(HighLowActionLow))
			}
			c.Actions = append(c.Actions, string(HighLowActionFold))
		case GameTypeDoppel:
			for _, mark := range AuthenticDoppelMarks[:min(v.Room.ParticipantCount+1, len(AuthenticDoppelMarks))] {
				c.Actions = append(c.Actions, string(mark))
			}
		}
		return c, nil
	}
	return c, ErrRoomViewForbidden
}
