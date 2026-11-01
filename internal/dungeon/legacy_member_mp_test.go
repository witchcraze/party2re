package dungeon_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/id"
)

type mpSpendingResolver struct {
	mpCost map[string]int
	seenMP map[string]int
}

func (r *mpSpendingResolver) Resolve(req corebattle.Request) (corebattle.Result, error) {
	return corebattle.Result{Outcome: corebattle.OutcomeWin}, nil
}

func (r *mpSpendingResolver) ResolvePartyBattle(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
	if r.seenMP == nil {
		r.seenMP = make(map[string]int, len(req.Allies))
	}
	remHP := make(map[string]int, len(req.Allies))
	remMP := make(map[string]int, len(req.Allies))
	for _, a := range req.Allies {
		r.seenMP[a.ID] = a.MP
		remHP[a.ID] = a.HP
		mp := a.MP - r.mpCost[a.ID]
		if mp < 0 {
			mp = 0
		}
		remMP[a.ID] = mp
	}
	return corebattle.PartyBattleResult{
		Outcome:     corebattle.OutcomeWin,
		WinnerSide:  "allies",
		Turns:       1,
		RemainingHP: remHP,
		RemainingMP: remMP,
	}, nil
}

type preFinalizeHookRepo struct {
	dungeon.Repository
	beforeFinalize func(ctx context.Context) error
}

func (r *preFinalizeHookRepo) FinalizeExpedition(
	ctx context.Context,
	history dungeon.DungeonExpeditionHistory,
	record dungeon.CharacterDungeonRecord,
	character *corecharacter.Character,
	rewardItems []coreitem.Instance,
	members ...dungeon.ExpeditionMember,
) error {
	if r.beforeFinalize != nil {
		if err := r.beforeFinalize(ctx); err != nil {
			return err
		}
	}
	return r.Repository.FinalizeExpedition(ctx, history, record, character, rewardItems, members...)
}

type failingFindCharRepo struct {
	inner   battle.CharacterRepository
	failIDs map[string]error
}

func (f *failingFindCharRepo) FindByID(ctx context.Context, charID string) (corecharacter.Character, error) {
	if err, ok := f.failIDs[charID]; ok && err != nil {
		return corecharacter.Character{}, err
	}
	return f.inner.FindByID(ctx, charID)
}

func (f *failingFindCharRepo) FindByIDForUpdate(ctx context.Context, charID string) (corecharacter.Character, error) {
	if err, ok := f.failIDs[charID]; ok && err != nil {
		return corecharacter.Character{}, err
	}
	return f.inner.FindByIDForUpdate(ctx, charID)
}

func (f *failingFindCharRepo) Update(ctx context.Context, char corecharacter.Character) error {
	return f.inner.Update(ctx, char)
}

type mockBattleCharRepo struct {
	*mockCharRepo
}

func (m *mockBattleCharRepo) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	return m.FindByID(ctx, id)
}

func (m *mockBattleCharRepo) Update(ctx context.Context, char corecharacter.Character) error {
	m.chars[char.ID] = char
	return nil
}

