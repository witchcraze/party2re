package database_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/monster"
)

type creditTamer struct{ svc *monster.Service }

func (a creditTamer) TameMonster(ctx context.Context, characterID, monsterID, name string) error {
	_, err := a.svc.TameMonster(ctx, characterID, monsterID, name)
	if errors.Is(err, monster.ErrBoxFull) {
		return battle.ErrMonsterBoxFull
	}
	return err
}

// In-run Dungeon credits apply once per run and credit ID through the shared
// post-battle owner: kill counter, recruitment within box capacity, and no
// grant after the run is finalized.
func TestDungeonRepository_SettleDefeatCredits(t *testing.T) {
	db := database.OpenTestDB(t)
	ctx := context.Background()
	repo, err := database.NewDungeonRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	chars, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	invs, err := database.NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	equips, err := database.NewEquipmentRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	monsters, err := database.NewMonsterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	tx := database.NewTransactionProvider(db)
	tamer := monster.NewService(chars, monsters, monster.WithTransactionProvider(tx))
	repo.SetPostBattleSettler(battle.NewService(
		battle.WithCharacterRepository(chars), battle.WithInventoryRepository(invs), battle.WithEquipmentRepository(equips),
		battle.WithTransactionProvider(tx), battle.WithMonsterTamer(creditTamer{tamer}),
	))

	killer, err := database.CreateTestCharacterWithFunds(ctx, db, "Credit Killer "+id.New()[:8], 100)
	if err != nil {
		t.Fatal(err)
	}
	member, err := database.CreateTestCharacterWithFunds(ctx, db, "Credit Member "+id.New()[:8], 100)
	if err != nil {
		t.Fatal(err)
	}
	database.RegisterCharacterCleanup(t, db, killer, member)
	crystal0, kills0 := killer.Crystal, killer.MonsterKills

	runID := id.New()
	kill := func(capture bool) battle.ExplicitDefeat {
		return battle.ExplicitDefeat{Monster: battle.DefeatedMonsterRecord{MonsterID: "slime", MonsterName: "スライム"}, StrongKill: true, Capture: capture}
	}
	credits := []dungeon.DefeatCredit{
		{ID: "r1:" + killer.ID, CharacterID: killer.ID, Reward: corebattle.Reward{Experience: 5, Currency: 30, Crystals: 1}, Defeats: []battle.ExplicitDefeat{kill(true)}},
		{ID: "r1:" + member.ID, CharacterID: member.ID, Reward: corebattle.Reward{Experience: 5, Currency: 30, Crystals: 1}},
	}
	assert := func(step string, id string, money, crystals, kills, boxed int) {
		t.Helper()
		c, err := chars.FindByID(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		n, err := monsters.CountByLocation(ctx, id, monster.LocationBox)
		if err != nil {
			t.Fatal(err)
		}
		if c.Money != money || c.Crystal != crystals || c.MonsterKills != kills || n != boxed {
			t.Fatalf("%s: money=%d crystal=%d kills=%d box=%d, want %d %d %d %d", step, c.Money, c.Crystal, c.MonsterKills, n, money, crystals, kills, boxed)
		}
	}
	for i := 0; i < 2; i++ { // the retry finds every receipt
		if err := repo.SettleDefeatCredits(ctx, runID, credits); err != nil {
			t.Fatal(err)
		}
	}
	assert("first credits", killer.ID, 130, crystal0+1, kills0+1, 1)
	assert("first credits", member.ID, 130, member.Crystal+1, member.MonsterKills, 0)

	// A full box turns the recruit away without failing the credit.
	for n := 1; n < monster.BaseBoxCapacity; n++ {
		if _, err := tamer.TameMonster(ctx, killer.ID, "filler", "filler"); err != nil {
			t.Fatal(err)
		}
	}
	full := []dungeon.DefeatCredit{{ID: "r2:" + killer.ID, CharacterID: killer.ID, Reward: corebattle.Reward{Currency: 10}, Defeats: []battle.ExplicitDefeat{kill(true)}}}
	if err := repo.SettleDefeatCredits(ctx, runID, full); err != nil {
		t.Fatal(err)
	}
	assert("full box", killer.ID, 140, crystal0+1, kills0+2, monster.BaseBoxCapacity)

	// Escape settles every credit before claiming the run; afterwards a stale credit is ignored.
	history := dungeon.DungeonExpeditionHistory{ID: runID, CharacterID: killer.ID, DungeonID: "dungeon-01", Outcome: dungeon.StatusEscaped, CreatedAt: time.Now().UTC()}
	if err := repo.FinalizeExpedition(ctx, history, &killer, dungeon.ExpeditionMember{CharacterID: killer.ID, CurrentHP: killer.Stats.HP}); err != nil {
		t.Fatal(err)
	}
	stale := []dungeon.DefeatCredit{{ID: "r3:" + killer.ID, CharacterID: killer.ID, Reward: corebattle.Reward{Currency: 1000}}}
	if err := repo.SettleDefeatCredits(ctx, runID, stale); err != nil {
		t.Fatal(err)
	}
	assert("after finalize", killer.ID, 140, crystal0+1, kills0+2, monster.BaseBoxCapacity)
	var receipts int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM dungeon_defeat_credits WHERE run_id = ?`, runID).Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if receipts != 0 {
		t.Fatalf("finalized run kept %d receipts", receipts)
	}
}
