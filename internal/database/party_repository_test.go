package database

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/party"
)

func TestPartyRepository(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	player, err := CreateTestPlayer(ctx, db)
	if err != nil {
		t.Fatal(err)
	}

	charRepo, err := NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	partyRepo, err := NewPartyRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	// Create 2 test characters
	c1, err := corecharacter.New(fmt.Sprintf("PartyLeader_%s", player.ID[:8]))
	if err != nil {
		t.Fatal(err)
	}
	c1.PlayerID = player.ID
	if err := charRepo.Save(ctx, c1); err != nil {
		t.Fatal(err)
	}

	c2, err := corecharacter.New(fmt.Sprintf("PartyMember_%s", player.ID[:8]))
	if err != nil {
		t.Fatal(err)
	}
	c2.PlayerID = player.ID
	if err := charRepo.Save(ctx, c2); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	pID := id.New()
	testParty := party.Party{
		ID:                pID,
		LeaderCharacterID: c1.ID,
		Name:              "DB Test Party",
		PasswordHash:      "passhash123",
		StageID:           "forest",
		Speed:             3,
		MaxMembers:        4,
		MinLevel:          1,
		MaxLevel:          99,
		MinHP:             0,
		Status:            party.StatusRecruiting,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	// 1. Save and Get Party
	if err := partyRepo.SaveParty(ctx, testParty); err != nil {
		t.Fatalf("SaveParty failed: %v", err)
	}

	loadedParty, err := partyRepo.GetParty(ctx, pID)
	if err != nil {
		t.Fatalf("GetParty failed: %v", err)
	}
	if loadedParty.Name != "DB Test Party" || loadedParty.LeaderCharacterID != c1.ID {
		t.Fatalf("unexpected loaded party: %+v", loadedParty)
	}

	// 2. Add Leader Member
	leaderMem := party.Member{
		PartyID:       pID,
		CharacterID:   c1.ID,
		CharacterName: c1.Name,
		JobID:         c1.JobID,
		Level:         c1.Level,
		HP:            c1.Stats.HP,
		MaxHP:         c1.Stats.MaxHP,
		IsLeader:      true,
		ReadyState:    true,
		JoinedAt:      now,
	}
	if err := partyRepo.AddMember(ctx, leaderMem); err != nil {
		t.Fatalf("AddMember(leader) failed: %v", err)
	}

	// 3. Add Second Member
	secondMem := party.Member{
		PartyID:       pID,
		CharacterID:   c2.ID,
		CharacterName: c2.Name,
		JobID:         c2.JobID,
		Level:         c2.Level,
		HP:            c2.Stats.HP,
		MaxHP:         c2.Stats.MaxHP,
		IsLeader:      false,
		ReadyState:    false,
		JoinedAt:      now,
	}
	if err := partyRepo.AddMember(ctx, secondMem); err != nil {
		t.Fatalf("AddMember(second) failed: %v", err)
	}

	// 4. Count and Get Members
	count, err := partyRepo.CountMembers(ctx, pID)
	if err != nil || count != 2 {
		t.Fatalf("expected count 2, got %d (err: %v)", count, err)
	}

	members, err := partyRepo.GetMembers(ctx, pID)
	if err != nil || len(members) != 2 {
		t.Fatalf("expected 2 members, got %d (err: %v)", len(members), err)
	}

	// 5. GetActivePartyByCharacter
	activeParty, m, err := partyRepo.GetActivePartyByCharacter(ctx, c2.ID)
	if err != nil {
		t.Fatalf("GetActivePartyByCharacter failed: %v", err)
	}
	if activeParty.ID != pID || m.CharacterID != c2.ID {
		t.Fatalf("unexpected active party result: party=%+v member=%+v", activeParty, m)
	}

	// 6. Update Member Ready
	if err := partyRepo.UpdateMemberReady(ctx, pID, c2.ID, true); err != nil {
		t.Fatalf("UpdateMemberReady failed: %v", err)
	}
	updatedM2, err := partyRepo.GetMember(ctx, pID, c2.ID)
	if err != nil || !updatedM2.ReadyState {
		t.Fatalf("expected ready_state=true, got %+v", updatedM2)
	}

	// 7. Save Adventure Log
	advLog := party.PartyAdventureLog{
		ID:                  id.New(),
		PartyID:             pID,
		StageID:             "forest",
		Outcome:             "win",
		Turns:               3,
		TotalEXP:            100,
		TotalGold:           50,
		SynergyBonusPercent: 10,
		DetailsJSON:         `{"outcome":"win"}`,
		CreatedAt:           now,
	}
	if err := partyRepo.SaveAdventureLog(ctx, advLog); err != nil {
		t.Fatalf("SaveAdventureLog failed: %v", err)
	}

	// 8. Remove Member
	if err := partyRepo.RemoveMember(ctx, pID, c2.ID); err != nil {
		t.Fatalf("RemoveMember failed: %v", err)
	}
	countAfterRemove, _ := partyRepo.CountMembers(ctx, pID)
	if countAfterRemove != 1 {
		t.Fatalf("expected 1 member after remove, got %d", countAfterRemove)
	}

	// 9. Delete Party
	if err := partyRepo.DeleteParty(ctx, pID); err != nil {
		t.Fatalf("DeleteParty failed: %v", err)
	}
	if _, err := partyRepo.GetParty(ctx, pID); !errors.Is(err, party.ErrNotFound) {
		t.Fatalf("expected party.ErrNotFound after delete, got %v", err)
	}
}