func TestDungeonMemberMP_EncodeDecodeContract(t *testing.T) {
	legacyJSON := `[{"character_id":"char-1","name":"MP audit","current_hp":40,"max_hp":100}]`
	legacyMembers := dungeon.DecodeMembers(legacyJSON)
	if len(legacyMembers) != 1 {
		t.Fatalf("expected 1 legacy member, got %d", len(legacyMembers))
	}
	if legacyMembers[0].HasCurrentMP() {
		t.Fatalf("expected legacy member without current_mp to report HasCurrentMP() == false")
	}
	reencodedLegacy := dungeon.EncodeMembers(legacyMembers)
	if strings.Contains(reencodedLegacy, "current_mp") {
		t.Fatalf("expected re-encoded legacy member to omit current_mp, got %s", reencodedLegacy)
	}

	nonZeroJSON := `[{"character_id":"char-1","name":"MP audit","current_hp":40,"max_hp":100,"current_mp":50,"max_mp":50}]`
	nonZeroMembers := dungeon.DecodeMembers(nonZeroJSON)
	if len(nonZeroMembers) != 1 || !nonZeroMembers[0].HasCurrentMP() || nonZeroMembers[0].CurrentMP != 50 || nonZeroMembers[0].MaxMP != 50 {
		t.Fatalf("unexpected decoded non-zero MP member: %+v (HasCurrentMP=%v)", nonZeroMembers, nonZeroMembers[0].HasCurrentMP())
	}

	zeroJSON := `[{"character_id":"char-1","name":"MP audit","current_hp":40,"max_hp":100,"current_mp":0,"max_mp":0}]`
	zeroMembers := dungeon.DecodeMembers(zeroJSON)
	if len(zeroMembers) != 1 || !zeroMembers[0].HasCurrentMP() || zeroMembers[0].CurrentMP != 0 {
		t.Fatalf("expected explicit 0 MP member to report HasCurrentMP() == true, got %+v", zeroMembers)
	}
	reencodedZero := dungeon.EncodeMembers(zeroMembers)
	if !strings.Contains(reencodedZero, `"current_mp":0`) {
		t.Fatalf("expected re-encoded explicit 0 MP member to retain current_mp:0, got %s", reencodedZero)
	}

	maxMPOnlyJSON := `[{"character_id":"char-1","name":"MP audit","current_hp":40,"max_hp":100,"max_mp":50}]`
	maxMPOnlyMembers := dungeon.DecodeMembers(maxMPOnlyJSON)
	if len(maxMPOnlyMembers) != 1 || maxMPOnlyMembers[0].HasCurrentMP() || maxMPOnlyMembers[0].MaxMP != 50 {
		t.Fatalf("expected max_mp-only member without current_mp to report HasCurrentMP() == false and MaxMP == 50, got %+v", maxMPOnlyMembers)
	}
	reencodedMaxMPOnly := dungeon.EncodeMembers(maxMPOnlyMembers)
	if strings.Contains(reencodedMaxMPOnly, "current_mp") || !strings.Contains(reencodedMaxMPOnly, `"max_mp":50`) {
		t.Fatalf("expected re-encoded max_mp-only member to omit current_mp and keep max_mp:50, got %s", reencodedMaxMPOnly)
	}
}

