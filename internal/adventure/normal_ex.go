package adventure

import (
	"fmt"
	"slices"
	"strings"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
	"github.com/witchcraze/party2re/internal/runhost"
)

// EX room-marker producers (StageTraits.EX.Marker).
const (
	markerRoyalBeeDefeat   = "royal_bee_defeat"
	markerAdvanceTwoAllies = "advance_two_allies"
	markerBossEncounter    = "boss_encounter"
	markerAdvance          = "advance"
	markerRoomCreation     = "room_creation"
)

func (s Stage) exMarker() string {
	if s.EX == nil {
		return ""
	}
	return s.EX.Marker
}

// HasEX reads the room marker against the current destination, never clear credit.
func (r *NormalRun) HasEX() bool {
	return r.EXUnlocked && r.Stage.EX != nil
}

func (r *NormalRun) markEXOnAdvance(actor string) {
	if r.Round != BossFloor {
		return
	}
	count := 0
	for _, a := range r.Battle.Actors {
		if a.TeamID == r.Actor(actor).TeamID && !a.Banished && !a.RosterDetached {
			count++
		}
	}
	if m := r.Stage.exMarker(); m == markerAdvance || m == markerAdvanceTwoAllies && count > 1 {
		r.EXUnlocked = true
	}
}

func (r *NormalRun) markEncounterCreated() {
	r.EncounterFloor, r.EncounterStageID = r.Round, r.Stage.ID
	if r.Round == BossFloor {
		r.ClearPending = true
		if r.Stage.exMarker() == markerBossEncounter {
			r.EXUnlocked = true
		}
	}
}

// royalBeeIdentity is the server-owned stage22 source entry (ff97fc5:stage/22.cgi:175).
const royalBeeIdentity = "ordinary-22-monster-175"

func rollRoyalBeeEX(stage Stage, identity string, rng random.Generator) bool {
	return stage.exMarker() == markerRoyalBeeDefeat && strings.TrimRight(identity, "ABCDEFGH") == royalBeeIdentity && rng.Intn(3) == 0
}

// prepareEXAllies follows add_ex_boss: after the roster exists, every ally,
// including helpers and fallen members, regains current maxima, then the
// declared default field is installed (_npc_action.cgi:715–722).
func (r *NormalRun) prepareEXAllies(actor string) {
	team := r.Actor(actor).TeamID
	for i := range r.Battle.Actors {
		if a := &r.Battle.Actors[i]; a.TeamID == team && !a.Banished && !a.RosterDetached {
			a.HP, a.MP, a.CMP = a.MaxHP, a.MaxMP, a.MaxCMP
		}
	}
	if r.Stage.Ordinary.EXField != "" {
		r.Battle.Fields.Default = r.Stage.Ordinary.EXField
	}
}

func isTreasureFloor(floor int) bool { return floor == TreasureRoomFloor || floor == EXTreasureFloor }

func (s Stage) exTreasureItems() []string {
	if s.EX == nil {
		return nil
	}
	return s.EX.TreasureItems
}

// treasureCount mirrors @partys at creation: same-side members, fallen roster
// members included (_battle.cgi:486–493). real counts only roster members.
func (r *NormalRun) treasureCount(actorID, blessing string, rng func(int) int) (count, real int) {
	a := r.Actor(actorID)
	alive, total := 0, 0
	for _, m := range r.Battle.Actors {
		inRoster := slices.Contains(r.Roster, m.ID)
		if m.TeamID != a.TeamID || m.Banished || m.RosterDetached {
			continue
		}
		total++
		if m.HP > 0 {
			alive++
		}
		if inRoster {
			real++
		}
	}
	return CalculateTreasureCount(TreasureCalculationInput{TripleTreasure: r.Stage.TripleTreasure, AliveMembers: alive, DeadMembers: total - alive, TotalMembers: total, HasMerchant: a.Jobs.JobID == merchantJob, HasGambler: a.Jobs.JobID == gamblerJob, HasTreasureHunter: a.Jobs.JobID == treasureHunterJob, HasLuckyPendant: ParticipantHasLuckyPendant(a.Participant), HasTreasureBlessing: IsTreasureBlessing(blessing), Rng: rng}), real
}

var fixedBoxNames = []string{"煤けた宝箱", "金の宝箱"}

// createEXTreasure follows add_ex_treasure then add_fixed_treasure
// (_npc_action.cgi:555–598,725–777): the ordinary allowance resets for every
// roster member, and one fixed chest exists per real member, fallen included.
// No Try conversion, clear credit or unseal event happens here.
func (s *NormalRunService) createEXTreasure(r *NormalRun, actorID, blessing string) error {
	items := r.Stage.exTreasureItems()
	if len(items) == 0 {
		return corebattle.ErrUnsupportedEffect
	}
	count, real := r.treasureCount(actorID, blessing, s.cfg.RNG.Intn)
	for _, id := range r.Roster {
		if p, ok := r.Recipients[id]; ok {
			p.Inspected = 0
			r.Recipients[id] = p
		}
	}
	r.Boxes = nil
	for i := range count {
		name := boxName(defaultBoxNames[s.cfg.RNG.Intn(len(defaultBoxNames))], i)
		r.Boxes = append(r.Boxes, runhost.TreasureBox{ID: fmt.Sprintf("box-%d", i+1), Name: name, ItemID: items[s.cfg.RNG.Intn(len(items))]})
	}
	if fixed := r.Stage.EX.FixedTreasure; fixed != "" {
		for i := range real {
			name := boxName(fixedBoxNames[s.cfg.RNG.Intn(len(fixedBoxNames))], i)
			r.Boxes = append(r.Boxes, runhost.TreasureBox{ID: fmt.Sprintf("box-%d", len(r.Boxes)+1), Name: name, ItemID: fixed, Fixed: true})
		}
	}
	r.EncounterFloor, r.EncounterStageID = r.Round, r.Stage.ID
	return nil
}
