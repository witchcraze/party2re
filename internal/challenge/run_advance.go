package challenge

import (
	"context"
	"fmt"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/runhost"
)

// advance is susumu (vs_challenge.cgi:36–55).
func (s *RunService) advance(ctx context.Context, r *Run, actorID string) (runhost.Effects, error) {
	a := r.Actor(actorID)
	if a == nil || a.HP <= 0 {
		return runhost.Effects{}, ErrActorFallen
	}
	if r.LivingEnemies() {
		return runhost.Effects{}, ErrEnemiesRemain
	}
	if r.Round < 1 && r.LeaderID != actorID {
		return runhost.Effects{}, ErrLeaderOnly
	}
	requester, err := s.cfg.Characters.FindByID(ctx, actorID)
	if err != nil {
		return runhost.Effects{}, err
	}
	effects := runhost.Effects{}
	if r.Round > r.Record {
		// update_record snapshots the members before the reset.
		entry := s.recordEntry(r, a.TeamID, requester)
		effects.Settled = func(ctx context.Context) error {
			raised, err := s.cfg.HallOfFame.RaiseHallOfFame(ctx, entry)
			if raised {
				r.Record = entry.HighestRound
			}
			return err
		}
	}
	reset, err := r.ResetContext(actorID, &requester.Level)
	if err != nil {
		return runhost.Effects{}, err
	}
	if err := battle.ResetAll(&r.Battle, &reset, s.cfg.RNG); err != nil {
		return runhost.Effects{}, err
	}
	r.Battle.Fields.ResetFloor()
	r.DiscardEncounter()
	r.Round++
	return effects, s.spawn(ctx, r, actorID, a.TeamID)
}

// recordEntry is update_record's line for the advancing actor's party
// (vs_challenge.cgi:80–84); a fallen member shows the fallen icon.
func (s *RunService) recordEntry(r *Run, team string, requester corecharacter.Character) HallOfFameEntry {
	entry := HallOfFameEntry{TierID: r.Admission.StageID, HighestRound: r.Round, PartyName: r.Admission.Name, PartyColor: requester.Color, ClearedAt: s.cfg.Clock.Now()}
	for _, m := range r.Battle.Actors {
		if m.TeamID != team {
			continue
		}
		icon := m.Icon
		if m.HP <= 0 {
			icon = fallenIcon
		}
		entry.Members = append(entry.Members, HallOfFameMember{CharacterID: m.ID, CharacterName: m.Name, Icon: icon, JobID: m.Jobs.JobID, OldJobID: m.Jobs.OldJobID, HP: m.HP, MP: m.MP, Attack: m.Attack, Defense: m.Defense, Agility: m.Agility})
	}
	return entry
}

// fallenIcon is the record icon of a fallen member.
const fallenIcon = "chr/099.gif"

// spawn is set_monster (vs_challenge.cgi:57–70). Loading the stage file draws
// the treasure threshold, then a random job once for the whole wave; the
// treasure room needs the advancing actor's taken-treasure flag ($m{is_get})
// unset, which a new run starts with (quest.cgi:1169).
func (s *RunService) spawn(ctx context.Context, r *Run, actorID, team string) error {
	tier := s.cfg.Tiers[r.Admission.StageID]
	rng := s.cfg.RNG
	party, alive := 0, 0
	for _, a := range r.Battle.Actors {
		if a.TeamID == team && !a.Banished && !a.RosterDetached {
			party++
			if a.HP > 0 {
				alive++
			}
		}
	}
	threshold := tier.TreasureRound + int(rng.Float64()*float64(max(tier.TreasureRoundMax-tier.TreasureRound+1, 1)))
	jobs := make([]string, len(tier.Monsters))
	for i, m := range tier.Monsters {
		if m.RandomJob > 0 {
			jobs[i] = legacyJobID(rng.Intn(m.RandomJob))
		}
	}
	if r.Recipients[actorID].Inspected == 0 && r.Round >= threshold && rng.Float64()*10 < 1 {
		blessing := ""
		if s.cfg.Blessings != nil {
			var err error
			if blessing, err = s.cfg.Blessings.GetActiveBlessing(ctx, actorID); err != nil {
				return fmt.Errorf("treasure blessing: %w", err)
			}
		}
		count := runhost.TreasureCount(0, party, party-alive, runhost.IsTreasureBlessing(blessing), rng.Intn)
		r.Boxes = runhost.GenerateTreasureBoxes(count, runhost.TreasurePools{Items: tier.TreasureItemPool}, s.cfg.Clock.Now(), rng.Intn)
		return nil
	}
	// add_monster(0, 1 + 0.1 × round) (_npc_action.cgi:635–680).
	strength := 1 + 0.1*float64(r.Round)
	count := int(rng.Float64()*(2.5+float64(party)*0.6)) + 1
	for i := range count {
		no := int(rng.Float64() * float64(len(tier.Monsters)))
		m := tier.Monsters[no]
		a := m.Combat.Clone()
		if jobs[no] != "" {
			a.Jobs.JobID = jobs[no]
		}
		base := [6]int{m.HP, m.MP, m.Attack, m.Defense, m.Agility, a.CMP}
		var v [6]int
		for k, b := range base {
			v[k] = int(float64(b) * (0.9 + rng.Float64()*0.3 + float64(party-2)*0.1) * strength)
		}
		a.ID = fmt.Sprintf("%s-r%d-v%d-%d", r.ID, r.Round, r.Version+1, i)
		a.Name = m.Name + string(rune('A'+i))
		a.EncounterIdentity = fmt.Sprintf("challenge-%s-%d", tier.ID, no) + string(rune('A'+i))
		a.HP, a.MP, a.Attack, a.Defense, a.Agility, a.CMP = v[0], v[1], v[2], v[3], v[4], v[5]
		a.MaxHP = a.HP
		if a.MaxMP == 0 {
			a.MaxMP = a.MP
		}
		if a.MaxCMP == 0 {
			a.MaxCMP = a.CMP
		}
		a.BaseStats.Attack, a.BaseStats.Defense, a.BaseStats.Agility = a.Attack, a.Defense, a.Agility
		a.ExperienceReward, a.GoldReward = m.ExperienceReward, m.GoldReward
		a.NPC, a.TeamID = true, "enemies"
		if a.Hit == 0 {
			a.Hit = 95
		}
		if a.Tension == 0 {
			a.Tension = 1
		}
		if a.Gear == nil {
			a.Gear = &corebattle.BattleGearStats{}
		}
		a.OriginalJobs = a.Jobs
		r.Battle.Actors = append(r.Battle.Actors, a)
		r.Enemies = append(r.Enemies, a.ID)
	}
	return nil
}

// legacyJobID names legacy job number n.
func legacyJobID(n int) string {
	if n == 0 {
		return "starter"
	}
	return fmt.Sprintf("job-%02d", n)
}
