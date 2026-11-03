package adventure

import (
	"context"
	"fmt"
	"slices"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func (s *NormalRunService) advance(ctx context.Context, r *NormalRun, actorID string) error {
	a := r.actor(actorID)
	if a == nil || a.HP <= 0 {
		return ErrUnconsciousCharacter
	}
	if r.Floor == 0 && r.LeaderID != actorID {
		return ErrLeaderOnly
	}
	if r.Floor >= TreasureRoomFloor || r.livingEnemies() {
		return ErrInvalidFloorAction
	}
	if s.cfg.Combat == nil {
		return ErrRunUnavailable
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
	if err := s.cfg.Combat.PrepareAdvance(ctx, r, actorID); err != nil {
		return err
	}
	// Old encounter actors do not survive creation; coordinates remain separate
	// from encounter provenance, so travel cannot manufacture boss clear credit.
	r.Battle.Actors = slices.DeleteFunc(r.Battle.Actors, func(a corebattle.LifetimeActor) bool { return slices.Contains(r.Enemies, a.ID) })
	r.Enemies = nil
	r.Floor++
	if r.Floor == TreasureRoomFloor {
		alive := 0
		for _, id := range r.Roster {
			if p := r.actor(id); p != nil && p.HP > 0 {
				alive++
			}
		}
		a = r.actor(actorID)
		count := CalculateTreasureCount(TreasureCalculationInput{StageID: r.Stage.ID, AliveMembers: alive, DeadMembers: len(r.Roster) - alive, TotalMembers: len(r.Roster), HasMerchant: IsMerchantJob(a.Jobs.JobID), HasGambler: IsGamblerJob(a.Jobs.JobID), HasTreasureHunter: IsTreasureHunterJob(a.Jobs.JobID), HasLuckyPendant: ParticipantHasLuckyPendant(a.Participant), HasTreasureBlessing: IsTreasureBlessing(blessing), Rng: s.cfg.RNG.Intn})
		r.Boxes = GenerateTreasureBoxesWithPools(count, StageTreasurePools{Weapons: r.Stage.TreasureWeapons, Armors: r.Stage.TreasureArmors, Items: r.Stage.TreasureItems}, s.cfg.Clock.Now(), s.cfg.RNG.Intn)
		if r.Mode == CrawlModeTry {
			for _, id := range r.Roster {
				p := r.Recipients[id]
				converted, err := scaleTryEarnings(p.Conditional)
				if err != nil {
					return err
				}
				p.Pending, err = addRunEarnings(p.Pending, converted)
				if err != nil {
					return err
				}
				p.Conditional = RunEarnings{}
				r.Recipients[id] = p
			}
			r.Mode = CrawlModeNormal
		}
		return nil
	}
	ids := r.Stage.GetBossIDs()
	if r.Floor < BossFloor {
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
		p := session.buildMonsterParticipant(fmt.Sprintf("%s-f%d-v%d-%d", m.ID, r.Floor, r.Version+1, i), m, float64(len(r.Roster)-2)*0.1, r.Floor == BossFloor)
		p.TeamID = "enemies"
		r.Battle.Actors = append(r.Battle.Actors, corebattle.LifetimeActor{Participant: p, NPC: true, Tension: 1, Hit: 95})
		r.Enemies = append(r.Enemies, p.ID)
	}
	r.EncounterFloor = r.Floor
	return nil
}

func (s *NormalRunService) claimChest(ctx context.Context, r *NormalRun, actorID, boxID string) (*TreasureBox, map[string][]corebattle.ConsumedItem, error) {
	if r.Floor != TreasureRoomFloor {
		return nil, nil, ErrInvalidFloorAction
	}
	a := r.actor(actorID)
	if a == nil || a.HP <= 0 {
		return nil, nil, ErrUnconsciousCharacter
	}
	i := slices.IndexFunc(r.Boxes, func(b TreasureBox) bool { return b.ID == boxID })
	if i < 0 || r.Boxes[i].OpenedBy != "" {
		return nil, nil, ErrNoUnopenedTreasure
	}
	p := r.Recipients[actorID]
	consumed := map[string][]corebattle.ConsumedItem{}
	if p.Inspected > 0 {
		if s.cfg.Inventories == nil {
			return nil, nil, ErrRunUnavailable
		}
		inv, err := s.cfg.Inventories.FindByCharacterID(ctx, actorID)
		if err != nil {
			return nil, nil, err
		}
		for _, item := range inv.Items {
			if item.DefinitionID == ItemTreasurePocket && item.Quantity > 0 {
				entry := corebattle.ConsumedItem{ID: item.DefinitionID, InstanceID: item.ID, Quantity: 1}
				consumed[actorID] = []corebattle.ConsumedItem{entry}
				removeHeldItem(&a.Participant, entry)
				break
			}
		}
		if len(consumed[actorID]) == 0 {
			return nil, nil, ErrTreasureLimitReached
		}
	}
	p.Inspected++
	r.Recipients[actorID] = p
	r.Boxes[i].OpenedBy = actorID
	return &r.Boxes[i], consumed, nil
}
