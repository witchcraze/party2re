package adventure

import (
	"context"
	"fmt"
	"slices"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/runhost"
)

func (s *NormalRunService) advance(ctx context.Context, r *NormalRun, actorID string) error {
	a := r.Actor(actorID)
	if a == nil || a.HP <= 0 {
		return ErrUnconsciousCharacter
	}
	if r.Round == 0 && r.LeaderID != actorID {
		return ErrLeaderOnly
	}
	switch {
	case r.LivingEnemies():
		return ErrInvalidFloorAction
	case r.Round < TreasureRoomFloor:
	case !r.HasEX() || r.Round > EXBossFloor: // ordinary end at 11, terminal 13
		return ErrInvalidFloorAction
	case r.Round == EXBossFloor && len(r.Stage.exTreasureItems()) == 0,
		r.Round == TreasureRoomFloor && (r.Stage.Ordinary == nil || len(r.Stage.Ordinary.EXRoster) == 0):
		return corebattle.ErrUnsupportedEffect
	}
	// Required reads precede reset, random draws, consumption and publication.
	blessing := ""
	if s.cfg.Blessings != nil {
		var err error
		blessing, err = s.cfg.Blessings.GetActiveBlessing(ctx, actorID)
		if err != nil {
			return fmt.Errorf("advance blessing: %w", err)
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// The all-member reset with item188's revival pass, then the floor field
	// reset that keeps the default field (vs_monster.cgi susumu).
	reset, err := s.resetContext(ctx, r, actorID)
	if err != nil {
		return err
	}
	if err := battle.ResetAll(&r.Battle, &reset, s.cfg.RNG); err != nil {
		return err
	}
	r.Battle.Fields.ResetFloor()
	// Old encounter actors do not survive creation; coordinates remain separate
	// from encounter provenance, so travel cannot manufacture boss clear credit.
	r.DiscardEncounter()
	r.EncounterPool, r.AppearancePositions = nil, nil
	r.markEXOnAdvance(actorID)
	r.Round++
	switch r.Round {
	case TreasureRoomFloor:
		return s.createTreasureEncounter(r, actorID, blessing)
	case EXTreasureFloor:
		return s.createEXTreasure(r, actorID, blessing)
	}
	return s.createEncounter(ctx, r, actorID)
}

func (s *NormalRunService) createEncounter(ctx context.Context, r *NormalRun, actorID string) error {
	if r.Stage.Ordinary != nil {
		party := make([]corebattle.LifetimeActor, 0, len(r.Battle.Actors))
		for _, a := range r.Battle.Actors {
			if a.TeamID == r.Actor(actorID).TeamID && !a.Banished && !a.RosterDetached {
				party = append(party, a)
			}
		}
		encounter, err := r.Stage.BuildOrdinaryEncounter(s.cfg.Monsters, party, *r.Actor(actorID), r.Round, r.Mode, fmt.Sprintf("%s-f%d-v%d", r.ID, r.Round, r.Version+1), s.cfg.RNG)
		if err != nil {
			return err
		}
		r.EncounterPool, r.AppearancePositions = encounter.Pool, encounter.Appears
		r.Battle.Actors = append(r.Battle.Actors, encounter.Actors...)
		for _, a := range encounter.Actors {
			r.Enemies = append(r.Enemies, a.ID)
		}
		if r.Round == EXBossFloor {
			r.prepareEXAllies(actorID)
		}
		// Restoration precedes the cap, which leaves current HP/MP untouched (_battle.cgi:1485–1530).
		if slices.Contains(r.Stage.SpecialBossFloors, r.Round) {
			reset, err := s.resetContext(ctx, r, actorID)
			if err != nil {
				return err
			}
			if err := r.Battle.ApplySpecialBossField(&reset, s.cfg.RNG); err != nil {
				return err
			}
			if !r.Battle.SpecialBossField {
				return corebattle.ErrUnsupportedEffect
			}
		}
		r.markEncounterCreated()
		return nil
	}
	ids := r.Stage.GetBossIDs()
	if r.Round < BossFloor {
		pool := r.Stage.GetNormalMonsterIDs()
		if len(pool) == 0 {
			return ErrStageNotFound
		}
		count := int(s.cfg.RNG.Float64()*(2.5+float64(len(r.Roster))*0.6)) + 1
		ids = make([]string, count)
		for i := range ids {
			ids[i] = pool[s.cfg.RNG.Intn(len(pool))]
		}
	}
	if len(ids) == 0 {
		return ErrStageNotFound
	}
	session := CrawlSession{Mode: r.Mode, Rng: s.cfg.RNG.Intn, RngFloat: s.cfg.RNG.Float64}
	for i, id := range ids {
		m, err := s.cfg.Monsters.FindByID(id)
		if err != nil {
			return err
		}
		a := session.buildMonsterActor(fmt.Sprintf("%s-f%d-v%d-%d", m.ID, r.Round, r.Version+1, i), m, float64(len(r.Roster)-2)*0.1, r.Round == BossFloor)
		a.TeamID = "enemies"
		r.Battle.Actors = append(r.Battle.Actors, a)
		r.Enemies = append(r.Enemies, a.ID)
	}
	r.markEncounterCreated()
	return nil
}

// createTreasureEncounter is shared by advance and job-driven room recreation.
func (s *NormalRunService) createTreasureEncounter(r *NormalRun, actorID, blessing string) error {
	count, _ := r.treasureCount(actorID, blessing, s.cfg.RNG.Intn)
	r.Boxes = runhost.GenerateTreasureBoxes(count, runhost.TreasurePools{Weapons: r.Stage.TreasureWeapons, Armors: r.Stage.TreasureArmors, Items: r.Stage.TreasureItems}, s.cfg.Clock.Now(), s.cfg.RNG.Intn)
	if r.Mode == CrawlModeTry {
		for _, id := range r.Roster {
			p := r.Recipients[id]
			converted, err := scaleTryEarnings(p.Conditional)
			if err != nil {
				return err
			}
			p.Pending, err = runhost.AddEarnings(p.Pending, converted)
			if err != nil {
				return err
			}
			p.Conditional = runhost.Earnings{}
			r.Recipients[id] = p
		}
		r.Mode = CrawlModeNormal
	}
	if r.ClearPending {
		r.StageCleared, r.ClearPending = true, false
	}
	r.EncounterFloor, r.EncounterStageID = r.Round, r.Stage.ID
	return nil
}

// resetContext is the all-member reset frame with the requester's saved level.
func (s *NormalRunService) resetContext(ctx context.Context, r *NormalRun, actorID string) (corebattle.EquipmentResetContext, error) {
	requester, err := s.cfg.Characters.FindByID(ctx, actorID)
	if err != nil {
		return corebattle.EquipmentResetContext{}, err
	}
	return r.ResetContext(actorID, &requester.Level)
}
