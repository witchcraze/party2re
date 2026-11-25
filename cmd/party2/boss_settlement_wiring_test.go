package main

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	valkeygo "github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/collection"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/logging"
	"github.com/witchcraze/party2re/internal/party"
	"github.com/witchcraze/party2re/internal/testutil"
)

// openTestSealingRoom stores a king1 sealing room with one weak boss for the given
// members where the production boss service reads it. A private lobby index keeps
// the room out of shared listings.
func openTestSealingRoom(t *testing.T, client valkeygo.Client, chars ...corecharacter.Character) string {
	t.Helper()
	ctx := context.Background()
	rooms := party.NewValkeyRepository(client, party.WithLobbiesIndexKey("party2:test:boss-wiring:"+id.New()))
	state := party.LobbyState{Party: party.Party{
		ID: id.New(), Kind: party.KindSealing, StageID: "king1", Status: party.StatusRecruiting, MaxMembers: 6, CreatedAt: time.Now().UTC(),
		Enemies: []corebattle.LifetimeActor{{Participant: corebattle.Participant{ID: "king1-boss-0", Name: "レッドストーン", HP: 1, MaxHP: 1, Icon: "mon/190.gif"}, NPC: true}},
	}}
	if err := rooms.SaveParty(ctx, state.Party); err != nil {
		t.Fatal(err)
	}
	for _, c := range chars {
		m := party.Member{PartyID: state.Party.ID, CharacterID: c.ID, CharacterName: c.Name, ReadyState: true}
		if err := rooms.AddMember(ctx, m); err != nil {
			t.Fatal(err)
		}
		state.Members = append(state.Members, m)
	}
	t.Cleanup(func() {
		if err := rooms.CleanupParty(context.Background(), state); err != nil {
			t.Error(err)
		}
	})
	return state.Party.ID
}

var errBossLegendInjected = errors.New("legend induction failed after write")

// writeThenFailLegend performs the real Hall of Fame write, then fails, so only the
// production transaction wiring can remove the already-written legend row.
type writeThenFailLegend struct{ real legendInductorAdapter }

func (l writeThenFailLegend) RecordLegend(ctx context.Context, category, characterID string) error {
	if err := l.real.RecordLegend(ctx, category, characterID); err != nil {
		return err
	}
	return errBossLegendInjected
}

func TestBossProductionSettlementRollsBackMandatoryCollectionFailure(t *testing.T) {
	db := database.OpenTestDB(t)
	ctx := context.Background()
	client := testutil.OpenValkey(t)
	core, err := newCoreServices(db, client)
	if err != nil {
		t.Fatal(err)
	}
	econ, err := newEconServices(db, core)
	if err != nil {
		t.Fatal(err)
	}
	soc, err := newSocServices(db, core, econ, client, logging.Nop())
	if err != nil {
		t.Fatal(err)
	}
	misc, err := newMiscServices(db, core, soc, econ, client)
	if err != nil {
		t.Fatal(err)
	}
	cmbt, err := newCmbtServices(db, core, soc, econ, client)
	if err != nil {
		t.Fatal(err)
	}
	// This test owns a boss settlement, not the recurring global schedules.
	soc.sched = nil
	if err := wireHooks(core, cmbt, econ, misc, soc); err != nil {
		t.Fatal(err)
	}
	// The banquet writes to the shared Event Plaza and has separate coverage.
	cmbt.boss.SetVictoryBanquetHook(nil)

	char, err := database.CreateTestCharacter(ctx, db, "Seal_"+id.New()[:6])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE characters SET level = 99, hp = 500000, max_hp = 500000, attack = 999999, defense = 99999, agility = 9999 WHERE id = ?`, char.ID); err != nil {
		t.Fatal(err)
	}
	// One entry short of completion: the first boss defeat completes the Monster Book,
	// which makes the Hall of Fame induction part of the mandatory boundary.
	collectionRepo, err := database.NewCollectionRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < collection.DefaultTotalMonsters-1; i++ {
		if err := collectionRepo.RecordMonsterDefeat(ctx, char.ID, collection.DefeatedMonsterRecord{
			MonsterID: fmt.Sprintf("seed-%03d", i), MonsterName: fmt.Sprintf("seed%03d", i), Habitat: "test",
		}); err != nil {
			t.Fatal(err)
		}
	}

	count := func(query string) int {
		t.Helper()
		var n int
		if err := db.QueryRowContext(ctx, query, char.ID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	type snapshot struct {
		book, legends, histories, completions int
	}
	take := func() snapshot {
		return snapshot{
			book:        count(`SELECT COUNT(*) FROM character_monster_book WHERE character_id = ?`),
			legends:     count(`SELECT COUNT(*) FROM legend_records WHERE character_id = ?`),
			histories:   count(`SELECT COUNT(*) FROM boss_challenge_history WHERE character_id = ?`),
			completions: count(`SELECT COUNT(*) FROM character_collection_completions WHERE character_id = ?`),
		}
	}
	beforeChar, err := core.charRepo.FindByID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	before := take()

	roomID := openTestSealingRoom(t, client, char)
	misc.collection.SetLegendInductor(writeThenFailLegend{legendInductorAdapter{ranking: soc.ranking}})
	if _, err := cmbt.boss.StartSealingBattle(ctx, roomID, char.ID); !errors.Is(err, errBossLegendInjected) {
		t.Fatalf("mandatory Hall of Fame failure must fail the settlement, got %v", err)
	}
	afterChar, err := core.charRepo.FindByID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(beforeChar, afterChar) {
		t.Fatalf("character partially settled:\nbefore=%+v\nafter=%+v", beforeChar, afterChar)
	}
	if after := take(); after != before {
		t.Fatalf("production wiring committed partial state: before=%+v after=%+v", before, after)
	}

	misc.collection.SetLegendInductor(legendInductorAdapter{ranking: soc.ranking})
	res, err := cmbt.boss.StartSealingBattle(ctx, roomID, char.ID)
	if err != nil {
		t.Fatalf("retry after failure: %v", err)
	}
	if res.Outcome != corebattle.OutcomeWin || res.HeroCountGained != 1 {
		t.Fatalf("retry did not settle a victory: %+v", res)
	}
	after := take()
	if after.legends != 1 || after.histories != 1 || after.completions != 1 {
		t.Fatalf("retry state: %+v", after)
	}
	assertTrajectoryKind(t, db, char.ID, corecharacter.MemoryCompletion)
}
