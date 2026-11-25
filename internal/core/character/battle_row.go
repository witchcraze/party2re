package character

// BattleRowRewards are the EXP and gold a player's battle row carries when it
// enters a room (quest.cgi:1177–1178).
func (c Character) BattleRowRewards() (exp, gold int) {
	return c.Level + c.JobLevel, c.Level / 2
}