func TestDungeonMemberMP_CombatAndEscape_MemoryAndValkey(t *testing.T) {
	ctx := context.Background()
	valkeyClient := openTestValkey(t)
	t.Cleanup(valkeyClient.Close)

	customDungeon := dungeon.Dungeon{
		ID:               "dg-mp-compat",
		Name:             "MP Compat Dungeon",
		MinLevel:         1,
		MaxTurnsPerFloor: 20,
		Floors: []dungeon.Floor{
			{
				FloorNumber: 1,
				Width:       3,
				Height:      1,
				StartX:      0,
				StartY:      0,
				Grid:        []string{"S0D"},
				Monsters: []dungeon.DungeonMonster{
					{ID: "slime", Name: "Slime", HP: 10, Attack: 1, Defense: 1, Agility: 1, ExpReward: 10, GoldReward: 10},
				},
			},
		},
	}

	type testCase struct {
		name               string
		membersJSON        func(leadID, allyID string) string
		expectedCombatLead int
		expectedCombatAlly int
		mpCostLead         int
		mpCostAlly         int
		expectedEscapeLead int
		expectedEscapeAlly int
	}

	cases := []testCase{
		{
			name: "legacy_missing_mp",
			membersJSON: func(leadID, allyID string) string {
				return fmt.Sprintf(
					`[{"character_id":%q,"name":"MP audit","current_hp":40,"max_hp":100},{"character_id":%q,"name":"Ally audit","current_hp":55,"max_hp":90}]`,
					leadID, allyID,
				)
			},
			expectedCombatLead: 50,
			expectedCombatAlly: 35,
			mpCostLead:         12,
			mpCostAlly:         5,
			expectedEscapeLead: 38,
			expectedEscapeAlly: 30,
		},
		{
			name: "new_nonzero_mp",
			membersJSON: func(leadID, allyID string) string {
				return fmt.Sprintf(
					`[{"character_id":%q,"name":"MP audit","current_hp":40,"max_hp":100,"current_mp":30,"max_mp":50},{"character_id":%q,"name":"Ally audit","current_hp":55,"max_hp":90,"current_mp":20,"max_mp":40}]`,
					leadID, allyID,
				)
			},
			expectedCombatLead: 30,
			expectedCombatAlly: 20,
			mpCostLead:         10,
			mpCostAlly:         5,
			expectedEscapeLead: 20,
			expectedEscapeAlly: 15,
		},
		{
			name: "new_zero_mp",
			membersJSON: func(leadID, allyID string) string {
				return fmt.Sprintf(
					`[{"character_id":%q,"name":"MP audit","current_hp":40,"max_hp":100,"current_mp":0,"max_mp":50},{"character_id":%q,"name":"Ally audit","current_hp":55,"max_hp":90,"current_mp":0,"max_mp":40}]`,
					leadID, allyID,
				)
			},
			expectedCombatLead: 0,
			expectedCombatAlly: 0,
			mpCostLead:         0,
			mpCostAlly:         0,
			expectedEscapeLead: 0,
			expectedEscapeAlly: 0,
		},
	}

	for _, storeKind := range []string{"memory", "valkey"} {
		for _, tc := range cases {
			t.Run(storeKind+"_"+tc.name, func(t *testing.T) {
				leadID := "lead-" + id.New()
				allyID := "ally-" + id.New()

				var store dungeon.ActiveExpeditionStore
				if storeKind == "memory" {
					store = dungeon.NewMemoryExpeditionRepository()
				} else {
					vStore, err := dungeon.NewValkeyExpeditionRepository(valkeyClient)
					if err != nil {
						t.Fatal(err)
					}
					store = vStore
					t.Cleanup(func() {
						cleanupDungeonValkey(t, valkeyClient, leadID)
						_ = valkeyClient.Do(ctx, valkeyClient.B().Del().Key("party2:dungeon:{char:"+allyID+"}:member").Build()).Error()
					})
				}

				leadChar := createTestChar(leadID, 10, 100, 30, 20)
				leadChar.Stats.HP = 80
				leadChar.Stats.MaxHP = 100
				leadChar.Stats.MP = 50
				leadChar.Stats.MaxMP = 50

				allyChar := createTestChar(allyID, 10, 90, 25, 15)
				allyChar.Stats.HP = 70
				allyChar.Stats.MaxHP = 90
				allyChar.Stats.MP = 35
				allyChar.Stats.MaxMP = 40

				charRepo := &mockCharRepo{chars: map[string]corecharacter.Character{
					leadID: leadChar,
					allyID: allyChar,
				}}
				repo := newMockDungeonRepo()
				resolver := &mpSpendingResolver{
					mpCost: map[string]int{
						leadID: tc.mpCostLead,
						allyID: tc.mpCostAlly,
					},
				}
				battleSvc := battle.NewService(battle.WithCharacterRepository(&mockBattleCharRepo{mockCharRepo: charRepo}))

				svc, err := dungeon.NewService(
					repo,
					charRepo,
					resolver,
					dungeon.WithCustomDungeons([]dungeon.Dungeon{customDungeon}),
					dungeon.WithActiveExpeditionStore(store),
					dungeon.WithParticipantBuilder(battleSvc),
				)
				if err != nil {
					t.Fatal(err)
				}

				rawMembers := tc.membersJSON(leadID, allyID)
				seedExp := dungeon.ActiveExpedition{
					ID:               "exp-" + id.New(),
					CharacterID:      leadID,
					PartyID:          "party-1",
					Members:          dungeon.DecodeMembers(rawMembers),
					DungeonID:        customDungeon.ID,
					CurrentFloor:     1,
					PosX:             0,
					PosY:             0,
					CurrentHP:        40,
					TurnsRemaining:   20,
					Status:           dungeon.StatusExploring,
					AccumulatedItems: []string{},
					ConsumedEvents:   []string{},
					StartedAt:        time.Now().UTC(),
					UpdatedAt:        time.Now().UTC(),
				}
				if err := store.SaveActiveExpedition(ctx, seedExp); err != nil {
					t.Fatal(err)
				}
				if storeKind == "valkey" {
					sKey := "party2:dungeon:{char:" + leadID + "}:state"
					if err := valkeyClient.Do(ctx, valkeyClient.B().Hset().Key(sKey).FieldValue().FieldValue("members", rawMembers).Build()).Error(); err != nil {
						t.Fatal(err)
					}
				}

				moveRes, err := svc.Move(ctx, leadID, dungeon.DirectionEast)
				if err != nil {
					t.Fatalf("Move failed: %v", err)
				}
				if moveRes.EventType != dungeon.EventBattle {
					t.Fatalf("expected EventBattle, got %v", moveRes.EventType)
				}
				if resolver.seenMP[leadID] != tc.expectedCombatLead {
					t.Errorf("expected leader combat start MP %d, got %d", tc.expectedCombatLead, resolver.seenMP[leadID])
				}
				if resolver.seenMP[allyID] != tc.expectedCombatAlly {
					t.Errorf("expected ally combat start MP %d, got %d", tc.expectedCombatAlly, resolver.seenMP[allyID])
				}

				escRes, err := svc.Escape(ctx, leadID)
				if err != nil {
					t.Fatalf("Escape failed: %v", err)
				}
				if !escRes.IsFinished {
					t.Fatalf("expected finished expedition after Escape")
				}
				if repo.savedChars[leadID].Stats.MP != tc.expectedEscapeLead {
					t.Errorf("expected leader saved MP %d, got %d", tc.expectedEscapeLead, repo.savedChars[leadID].Stats.MP)
				}
				if repo.savedChars[allyID].Stats.MP != tc.expectedEscapeAlly {
					t.Errorf("expected ally saved MP %d, got %d", tc.expectedEscapeAlly, repo.savedChars[allyID].Stats.MP)
				}
			})
		}
	}
}

