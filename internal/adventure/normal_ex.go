package adventure

// HasEX reads the room marker against the current destination, never clear credit.
func (r *NormalRun) HasEX() bool {
	return r.EXUnlocked && (r.Stage.ID == "stage-22" || r.Stage.ID == "stage-23" || r.Stage.ID == "stage-24" || r.Stage.ID == "stage-25" || r.Stage.ID == "stage-26")
}

func (r *NormalRun) markEXOnAdvance(actor string) {
	if r.Floor != BossFloor {
		return
	}
	count := 0
	for _, a := range r.Battle.Actors {
		if a.TeamID == r.actor(actor).TeamID && !a.Banished && !a.RosterDetached {
			count++
		}
	}
	if r.Stage.ID == "stage-25" || r.Stage.ID == "stage-23" && count > 1 {
		r.EXUnlocked = true
	}
}

func (r *NormalRun) markEncounterCreated() {
	r.EncounterFloor, r.EncounterStageID = r.Floor, r.Stage.ID
	if r.Floor == BossFloor {
		r.ClearPending = true
		if r.Stage.ID == "stage-24" {
			r.EXUnlocked = true
		}
	}
}
