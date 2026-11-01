package dungeon_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/testutil"
)

type pauseEscapeRead struct {
	dungeon.ActiveExpeditionStore
	ready, release chan struct{}
	once           sync.Once
}
type escapeReadKey struct{}

func (s *pauseEscapeRead) GetActiveExpedition(ctx context.Context, actor string) (*dungeon.ActiveExpedition, error) {
	exp, err := s.ActiveExpeditionStore.GetActiveExpedition(ctx, actor)
	if ctx.Value(escapeReadKey{}) != nil {
		s.once.Do(func() { close(s.ready) })
		select {
		case <-s.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return exp, err
}

func TestEncounterFightEscapeStaleSettlement(t *testing.T) {
	for _, kind := range []string{"memory", "valkey"} {
		for _, event := range []dungeon.TileEventType{dungeon.EventBattle, dungeon.EventBoss} {
			t.Run(kind+"/"+string(event), func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
				defer cancel()
				underlying := encounterStore(t, kind)
				store := &pauseEscapeRead{ActiveExpeditionStore: underlying, ready: make(chan struct{}), release: make(chan struct{})}
				var release sync.Once
				unblock := func() { release.Do(func() { close(store.release) }) }
				defer unblock()
				char := createTestChar(id.New(), 10, 100, 30, 20)
				repo := &errorInjectingDungeonRepo{mockDungeonRepo: newMockDungeonRepo()}
				if event == dungeon.EventBoss {
					repo.errFinalizeExpedition = errors.New("SQL unavailable after boss victory")
				}
				monster := dungeon.DungeonMonster{ID: "enemy", HP: 100, Attack: 10, ExpReward: 17, GoldReward: 23, DropItemID: "potion"}
				exp := sampleExpedition(id.New(), char.ID, "review")
				exp.Encounter = &dungeon.ActiveEncounter{Monster: monster, Enemies: []corebattle.Participant{corebattle.MustNewParticipant("enemy", 30, 10, 0)}, EventType: event, X: 1}
				if err := underlying.SaveActiveExpedition(ctx, exp); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if err := underlying.DeleteActiveExpedition(context.Background(), char.ID, exp.ID); err != nil {
						t.Error(err)
					}
				})
				engine := &customPartyBattleEngine{resolvePartyFn: func(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
					return corebattle.PartyBattleResult{Outcome: corebattle.OutcomeWin, WinnerSide: "allies", RemainingHP: map[string]int{char.ID: 80, "enemy": 0}}, nil
				}}
				svc, err := dungeon.NewService(repo, &mockCharRepo{chars: map[string]corecharacter.Character{char.ID: char}}, engine, dungeon.WithActiveExpeditionStore(store), dungeon.WithCustomDungeons([]dungeon.Dungeon{{ID: "review", Tier: 1, ClearExpBonus: 7, ClearGoldBonus: 11}}))
				if err != nil {
					t.Fatal(err)
				}
				fightErr, escapeErr := testutil.RunRace2(func() error {
					select {
					case <-store.ready:
					case <-ctx.Done():
						return ctx.Err()
					}
					defer unblock()
					_, err := svc.Fight(ctx, char.ID)
					return err
				}, func() error {
					_, err := svc.Escape(context.WithValue(ctx, escapeReadKey{}, true), char.ID)
					return err
				})
				if !errors.Is(fightErr, repo.errFinalizeExpedition) {
					t.Fatal(fightErr)
				}
				if escapeErr == nil {
					t.Fatalf("stale Escape settled after Fight won: histories=%+v", repo.histories[char.ID])
				}
				if len(repo.histories[char.ID]) != 0 {
					t.Fatal("stale snapshot reached SQL finalizer")
				}
				repo.errFinalizeExpedition = nil
				expectedGold, expectedExp := 23, 17
				if event == dungeon.EventBoss {
					expectedGold, expectedExp = 34, 24
				}
				if _, err = svc.Escape(ctx, char.ID); err != nil {
					t.Fatal(err)
				}
				if len(repo.histories[char.ID]) != 1 || repo.histories[char.ID][0].GoldReward != expectedGold || repo.histories[char.ID][0].ExpReward != expectedExp || repo.histories[char.ID][0].ItemsRewardCount != 1 || repo.savedChars[char.ID].Stats.HP != 80 {
					t.Fatalf("lost victory reward/vitality: %+v", repo.histories[char.ID])
				}
			})
		}
	}
}