func TestDungeonIntegration_LegacyMemberMP_RealSQLAndValkey(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	valkeyClient := openTestValkey(t)
	t.Cleanup(valkeyClient.Close)

	store, err := dungeon.NewValkeyExpeditionRepository(valkeyClient)
	if err != nil {
		t.Fatal(err)
	}
	dungeonRepo, err := database.NewDungeonRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	setupFixture := func(t *testing.T, leadMP, leadMaxMP, allyMP, allyMaxMP int) (corecharacter.Character, corecharacter.Character) {
		t.Helper()
		lead, err := database.CreateTestCharacterWithFunds(ctx, db, "MP audit", 500)
		if err != nil {
			t.Fatal(err)
		}
		ally, err := database.CreateTestCharacterWithFunds(ctx, db, "Ally audit", 500)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			cleanupCtx := context.Background()
			cleanupDungeonValkey(t, valkeyClient, lead.ID)
			_ = valkeyClient.Do(cleanupCtx, valkeyClient.B().Del().Key("party2:dungeon:{char:"+ally.ID+"}:member").Build()).Error()
			_, _ = db.ExecContext(cleanupCtx, "DELETE FROM dungeon_expedition_history WHERE character_id IN (?, ?)", lead.ID, ally.ID)
			_, _ = db.ExecContext(cleanupCtx, "DELETE FROM character_dungeon_records WHERE character_id IN (?, ?)", lead.ID, ally.ID)
			_, _ = db.ExecContext(cleanupCtx, "DELETE FROM dungeon_active_expeditions WHERE character_id IN (?, ?)", lead.ID, ally.ID)
			_ = charRepo.Delete(cleanupCtx, lead.ID)
			_, _ = db.ExecContext(cleanupCtx, "DELETE FROM players WHERE id = ?", lead.PlayerID)
			_ = charRepo.Delete(cleanupCtx, ally.ID)
			_, _ = db.ExecContext(cleanupCtx, "DELETE FROM players WHERE id = ?", ally.PlayerID)
		})

		lead.Level = 10
		lead.Stats.HP = 80
		lead.Stats.MaxHP = 100
		lead.Stats.MP = leadMP
		lead.Stats.MaxMP = leadMaxMP
		if err := charRepo.Update(ctx, lead); err != nil {
			t.Fatal(err)
		}

		ally.Level = 10
		ally.Stats.HP = 70
		ally.Stats.MaxHP = 90
		ally.Stats.MP = allyMP
		ally.Stats.MaxMP = allyMaxMP
		if err := charRepo.Update(ctx, ally); err != nil {
			t.Fatal(err)
		}
		return lead, ally
	}

	t.Run("non_combat_escape_preserves_legacy_2_member_mp", func(t *testing.T) {
		lead, ally := setupFixture(t, 50, 50, 35, 40)
		battleSvc := battle.NewService(battle.WithCharacterRepository(charRepo))
		svc, err := dungeon.NewService(
			dungeonRepo,
			charRepo,
			corebattle.Engine{},
			dungeon.WithActiveExpeditionStore(store),
			dungeon.WithParticipantBuilder(battleSvc),
		)
		if err != nil {
			t.Fatal(err)
		}

		exp, err := svc.StartExpedition(ctx, lead.ID, "dungeon-01")
		if err != nil {
			t.Fatal(err)
		}
		legacyMembersJSON := fmt.Sprintf(
			`[{"character_id":%q,"name":"MP audit","current_hp":40,"max_hp":100},{"character_id":%q,"name":"Ally audit","current_hp":55,"max_hp":90}]`,
			lead.ID, ally.ID,
		)
		sKey := "party2:dungeon:{char:" + lead.ID + "}:state"
		if err := valkeyClient.Do(ctx, valkeyClient.B().Hset().Key(sKey).FieldValue().
			FieldValue("current_hp", "40").
			FieldValue("members", legacyMembersJSON).
			Build()).Error(); err != nil {
			t.Fatal(err)
		}

		if _, err := svc.Escape(ctx, lead.ID); err != nil {
			t.Fatalf("Escape failed: %v", err)
		}

		reloadedLead, err := charRepo.FindByID(ctx, lead.ID)
		if err != nil {
			t.Fatal(err)
		}
		reloadedAlly, err := charRepo.FindByID(ctx, ally.ID)
		if err != nil {
			t.Fatal(err)
		}
		if reloadedLead.Stats.HP != 40 || reloadedLead.Stats.MP != 50 {
			t.Errorf("expected leader HP=40 MP=50, got HP=%d MP=%d (exp=%s)", reloadedLead.Stats.HP, reloadedLead.Stats.MP, exp.ID)
		}
		if reloadedAlly.Stats.HP != 55 || reloadedAlly.Stats.MP != 35 {
			t.Errorf("expected ally HP=55 MP=35, got HP=%d MP=%d", reloadedAlly.Stats.HP, reloadedAlly.Stats.MP)
		}
	})

	t.Run("new_explicit_zero_mp_persists_zero_on_escape", func(t *testing.T) {
		lead, ally := setupFixture(t, 50, 50, 35, 40)
		battleSvc := battle.NewService(battle.WithCharacterRepository(charRepo))
		svc, err := dungeon.NewService(
			dungeonRepo,
			charRepo,
			corebattle.Engine{},
			dungeon.WithActiveExpeditionStore(store),
			dungeon.WithParticipantBuilder(battleSvc),
		)
		if err != nil {
			t.Fatal(err)
		}

		if _, err := svc.StartExpedition(ctx, lead.ID, "dungeon-01"); err != nil {
			t.Fatal(err)
		}
		zeroMembersJSON := fmt.Sprintf(
			`[{"character_id":%q,"name":"MP audit","current_hp":40,"max_hp":100,"current_mp":0,"max_mp":50},{"character_id":%q,"name":"Ally audit","current_hp":55,"max_hp":90,"current_mp":0,"max_mp":40}]`,
			lead.ID, ally.ID,
		)
		sKey := "party2:dungeon:{char:" + lead.ID + "}:state"
		if err := valkeyClient.Do(ctx, valkeyClient.B().Hset().Key(sKey).FieldValue().
			FieldValue("current_hp", "40").
			FieldValue("members", zeroMembersJSON).
			Build()).Error(); err != nil {
			t.Fatal(err)
		}

		if _, err := svc.Escape(ctx, lead.ID); err != nil {
			t.Fatalf("Escape failed: %v", err)
		}

		reloadedLead, err := charRepo.FindByID(ctx, lead.ID)
		if err != nil {
			t.Fatal(err)
		}
		reloadedAlly, err := charRepo.FindByID(ctx, ally.ID)
		if err != nil {
			t.Fatal(err)
		}
		if reloadedLead.Stats.MP != 0 {
			t.Errorf("expected leader explicit 0 MP to persist as 0, got %d", reloadedLead.Stats.MP)
		}
		if reloadedAlly.Stats.MP != 0 {
			t.Errorf("expected ally explicit 0 MP to persist as 0, got %d", reloadedAlly.Stats.MP)
		}
	})

	t.Run("combat_start_and_escape_from_legacy_2_member_payload", func(t *testing.T) {
		lead, ally := setupFixture(t, 50, 50, 35, 40)
		battleSvc := battle.NewService(battle.WithCharacterRepository(charRepo))
		resolver := &mpSpendingResolver{
			mpCost: map[string]int{
				lead.ID: 12,
				ally.ID: 5,
			},
		}
		combatDungeon := dungeon.Dungeon{
			ID:               "dg-real-combat-mp",
			Name:             "Real Combat MP Dungeon",
			MinLevel:         1,
			MaxTurnsPerFloor: 20,
			Floors: []dungeon.Floor{
				{
					FloorNumber: 1,
					Width:       3,
					Height:      1,
					StartX:      0,
					StartY:      0,
					Grid:        []string{"S0D"},
					Monsters: []dungeon.DungeonMonster{
						{ID: "slime", Name: "Slime", HP: 10, Attack: 1, Defense: 1, Agility: 1, ExpReward: 10, GoldReward: 10},
					},
				},
			},
		}
		svc, err := dungeon.NewService(
			dungeonRepo,
			charRepo,
			resolver,
			dungeon.WithCustomDungeons([]dungeon.Dungeon{combatDungeon}),
			dungeon.WithActiveExpeditionStore(store),
			dungeon.WithParticipantBuilder(battleSvc),
		)
		if err != nil {
			t.Fatal(err)
		}

		if _, err := svc.StartExpedition(ctx, lead.ID, combatDungeon.ID); err != nil {
			t.Fatal(err)
		}
		legacyMembersJSON := fmt.Sprintf(
			`[{"character_id":%q,"name":"MP audit","current_hp":40,"max_hp":100},{"character_id":%q,"name":"Ally audit","current_hp":55,"max_hp":90}]`,
			lead.ID, ally.ID,
		)
		sKey := "party2:dungeon:{char:" + lead.ID + "}:state"
		if err := valkeyClient.Do(ctx, valkeyClient.B().Hset().Key(sKey).FieldValue().
			FieldValue("current_hp", "40").
			FieldValue("members", legacyMembersJSON).
			Build()).Error(); err != nil {
			t.Fatal(err)
		}

		if _, err := svc.Move(ctx, lead.ID, dungeon.DirectionEast); err != nil {
			t.Fatalf("Move into combat failed: %v", err)
		}
		if resolver.seenMP[lead.ID] != 50 || resolver.seenMP[ally.ID] != 35 {
			t.Fatalf("expected combat start MP (50, 35), got (%d, %d)", resolver.seenMP[lead.ID], resolver.seenMP[ally.ID])
		}

		if _, err := svc.Escape(ctx, lead.ID); err != nil {
			t.Fatalf("Escape after combat failed: %v", err)
		}

		reloadedLead, err := charRepo.FindByID(ctx, lead.ID)
		if err != nil {
			t.Fatal(err)
		}
		reloadedAlly, err := charRepo.FindByID(ctx, ally.ID)
		if err != nil {
			t.Fatal(err)
		}
		if reloadedLead.Stats.MP != 38 || reloadedAlly.Stats.MP != 30 {
			t.Errorf("expected post-combat Escape MP (38, 30), got (%d, %d)", reloadedLead.Stats.MP, reloadedAlly.Stats.MP)
		}
	})

	t.Run("concurrent_mp_update_before_finalize_lock_is_preserved_for_legacy_members", func(t *testing.T) {
		lead, ally := setupFixture(t, 30, 50, 20, 40)
		battleSvc := battle.NewService(battle.WithCharacterRepository(charRepo))
		attempt := 0
		transientErr := errors.New("transient SQL contention before lock")
		hookRepo := &preFinalizeHookRepo{
			Repository: dungeonRepo,
			beforeFinalize: func(ctx context.Context) error {
				attempt++
				if attempt == 1 {
					// Simulate concurrent MP recovery (e.g. Tavern meal/delivery) after Escape read leader MP=30
					// and before FinalizeExpedition's SELECT ... FOR UPDATE, followed by a transient failure.
					if _, err := db.ExecContext(ctx, "UPDATE characters SET mp = 48 WHERE id = ?", lead.ID); err != nil {
						return err
					}
					if _, err := db.ExecContext(ctx, "UPDATE characters SET mp = 39 WHERE id = ?", ally.ID); err != nil {
						return err
					}
					return transientErr
				}
				// On retry (resumeSettlement), simulate another concurrent MP update before SELECT ... FOR UPDATE.
				if _, err := db.ExecContext(ctx, "UPDATE characters SET mp = 49 WHERE id = ?", lead.ID); err != nil {
					return err
				}
				return nil
			},
		}
		svc, err := dungeon.NewService(
			hookRepo,
			charRepo,
			corebattle.Engine{},
			dungeon.WithActiveExpeditionStore(store),
			dungeon.WithParticipantBuilder(battleSvc),
		)
		if err != nil {
			t.Fatal(err)
		}

		if _, err := svc.StartExpedition(ctx, lead.ID, "dungeon-01"); err != nil {
			t.Fatal(err)
		}
		legacyMembersJSON := fmt.Sprintf(
			`[{"character_id":%q,"name":"MP audit","current_hp":40,"max_hp":100},{"character_id":%q,"name":"Ally audit","current_hp":55,"max_hp":90}]`,
			lead.ID, ally.ID,
		)
		sKey := "party2:dungeon:{char:" + lead.ID + "}:state"
		if err := valkeyClient.Do(ctx, valkeyClient.B().Hset().Key(sKey).FieldValue().
			FieldValue("current_hp", "40").
			FieldValue("members", legacyMembersJSON).
			Build()).Error(); err != nil {
			t.Fatal(err)
		}

		// First Escape freezes the expedition and fails in beforeFinalize after concurrent DB MP updates.
		if _, err := svc.Escape(ctx, lead.ID); !errors.Is(err, transientErr) {
			t.Fatalf("expected transient error %v, got %v", transientErr, err)
		}
		// Second Escape resumes settlement and must preserve the latest locked DB MP (49 and 39), never stale 30/48 or 0.
		if _, err := svc.Escape(ctx, lead.ID); err != nil {
			t.Fatalf("retry Escape failed: %v", err)
		}

		reloadedLead, err := charRepo.FindByID(ctx, lead.ID)
		if err != nil {
			t.Fatal(err)
		}
		reloadedAlly, err := charRepo.FindByID(ctx, ally.ID)
		if err != nil {
			t.Fatal(err)
		}
		if reloadedLead.Stats.HP != 40 || reloadedLead.Stats.MP != 49 {
			t.Errorf("expected leader HP=40 MP=49 (locked DB value), got HP=%d MP=%d", reloadedLead.Stats.HP, reloadedLead.Stats.MP)
		}
		if reloadedAlly.Stats.HP != 55 || reloadedAlly.Stats.MP != 39 {
			t.Errorf("expected ally HP=55 MP=39 (locked DB value), got HP=%d MP=%d", reloadedAlly.Stats.HP, reloadedAlly.Stats.MP)
		}
	})

	t.Run("read_failure_preserves_db_mp_and_active_expedition", func(t *testing.T) {
		lead, ally := setupFixture(t, 50, 50, 35, 40)
		injectedErr := errors.New("simulated character repository read outage")
		flakyCharRepo := &failingFindCharRepo{
			inner:   charRepo,
			failIDs: map[string]error{},
		}
		battleSvc := battle.NewService(battle.WithCharacterRepository(flakyCharRepo))
		combatDungeon := dungeon.Dungeon{
			ID:               "dg-read-fail-mp",
			Name:             "Read Fail MP Dungeon",
			MinLevel:         1,
			MaxTurnsPerFloor: 20,
			Floors: []dungeon.Floor{
				{
					FloorNumber: 1,
					Width:       3,
					Height:      1,
					StartX:      0,
					StartY:      0,
					Grid:        []string{"S0D"},
					Monsters: []dungeon.DungeonMonster{
						{ID: "slime", Name: "Slime", HP: 10, Attack: 1, Defense: 1, Agility: 1, ExpReward: 10, GoldReward: 10},
					},
				},
			},
		}
		svc, err := dungeon.NewService(
			dungeonRepo,
			flakyCharRepo,
			corebattle.Engine{},
			dungeon.WithCustomDungeons([]dungeon.Dungeon{combatDungeon}),
			dungeon.WithActiveExpeditionStore(store),
			dungeon.WithParticipantBuilder(battleSvc),
		)
		if err != nil {
			t.Fatal(err)
		}

		exp, err := svc.StartExpedition(ctx, lead.ID, combatDungeon.ID)
		if err != nil {
			t.Fatal(err)
		}
		legacyMembersJSON := fmt.Sprintf(
			`[{"character_id":%q,"name":"MP audit","current_hp":40,"max_hp":100},{"character_id":%q,"name":"Ally audit","current_hp":55,"max_hp":90}]`,
			lead.ID, ally.ID,
		)
		sKey := "party2:dungeon:{char:" + lead.ID + "}:state"
		if err := valkeyClient.Do(ctx, valkeyClient.B().Hset().Key(sKey).FieldValue().
			FieldValue("current_hp", "40").
			FieldValue("members", legacyMembersJSON).
			Build()).Error(); err != nil {
			t.Fatal(err)
		}

		// 1. Inject read failure on ally during combat participant build (Move)
		flakyCharRepo.failIDs[ally.ID] = injectedErr
		if _, err := svc.Move(ctx, lead.ID, dungeon.DirectionEast); !errors.Is(err, injectedErr) {
			t.Fatalf("expected Move to fail with %v, got %v", injectedErr, err)
		}

		// 2. Inject read failure on leader during Escape before freeze
		flakyCharRepo.failIDs[lead.ID] = injectedErr
		if _, err := svc.Escape(ctx, lead.ID); !errors.Is(err, injectedErr) {
			t.Fatalf("expected Escape to fail with %v, got %v", injectedErr, err)
		}
		delete(flakyCharRepo.failIDs, lead.ID)
		delete(flakyCharRepo.failIDs, ally.ID)

		// Active expedition must remain Exploring in Valkey, and DB characters must remain untouched
		stillActive, err := store.GetActiveExpedition(ctx, lead.ID)
		if err != nil || stillActive == nil || stillActive.ID != exp.ID || stillActive.Status != dungeon.StatusExploring {
			t.Fatalf("expected active expedition to remain StatusExploring on read failure, got %+v (err=%v)", stillActive, err)
		}
		reloadedLead, err := charRepo.FindByID(ctx, lead.ID)
		if err != nil {
			t.Fatal(err)
		}
		reloadedAlly, err := charRepo.FindByID(ctx, ally.ID)
		if err != nil {
			t.Fatal(err)
		}
		if reloadedLead.Stats.HP != 80 || reloadedLead.Stats.MP != 50 {
			t.Errorf("expected leader DB stats untouched (HP=80, MP=50), got HP=%d MP=%d", reloadedLead.Stats.HP, reloadedLead.Stats.MP)
		}
		if reloadedAlly.Stats.HP != 70 || reloadedAlly.Stats.MP != 35 {
			t.Errorf("expected ally DB stats untouched (HP=70, MP=35), got HP=%d MP=%d", reloadedAlly.Stats.HP, reloadedAlly.Stats.MP)
		}
	})
}
