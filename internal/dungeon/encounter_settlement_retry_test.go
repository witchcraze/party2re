package dungeon_test

import (
	"context"
	"errors"
	"testing"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/testutil"
)

type failedCleanupStore struct {
	dungeon.ActiveExpeditionStore
	fail bool
}

func (s *failedCleanupStore) DeleteActiveExpedition(ctx context.Context, actor, run string) error {
	if s.fail {
		return errors.New("cleanup unavailable")
	}
	return s.ActiveExpeditionStore.DeleteActiveExpedition(ctx, actor, run)
}

func TestEncounterFrozenSettlementRetry(t *testing.T) {
	for _, kind := range []string{"memory", "valkey"} {
		for _, ending := range []string{"escape", "boss", "wipeout"} {
			t.Run(kind+"/"+ending, func(t *testing.T) {
				ctx := context.Background()
				store := &failedCleanupStore{ActiveExpeditionStore: encounterStore(t, kind)}
				char := createTestChar(id.New(), 10, 100, 30, 20)
				repo := &errorInjectingDungeonRepo{mockDungeonRepo: newMockDungeonRepo(), errFinalizeExpedition: errors.New("SQL unavailable")}
				exp := sampleExpedition(id.New(), char.ID, "retry")
				monster := dungeon.DungeonMonster{ID: "enemy", HP: 100, Attack: 10, ExpReward: 17, GoldReward: 23}
				exp.Encounter = &dungeon.ActiveEncounter{Monster: monster, Enemies: []corebattle.Participant{corebattle.MustNewParticipant("enemy", 30, 10, 0)}, EventType: dungeon.EventBattle, X: 1}
				if ending == "boss" {
					exp.Encounter.EventType = dungeon.EventBoss
				}
				if err := store.SaveActiveExpedition(ctx, exp); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if err := store.ActiveExpeditionStore.DeleteActiveExpedition(ctx, char.ID, exp.ID); err != nil {
						t.Error(err)
					}
				})
				engine := &customPartyBattleEngine{resolvePartyFn: func(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
					outcome, side, hp := corebattle.OutcomeWin, "allies", 80
					if ending == "wipeout" {
						outcome, side, hp = corebattle.OutcomeDefeat, "enemies", 0
					}
					return corebattle.PartyBattleResult{Outcome: outcome, WinnerSide: side, RemainingHP: map[string]int{char.ID: hp, "enemy": 0}}, nil
				}}
				catalog := []dungeon.Dungeon{{ID: "retry", Tier: 1, ClearExpBonus: 7, ClearGoldBonus: 11}}
				svc, err := dungeon.NewService(repo, &mockCharRepo{chars: map[string]corecharacter.Character{char.ID: char}}, engine, dungeon.WithActiveExpeditionStore(store), dungeon.WithCustomDungeons(catalog))
				if err != nil {
					t.Fatal(err)
				}
				if ending == "escape" {
					_, err = svc.Escape(ctx, char.ID)
				} else {
					_, err = svc.Fight(ctx, char.ID)
				}
				if !errors.Is(err, repo.errFinalizeExpedition) {
					t.Fatalf("expected SQL failure, got %v", err)
				}
				frozen, err := store.GetActiveExpedition(ctx, char.ID)
				if err != nil {
					t.Fatal(err)
				}
				expectedStatus := dungeon.StatusEscaped
				expectedGold, expectedExp := 0, 0
				if ending == "boss" {
					expectedStatus = dungeon.StatusCleared
					expectedGold, expectedExp = 34, 24
				}
				if ending == "wipeout" {
					expectedStatus = dungeon.StatusWipedOut
				}
				if frozen == nil || frozen.Status != expectedStatus {
					t.Fatalf("unfrozen after SQL failure: %+v", frozen)
				}
				if _, err = store.Step(ctx, char.ID, dungeon.StepParams{ExpectedExpeditionID: exp.ID, ExpectedRevision: &frozen.Revision, Combat: true, GoldDelta: 999, Now: time.Now().UTC()}); !errors.Is(err, dungeon.ErrExpeditionNotActive) {
					t.Fatalf("frozen run accepted a step: %v", err)
				}
				repo.errFinalizeExpedition = nil
				store.fail = true
				// A retry through Escape must honor an already frozen clear/wipeout outcome.
				res, err := svc.Escape(ctx, char.ID)
				if err != nil || res.Expedition.Status != expectedStatus {
					t.Fatalf("retry changed outcome: %+v %v", res, err)
				}
				store.fail = false
				if _, err = svc.Fight(ctx, char.ID); err != nil {
					t.Fatalf("retry after SQL commit / lost cleanup: %v", err)
				}
				history := repo.histories[char.ID]
				if len(history) != 1 || history[0].GoldReward != expectedGold || history[0].ExpReward != expectedExp || history[0].Outcome != expectedStatus {
					t.Fatalf("duplicate/wrong settlement: %+v", history)
				}
				if got, err := store.GetActiveExpedition(ctx, char.ID); err != nil || got != nil {
					t.Fatalf("cleanup retry retained buffer: %+v %v", got, err)
				}
			})
		}
	}
}

