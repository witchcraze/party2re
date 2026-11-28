package boss

import (
	"context"
	"errors"
	"fmt"
	"strings"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/runhost"
)

const cloneStageID = "king99"

// slot is a stage's live-room identity on the run host; a stage never has two rooms.
func slot(stageID string) string { return "sealing:" + stageID }

// OpenSealingRooms opens a system-owned room for every king stage that has
// none and publishes the unseal news (vs_monster.cgi make_vs_king). It runs
// after the unseal commits, so a failure never undoes that settlement.
// unsealers is the unsealing party at the treasure creation, leader first.
func (s *RunService) OpenSealingRooms(ctx context.Context, partyName string, unsealers []corebattle.LifetimeActor) error {
	if len(unsealers) == 0 {
		return errors.New("no unsealing party")
	}
	var errs []error
	for _, stage := range s.stages {
		roster, leader := stageRoster(stage, unsealers)
		a := runhost.Admission{StageID: stage.ID, Name: stage.Name, Speed: stage.Speed}
		if _, err := s.Open(ctx, slot(stage.ID), a, func(r *Run) {
			// make_vs_king writes round 1: the room is a running battle from the start.
			r.Round, r.Leader, r.Battle.Actors = 1, leader, roster
			// vs_king doubles created field durations (_battle.cgi:1615).
			r.Battle.Fields.DoubleTurns = true
			for _, e := range roster {
				r.Enemies = append(r.Enemies, e.ID)
			}
		}); err != nil {
			errs = append(errs, fmt.Errorf("open %s room: %w", stage.ID, err))
		}
	}
	if s.cfg.News != nil {
		names := make([]string, len(unsealers))
		for i, a := range unsealers {
			names[i] = a.Name
		}
		msg := fmt.Sprintf("%s(%s) によって封印されし者達の封印が解かれました！", partyName, strings.Join(names, "、"))
		if err := s.cfg.News.PublishNews(ctx, "boss", msg, msg, "System", s.cfg.Clock.Now()); err != nil {
			errs = append(errs, fmt.Errorf("publish unseal news: %w", err))
		}
	}
	return errors.Join(errs...)
}

// Rooms lists the live sealing rooms in stage order. Reading never changes a room;
// the scheduled idle sweep deletes an empty room idle for 30 minutes (quest.cgi:86–90).
func (s *RunService) Rooms(ctx context.Context) ([]Run, error) {
	var rooms []Run
	for _, stage := range s.stages {
		r, err := s.FindSlot(ctx, slot(stage.ID))
		if err != nil {
			return nil, err
		}
		if r != nil {
			rooms = append(rooms, *r)
		}
	}
	return rooms, nil
}

// stageRoster is a room's initial enemies and the leader boss among them:
// the stage's bosses, or for king99 a clone of each unsealer (stage/king99.cgi)
// led by the unsealing leader's clone.
func stageRoster(stage BossStage, unsealers []corebattle.LifetimeActor) ([]corebattle.LifetimeActor, string) {
	var roster []corebattle.LifetimeActor
	leader := ""
	if stage.ID == cloneStageID {
		for i, a := range unsealers {
			roster = append(roster, king99Clone(i, a))
		}
		return roster, roster[0].ID
	}
	for i, b := range stage.Bosses {
		a := b.actor(fmt.Sprintf("%s-boss-%d", stage.ID, i))
		if leader == "" && b.Name == stage.LeaderName {
			leader = a.ID
		}
		roster = append(roster, a)
	}
	return roster, leader
}

// king99Clone copies an unsealer's battle row (stage/king99.cgi).
func king99Clone(i int, a corebattle.LifetimeActor) corebattle.LifetimeActor {
	hp, mp := a.MaxHP*50, a.MaxMP*50
	at, df, ag := a.Attack*2, a.Defense*2, a.Agility*2
	return corebattle.LifetimeActor{
		Participant: corebattle.Participant{
			ID: fmt.Sprintf("king99-clone-%d", i), Name: a.Name, TeamID: "enemies", HP: hp, MaxHP: hp, MP: mp, MaxMP: mp,
			Attack: at, Defense: df, Agility: ag, BaseStats: corebattle.BattleBaseStats{Attack: at, Defense: df, Agility: ag},
			Gear: &corebattle.BattleGearStats{}, Icon: a.Icon,
			ExperienceReward: a.ExperienceReward * 30, GoldReward: a.GoldReward * 30,
		},
		Jobs: a.Jobs, OriginalJobs: a.Jobs, NPC: true, Hit: 95, Tension: 1, Temporary: corebattle.TemporaryStrongDefense,
		EncounterIdentity: fmt.Sprintf("king99-clone-%d", i), CrystalReward: 20,
	}
}