func TestEncounterAlliedStatusAcrossDraws(t *testing.T) {
	for _, kind := range []string{"memory", "valkey"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			store := encounterStore(t, kind)
			char := createTestChar(id.New(), 10, 100, 30, 20)
			chars := &mockCharRepo{chars: map[string]corecharacter.Character{char.ID: char}}
			monster := dungeon.DungeonMonster{ID: "enemy", HP: 100, Attack: 10}
			catalog := []dungeon.Dungeon{{ID: "status", MaxTurnsPerFloor: 20, Floors: []dungeon.Floor{{FloorNumber: 1, Width: 2, Height: 1, Grid: []string{"S0"}, Monsters: []dungeon.DungeonMonster{monster}}}}}
			calls := 0
			engine := &customPartyBattleEngine{resolvePartyFn: func(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
				calls++
				expected := ""
				if calls == 2 {
					expected = corebattle.StatusPoison
				}
				if calls == 3 {
					expected = corebattle.StatusParalyze
				}
				if req.Allies[0].Status != expected {
					return corebattle.PartyBattleResult{}, errors.New("allied status was lost across a draw")
				}
				expectedCMP := 25
				if calls == 2 {
					expectedCMP = 13
				}
				if calls >= 3 {
					expectedCMP = 0
				}
				if req.Allies[0].CMP != expectedCMP {
					return corebattle.PartyBattleResult{}, errors.New("CMP was restored across a draw")
				}
				if calls == 2 && (req.InitialField == nil || req.InitialField.Element != corebattle.ElementFire || req.InitialField.RemainingTurn != 4) {
					return corebattle.PartyBattleResult{}, errors.New("field was lost across a draw")
				}
				if calls == 3 && (req.InitialField == nil || req.InitialField.RemainingTurn != 3) {
					return corebattle.PartyBattleResult{}, errors.New("field duration reset")
				}
				if calls == 4 && (req.InitialField == nil || req.InitialField.Element != "") {
					return corebattle.PartyBattleResult{}, errors.New("expired field restored")
				}
				status := corebattle.StatusPoison
				if calls == 2 {
					status = corebattle.StatusParalyze
				}
				if calls >= 3 {
					status = ""
				}
				remainingCMP := 13
				if calls >= 2 {
					remainingCMP = 0
				}
				field := &corebattle.FieldState{Element: corebattle.ElementFire, RemainingTurn: 5 - calls}
				if calls >= 3 {
					field = &corebattle.FieldState{}
				}
				return corebattle.PartyBattleResult{RemainingCMP: map[string]int{char.ID: remainingCMP}, FinalField: field, Outcome: corebattle.OutcomeDraw, RemainingHP: map[string]int{char.ID: 90, "enemy": 50}, RemainingStatus: map[string]string{char.ID: status}}, nil
			}}
			svc, err := dungeon.NewService(newMockDungeonRepo(), chars, engine, dungeon.WithActiveExpeditionStore(store), dungeon.WithCustomDungeons(catalog), dungeon.WithParticipantBuilder(&customParticipantBuilder{buildFn: func(_ context.Context, actor string, hp int) (corebattle.Participant, error) {
				return corebattle.Participant{ID: actor, HP: hp, MaxHP: 100, CMP: 25, MaxCMP: 25}, nil
			}}))
			if err != nil {
				t.Fatal(err)
			}
			exp, err := svc.StartExpedition(ctx, char.ID, "status")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := store.DeleteActiveExpedition(ctx, char.ID, exp.ID); err != nil {
					t.Error(err)
				}
			})
			first, err := svc.Move(ctx, char.ID, dungeon.DirectionEast)
			if err != nil {
				t.Fatal(err)
			}
			// Returned snapshots must not alias persisted maps or field state.
			first.Expedition.Encounter.Statuses[char.ID] = "wrong"
			first.Expedition.Encounter.CMP[char.ID] = 999
			first.Expedition.Encounter.Field.Element = corebattle.ElementWater
			for i := 0; i < 3; i++ {
				if _, err = svc.Fight(ctx, char.ID); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestEncounterRestoredParalysisUsesSharedResolver(t *testing.T) {
	for _, kind := range []string{"memory", "valkey"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			store := encounterStore(t, kind)
			char := createTestChar(id.New(), 10, 1000, 30, 20)
			exp := sampleExpedition(id.New(), char.ID, "dungeon-01")
			exp.CurrentHP = 1000
			monster := dungeon.DungeonMonster{ID: "enemy", HP: 1000, Attack: 0}
			exp.Encounter = &dungeon.ActiveEncounter{Monster: monster, Enemies: []corebattle.Participant{corebattle.MustNewParticipant("enemy", 1000, 0, 0)}, Statuses: map[string]string{char.ID: corebattle.StatusParalyze}, EventType: dungeon.EventBattle, X: 1}
			if err := store.SaveActiveExpedition(ctx, exp); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := store.DeleteActiveExpedition(ctx, char.ID, exp.ID); err != nil {
					t.Error(err)
				}
			})
			svc, err := dungeon.NewService(newMockDungeonRepo(), &mockCharRepo{chars: map[string]corecharacter.Character{char.ID: char}}, corebattle.Engine{}, dungeon.WithActiveExpeditionStore(store), dungeon.WithRNG(&minimalRandomSource{}))
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				res, err := svc.Fight(ctx, char.ID)
				if err != nil {
					t.Fatal(err)
				}
				if res.BattleResult.Outcome != corebattle.OutcomeDraw || res.Expedition.Encounter == nil || res.Expedition.Encounter.Enemies[0].HP != 1000 || res.Expedition.Encounter.Statuses[char.ID] != corebattle.StatusParalyze {
					t.Fatalf("paralysis did not survive shared resolver batch: %+v", res)
				}
				skipped := false
				for _, log := range res.BattleResult.Logs {
					if log.ActorID == char.ID {
						if log.ActionName != "行動不能" {
							t.Fatalf("paralyzed ally acted: %+v", log)
						}
						skipped = true
					}
				}
				if !skipped {
					t.Fatal("no paralyzed ally action observed")
				}
			}
		})
	}
}
