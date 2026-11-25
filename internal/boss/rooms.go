package boss

import (
	"context"
	"errors"
	"fmt"
	"strings"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/party"
)

const cloneStageID = "king99"

// OpenSealingRooms opens a system-owned room for every king stage that has
// none and publishes the unseal news (vs_monster.cgi make_vs_king). It runs
// after the unseal commits, so a failure never undoes that settlement.
// unsealers is the unsealing party at the treasure creation, leader first.
func (s *Service) OpenSealingRooms(ctx context.Context, partyName string, unsealers []corebattle.LifetimeActor) error {
	if s.partyRepo == nil {
		return errors.New("party repository not configured")
	}
	if len(unsealers) == 0 {
		return errors.New("no unsealing party")
	}
	now := s.now()
	var errs []error
	for _, stage := range s.stages {
		leader := "@" + stage.LeaderName
		if stage.ID == cloneStageID {
			leader = "@" + unsealers[0].Name
		}
		room := party.Party{
			ID: id.New(), Kind: party.KindSealing, LeaderName: leader, Name: stage.Name, StageID: stage.ID,
			Speed: stage.Speed, MaxMembers: stage.MaxMembers, NeedJoin: stage.NeedJoin, Status: party.StatusRecruiting,
			CreatedAt: now, UpdatedAt: now, Enemies: stageRoster(stage, unsealers),
		}
		if _, err := s.partyRepo.OpenSystemRoom(ctx, room); err != nil {
			errs = append(errs, fmt.Errorf("open %s room: %w", stage.ID, err))
		}
	}
	if s.newsPub != nil {
		names := make([]string, len(unsealers))
		for i, a := range unsealers {
			names[i] = a.Name
		}
		msg := fmt.Sprintf("%s(%s) によって封印されし者達の封印が解かれました！", partyName, strings.Join(names, "、"))
		if err := s.newsPub.PublishNews(ctx, "boss", msg, msg, "System", now); err != nil {
			errs = append(errs, fmt.Errorf("publish unseal news: %w", err))
		}
	}
	return errors.Join(errs...)
}

// stageRoster is a room's initial enemies: the catalog bosses, or for king99
// a clone of each unsealer (stage/king99.cgi).
func stageRoster(stage BossStage, unsealers []corebattle.LifetimeActor) []corebattle.LifetimeActor {
	if stage.ID == cloneStageID {
		roster := make([]corebattle.LifetimeActor, len(unsealers))
		for i, a := range unsealers {
			roster[i] = king99Clone(i, a)
		}
		return roster
	}
	roster := make([]corebattle.LifetimeActor, len(stage.Bosses))
	for i, b := range stage.Bosses {
		var abilities []string
		if b.TMP != "" {
			abilities = []string{b.TMP}
		}
		sp, oldSP := b.SP, b.OldSP
		if sp <= 0 && b.Job > 0 {
			sp = 999
		}
		if oldSP <= 0 && b.OldJob > 0 {
			oldSP = 999
		}
		roster[i] = corebattle.LifetimeActor{
			Participant: corebattle.Participant{
				ID: fmt.Sprintf("%s-boss-%d", stage.ID, i), Name: b.Name, HP: b.HP, MaxHP: b.MaxHP, MP: b.MP, MaxMP: b.MaxMP,
				Attack: b.Attack, Defense: b.Defense, Agility: b.Agility, Abilities: abilities, Icon: b.Icon,
				Skills:           enemySkills(b.Job, sp, b.OldJob, oldSP),
				ExperienceReward: b.GetExp, GoldReward: b.GetMoney,
			},
			NPC: true, CrystalReward: b.GetCrystal,
		}
	}
	return roster
}

// king99Clone copies an unsealer's battle row (stage/king99.cgi).
func king99Clone(i int, a corebattle.LifetimeActor) corebattle.LifetimeActor {
	hp, mp := a.MaxHP*50, a.MaxMP*50
	return corebattle.LifetimeActor{
		Participant: corebattle.Participant{
			ID: fmt.Sprintf("king99-clone-%d", i), Name: "@" + a.Name, HP: hp, MaxHP: hp, MP: mp, MaxMP: mp,
			Attack: a.Attack * 2, Defense: a.Defense * 2, Agility: a.Agility * 2, Abilities: []string{"大防御"}, Icon: a.Icon,
			Skills:           enemySkills(parseJobID(a.Jobs.JobID), a.Jobs.SP, parseJobID(a.Jobs.OldJobID), a.Jobs.OldSP),
			ExperienceReward: a.ExperienceReward * 30, GoldReward: a.GoldReward * 30,
		},
		Jobs: a.Jobs, NPC: true, CrystalReward: 20,
	}
}

func enemySkills(job, sp, oldJob, oldSP int) []corebattle.ActionSkill {
	skills := []corebattle.ActionSkill{{ID: "dejon", Name: "デジョン", Kind: corebattle.ActionKindDejon}}
	skills = append(skills, bossSkillsForJob(job, sp)...)
	return append(skills, bossSkillsForJob(oldJob, oldSP)...)
}
