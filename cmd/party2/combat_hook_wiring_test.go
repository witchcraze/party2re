package main

import (
	"context"
	"testing"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/eventplaza"
	"github.com/witchcraze/party2re/internal/monster"
)

type tameWiringRandom struct{}

func (tameWiringRandom) Intn(int) (int, error) { return 0, nil }

func TestMonsterTamerProductionHook(t *testing.T) {
	f := newHookWiringFixture(t, nil)
	// A deterministic battle service leaves Monster injection to wireHooks.
	f.cmbt.battle = battle.NewService(
		battle.WithCharacterRepository(f.core.charRepo),
		battle.WithInventoryRepository(f.core.invRepo),
		battle.WithEquipmentRepository(f.econ.equipRepo),
		battle.WithTransactionProvider(f.core.txProvider),
		battle.WithRandomSource(tameWiringRandom{}))
	if err := wireHooks(f.core, f.cmbt, f.econ, f.misc, f.soc); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := f.cmbt.battle.ApplyPostBattleResult(ctx, battle.ApplyPostBattleRequest{
		CharacterIDs:    []string{f.char.ID},
		BattleResult:    corebattle.PartyBattleResult{Outcome: corebattle.OutcomeWin, RemainingHP: map[string]int{f.char.ID: f.char.Stats.HP}},
		DefeatedEnemies: []corebattle.Participant{{ID: "monster-002-f1-1", Name: "Slime", MaxHP: 10, Attack: 5, Defense: 5, Agility: 5}},
	}); err != nil {
		t.Fatal(err)
	}
	stored, err := f.misc.monsterRepo.ListByCharacterIDAndLocation(ctx, f.char.ID, monster.LocationBox)
	if err != nil || len(stored) != 1 || stored[0].MonsterID != "monster-002" {
		t.Fatalf("production tamer connection = %+v, %v", stored, err)
	}
}

func TestBossVictoryBanquetProductionHook(t *testing.T) {
	f := newHookWiringFixture(t, nil)
	tracker := &victoryCrowdTracker{}
	plaza, err := eventplaza.NewService(
		&struct{ eventplaza.Repository }{}, &struct{ eventplaza.CharacterRepository }{},
		&struct{ eventplaza.InventoryRepository }{}, eventplaza.WithPresenceTracker(tracker))
	if err != nil {
		t.Fatal(err)
	}
	f.misc.eventplaza = plaza
	if err := wireHooks(f.core, f.cmbt, f.econ, f.misc, f.soc); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	f.char.Level, f.char.Stats.HP, f.char.Stats.MaxHP = 99, 500000, 500000
	f.char.Stats.Attack, f.char.Stats.Defense, f.char.Stats.Agility = 999999, 99999, 9999
	if err := f.core.charRepo.Update(ctx, f.char); err != nil {
		t.Fatal(err)
	}
	result, err := f.cmbt.boss.ChallengeBoss(ctx, f.char.ID, "king1")
	if err != nil || result.Outcome != corebattle.OutcomeWin {
		t.Fatalf("boss victory = %+v, %v", result, err)
	}
	if tracker.calls != 1 {
		t.Fatalf("production banquet calls = %d", tracker.calls)
	}
}