func TestEncounterEscapeFencesPausedFight(t *testing.T) {
	for _, kind := range []string{"memory", "valkey"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			store := encounterStore(t, kind)
			char := createTestChar(id.New(), 10, 100, 30, 20)
			repo := &errorInjectingDungeonRepo{mockDungeonRepo: newMockDungeonRepo(), errFinalizeExpedition: errors.New("SQL unavailable")}
			exp := sampleExpedition(id.New(), char.ID, "dungeon-01")
			monster := dungeon.DungeonMonster{ID: "enemy", HP: 100, Attack: 10, GoldReward: 23}
			exp.Encounter = &dungeon.ActiveEncounter{Monster: monster, Enemies: []corebattle.Participant{corebattle.MustNewParticipant("enemy", 30, 10, 0)}, EventType: dungeon.EventBattle, X: 1}
			if err := store.SaveActiveExpedition(ctx, exp); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := store.DeleteActiveExpedition(context.Background(), char.ID, exp.ID); err != nil {
					t.Error(err)
				}
			})
			ready, release := make(chan struct{}), make(chan struct{})
			engine := &customPartyBattleEngine{resolvePartyFn: func(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
				close(ready)
				select {
				case <-release:
				case <-ctx.Done():
					return corebattle.PartyBattleResult{}, ctx.Err()
				}
				return corebattle.PartyBattleResult{Outcome: corebattle.OutcomeWin, WinnerSide: "allies", RemainingHP: map[string]int{char.ID: 80, "enemy": 0}}, nil
			}}
			svc, err := dungeon.NewService(repo, &mockCharRepo{chars: map[string]corecharacter.Character{char.ID: char}}, engine, dungeon.WithActiveExpeditionStore(store))
			if err != nil {
				t.Fatal(err)
			}
			fightErr, escapeErr := testutil.RunRace2(func() error {
				_, err := svc.Fight(ctx, char.ID)
				return err
			}, func() error {
				defer close(release)
				select {
				case <-ready:
				case <-ctx.Done():
					return ctx.Err()
				}
				_, err := svc.Escape(ctx, char.ID)
				return err
			})
			if !errors.Is(escapeErr, repo.errFinalizeExpedition) || !errors.Is(fightErr, dungeon.ErrExpeditionNotActive) {
				t.Fatalf("escape failed to fence combat: escape=%v fight=%v", escapeErr, fightErr)
			}
			frozen, err := store.GetActiveExpedition(ctx, char.ID)
			if err != nil {
				t.Fatal(err)
			}
			if frozen.Status != dungeon.StatusEscaped || frozen.AccumulatedGold != 0 || frozen.CurrentHP != 100 {
				t.Fatalf("late combat changed frozen snapshot: %+v", frozen)
			}
			repo.errFinalizeExpedition = nil
			if _, err = svc.Escape(ctx, char.ID); err != nil {
				t.Fatal(err)
			}
			if len(repo.histories[char.ID]) != 1 || repo.histories[char.ID][0].GoldReward != 0 {
				t.Fatal("late fight rewarded escaped run")
			}
		})
	}
}
