package adventure

import (
	"strings"

	"github.com/witchcraze/party2re/internal/core/random"
)

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

// royalBeeIdentity is the server-owned stage22 source entry (ff97fc5:stage/22.cgi:175).
const royalBeeIdentity = "ordinary-22-monster-175"

// recordCreditedDefeats applies _battle.cgi:277–280 once per player-credited
// defeat, in defeat order. The draw happens even when the marker already exists.
func (r *NormalRun) recordCreditedDefeats(identities []string, rng random.Generator) {
	if r.Stage.ID != "stage-22" {
		return
	}
	for _, identity := range identities {
		// Encounter suffixes A–H keep the catalog identity (add_monster @alfas).
		if strings.TrimRight(identity, "ABCDEFGH") == royalBeeIdentity && rng.Intn(3) == 0 {
			r.EXUnlocked = true
		}
	}
}

// prepareEXAllies follows add_ex_boss: after the roster exists, every ally,
// including helpers and fallen members, regains current maxima, then the
// declared default field is installed (_npc_action.cgi:715–722).
func (r *NormalRun) prepareEXAllies(actor string) {
	team := r.actor(actor).TeamID
	for i := range r.Battle.Actors {
		if a := &r.Battle.Actors[i]; a.TeamID == team && !a.Banished && !a.RosterDetached {
			a.HP, a.MP, a.CMP = a.MaxHP, a.MaxMP, a.MaxCMP
		}
	}
	if r.Stage.Ordinary.EXField != "" {
		r.Battle.Fields.Default = r.Stage.Ordinary.EXField
	}
}
