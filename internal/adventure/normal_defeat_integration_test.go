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
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/monster"
	"github.com/witchcraze/party2re/internal/runhost"
)

// slimeCommand adds a tameable Test Slime (monster-002) at 1 HP that any
// attack fells and returns the leader's attack on it. Its crystal comes from
// the drop roll.
func slimeCommand(t *testing.T, f *normalFixture, runID string) runhost.Command {
	t.Helper()
	slime := runEnemy("slime-"+id.New()[:8], 1)
	slime.Status, slime.Name, slime.EncounterIdentity = corebattle.StatusKinju, "Test Slime", "monster-002"
	slime.ExperienceReward, slime.GoldReward, slime.MaxHP = 3, 5, 500 // strong against the leader
	f.stage(t, runID, func(r *adventure.NormalRun) {
		r.Round = max(r.Round, 1)
		r.Battle.Actors = append(r.Battle.Actors, slime)
		r.Enemies = append(r.Enemies, slime.ID)
	})
	cmd := exCommand(t, f, runID, id.New(), "attack")
	cmd.TargetID = slime.ID
	return cmd
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
	bad := slimeCommand(t, f, runID)
	before, err := f.store.FindRun(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	f.store.failWrite.Store(true)
	if _, err := f.svc.Command(ctx, bad); err == nil {
		t.Fatal("failed defeat committed")
	}
	f.store.failWrite.Store(false)
	if after, err := f.store.FindRun(ctx, runID); err != nil || !reflect.DeepEqual(after, before) {
		t.Fatal("failed defeat was saved", after, err)
	}
	assertBalances(0)
	if count, err := repo.CountByLocation(ctx, leader.ID, monster.LocationBox); err != nil || count != 0 {
		t.Fatal("failed defeat tamed a monster", count, err)
	}
	first, err := f.svc.Command(ctx, bad)
	if err != nil {
		t.Fatal(err)
	}
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
	second, err := f.svc.Command(ctx, slimeCommand(t, f, runID))
	if err != nil {
		t.Fatal(err)
	}
	if len(second.MonsterTames) != 1 || second.MonsterTames[0].Success || !second.MonsterTames[0].BoxFull {
		t.Fatal(second.MonsterTames)
	}
	assertBalances(2)
	// The ally's own operation releases the stock it earned; a fall keeps it.
	if _, err := f.svc.Command(ctx, f.idleCommand(t, runID, 1)); err != nil {
		t.Fatal(err)
	}
	f.stage(t, runID, fall(ally.ID))
	if got, err := f.chars.FindByID(ctx, ally.ID); err != nil || got.Experience != ally.Experience+6 || got.Money != ally.Money+10 || got.Crystal != ally.Crystal+2 || got.MonsterKills != ally.MonsterKills {
		t.Fatal("ally lost previously earned stock when falling", got, err)
	}
}
