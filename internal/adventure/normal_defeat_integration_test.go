package adventure_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/monster"
	"github.com/witchcraze/party2re/internal/runhost"
)

type normalDefeatCombat struct {
	adventure.RunCombat
	requester corecharacter.Character
	rng       *normalRNG
}

func (c normalDefeatCombat) ExecuteRunAction(ctx context.Context, r *adventure.NormalRun, cmd runhost.Command) (adventure.RunActionEffects, error) {
	if cmd.Action != "defeat" && cmd.Action != "reject-defeat" && cmd.Action != "bee" {
		return c.RunCombat.ExecuteRunAction(ctx, r, cmd)
	}
	target := corebattle.LifetimeActor{
		Participant: corebattle.Participant{ID: cmd.ID + "-enemy", TeamID: "enemies", Name: "Test Slime", HP: 500, MaxHP: 500, Gear: &corebattle.BattleGearStats{}, ExperienceReward: 3, GoldReward: 5},
		NPC:         true, EncounterIdentity: "monster-002",
	}
	if cmd.Action == "bee" {
		stages, err := adventure.InitialStageCatalog()
		if err != nil {
			return adventure.RunActionEffects{}, err
		}
		stage, err := stages.FindByID(cmd.TargetID)
		if err != nil {
			return adventure.RunActionEffects{}, err
		}
		r.Stage.ID, r.Stage.StageTraits = stage.ID, stage.StageTraits
		target.EncounterIdentity, target.Icon, target.CrystalReward = "ordinary-22-monster-175B", "upload/test", 1
	}
	r.Battle.Actors = append(r.Battle.Actors, target)
	r.Enemies = append(r.Enemies, target.ID)
	effects := adventure.RunActionEffects{RoomActivity: true, ActorActivity: true, ReleaseStock: true}
	defeat := &corebattle.DefeatContext{
		EquipmentResetContext: corebattle.EquipmentResetContext{TargetContext: corebattle.TargetContext{ActorID: cmd.ActorID, PartyIDs: r.Roster, EnemyIDs: r.Enemies}},
		Rewards:               adventure.NewRunDefeatRewards(r, c.requester, "", false, c.rng, &effects),
	}
	if _, err := r.Battle.ApplyDefeat(defeat, target.ID, c.rng); err != nil {
		return adventure.RunActionEffects{}, err
	}
	if cmd.Action == "reject-defeat" {
		return adventure.RunActionEffects{}, runhost.ErrUnavailable
	}
	return effects, nil
}

type explicitRunMonsterTamer struct{ svc *monster.Service }

func (t explicitRunMonsterTamer) TameMonster(ctx context.Context, characterID, monsterID, name string) error {
	_, err := t.svc.TameMonster(ctx, characterID, monsterID, name)
	if errors.Is(err, monster.ErrBoxFull) {
		return battle.ErrMonsterBoxFull
	}
	return err
}

func TestNormalRunDefeatRewardsSettleAndReplay(t *testing.T) {
	f := newNormalFixture(t, 2)
	ctx := context.Background()
	leader, ally := f.actors[0], f.actors[1]
	f.cfg.Combat = normalDefeatCombat{f.combat, leader, f.rng}
	repo, err := database.NewMonsterRepository(f.db)
	if err != nil {
		t.Fatal(err)
	}
	tamer := monster.NewService(f.chars, repo, monster.WithTransactionProvider(database.NewTransactionProvider(f.db)))
	f.cfg.Settler.(*battle.Service).SetMonsterTamer(explicitRunMonsterTamer{tamer})
	f.restart(t)
	_, lobby := f.waitingParty(t, "normal")
	entry, err := f.svc.EnterParty(ctx, leader.PlayerID, leader.ID, lobby)
	if err != nil {
		t.Fatal(err)
	}
	runID := entry.Command.RunID
	assertBalances := func(wantGrants int) {
		t.Helper()
		got, err := f.chars.FindByID(ctx, leader.ID)
		if err != nil || got.Experience != leader.Experience+3*wantGrants || got.Money != leader.Money+5*wantGrants || got.Crystal != leader.Crystal+wantGrants || got.MonsterKills != leader.MonsterKills+wantGrants {
			t.Fatal("actor balances", got, err)
		}
	}
	bad := exCommand(t, f, runID, "reject-defeat", "reject-defeat")
	before, err := f.store.FindRun(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Command(ctx, bad); !errors.Is(err, runhost.ErrUnavailable) {
		t.Fatal(err)
	}
	if after, err := f.store.FindRun(ctx, runID); err != nil || !reflect.DeepEqual(after, before) {
		t.Fatal("rejected defeat was saved", after, err)
	}
	assertBalances(0)
	if count, err := repo.CountByLocation(ctx, leader.ID, monster.LocationBox); err != nil || count != 0 {
		t.Fatal("rejected defeat tamed a monster", count, err)
	}
	first := f.send(t, runID, 0, "defeat", "")
	if len(first.MonsterTames) != 1 || !first.MonsterTames[0].Success || first.MonsterTames[0].MonsterID != "monster-002" || first.MonsterTames[0].MonsterName != "Test Slime" {
		t.Fatal(first.MonsterTames)
	}
	if count, err := repo.CountByLocation(ctx, leader.ID, monster.LocationBox); err != nil || count != 1 {
		t.Fatal("accepted defeat did not tame a monster", count, err)
	}
	assertBalances(1)
	f.restart(t)
	draws := f.rng.calls.Load()
	if replay, err := f.svc.Command(ctx, first.Command); err != nil || !reflect.DeepEqual(replay, first) || f.rng.calls.Load() != draws {
		t.Fatal("replay changed the defeat", replay, err)
	}
	for range monster.BoxCapacityForCharacter(leader) - 1 {
		now := time.Now().UTC()
		if err := repo.Save(ctx, monster.MonsterInstance{ID: id.New(), CharacterID: leader.ID, MonsterID: "monster-002", CustomName: "Slime", Location: monster.LocationBox, CreatedAt: now, UpdatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	second := f.send(t, runID, 0, "defeat", "")
	if len(second.MonsterTames) != 1 || second.MonsterTames[0].Success || !second.MonsterTames[0].BoxFull {
		t.Fatal(second.MonsterTames)
	}
	assertBalances(2)
	f.send(t, runID, 1, "fall", "")
	if got, err := f.chars.FindByID(ctx, ally.ID); err != nil || got.Experience != ally.Experience+6 || got.Money != ally.Money+10 || got.Crystal != ally.Crystal+2 || got.MonsterKills != ally.MonsterKills {
		t.Fatal("ally lost previously earned stock when falling", got, err)
	}
}
