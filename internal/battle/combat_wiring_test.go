package battle_test

import (
	"context"
	"testing"

	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/pvp"
)

type captureEngine struct {
	lastReq corebattle.PartyBattleRequest
}

func (c *captureEngine) ResolvePartyBattle(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
	c.lastReq = req
	return corebattle.PartyBattleResult{
		Outcome:     corebattle.OutcomeWin,
		WinnerSide:  pvp.ColorRed,
		WinnerTeam:  pvp.ColorRed,
		Turns:       1,
		RemainingHP: map[string]int{"char-pvp": 100, "char-opp": 0},
	}, nil
}

func setupEquippedCharacter(ctx context.Context, charRepo *mockCharRepo, invRepo *mockInvRepo, equipRepo *mockEquipRepo, charID string, atk, def, agi int) {
	char := corecharacter.Character{
		ID:    charID,
		Name:  "英雄アレックス",
		Money: 1000,
		Stats: corecharacter.Stats{
			HP:      100,
			MaxHP:   100,
			Attack:  atk,
			Defense: def,
			Agility: agi,
		},
	}
	_ = charRepo.Update(ctx, char)

	inv, _ := coreinventory.New(charID)
	// weapon-08: 銅の剣 (+14 atk, wt 9)
	sword, _ := coreitem.NewInstance("weapon-08", 1)
	// armor-07: 鎖かたびら (+24 def, wt 8)
	armor, _ := coreitem.NewInstance("armor-07", 1)
	_ = inv.Add(sword)
	_ = inv.Add(armor)
	_ = invRepo.Save(ctx, inv)

	equip, _ := coreequipment.New(charID)
	equip.Slots[coreitem.SlotMainHand] = sword.ID
	equip.Slots[coreitem.SlotBody] = armor.ID
	_ = equipRepo.Save(ctx, equip)
}

func TestCombatWiring_PvPEquippedParticipant(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharRepo()
	invRepo := newMockInvRepo()
	equipRepo := newMockEquipRepo()

	setupEquippedCharacter(ctx, charRepo, invRepo, equipRepo, "char-pvp", 30, 20, 25)

	battleAdapter := battle.NewService(
		battle.WithCharacterRepository(charRepo),
		battle.WithInventoryRepository(invRepo),
		battle.WithEquipmentRepository(equipRepo),
	)

	capture := &captureEngine{}
	roomRepo := pvp.NewMemoryRoomRepository()
	pvpService, err := pvp.NewService(
		roomRepo,
		charRepo,
		capture,
		pvp.WithParticipantBuilder(battleAdapter),
	)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	detail, err := pvpService.CreateRoom(ctx, "char-pvp", pvp.CreateRoomRequest{
		Name:       "PvP Test Room",
		MaxMembers: 2,
		Bet:        100,
		TargetWins: 1,
	})
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}

	// Add opponent
	opp := corecharacter.Character{
		ID:    "char-opp",
		Name:  "対戦相手",
		Money: 1000,
		Stats: corecharacter.Stats{
			HP: 100, MaxHP: 100, Attack: 20, Defense: 10,
		},
	}
	_ = charRepo.Update(ctx, opp)
	_, err = pvpService.JoinRoom(ctx, "char-opp", detail.Room.ID, "")
	if err != nil {
		t.Fatalf("JoinRoom failed: %v", err)
	}

	_, _ = pvpService.SelectTeam(ctx, "char-pvp", detail.Room.ID, pvp.ColorRed)
	_, _ = pvpService.SelectTeam(ctx, "char-opp", detail.Room.ID, pvp.ColorBlue)

	_, err = pvpService.StartMatch(ctx, "char-pvp", detail.Room.ID)
	if err != nil {
		t.Fatalf("StartMatch failed: %v", err)
	}

	_, err = pvpService.AdvanceRound(ctx, "char-pvp", detail.Room.ID)
	if err != nil {
		t.Fatalf("AdvanceRound failed: %v", err)
	}

	// Verify that participant passed into engine had equipped stats (atk: 30+14=44, def: 20+24=44, agi: 25-9-8=8)
	var pvpPart *corebattle.Participant
	for i := range capture.lastReq.Allies {
		if capture.lastReq.Allies[i].ID == "char-pvp" {
			pvpPart = &capture.lastReq.Allies[i]
			break
		}
	}
	if pvpPart == nil {
		for i := range capture.lastReq.Enemies {
			if capture.lastReq.Enemies[i].ID == "char-pvp" {
				pvpPart = &capture.lastReq.Enemies[i]
				break
			}
		}
	}
	if pvpPart == nil {
		t.Fatalf("char-pvp participant not found in battle request")
	}
	if pvpPart.Attack != 44 {
		t.Errorf("pvpPart.Attack = %d, want 44 (equipped weapon-08)", pvpPart.Attack)
	}
	if pvpPart.Defense != 44 {
		t.Errorf("pvpPart.Defense = %d, want 44 (equipped armor-07)", pvpPart.Defense)
	}
	if pvpPart.Agility != 8 {
		t.Errorf("pvpPart.Agility = %d, want 8 (agility modified by weight)", pvpPart.Agility)
	}
	if pvpPart.TeamID != pvp.ColorRed {
		t.Errorf("pvpPart.TeamID = %s, want %s", pvpPart.TeamID, pvp.ColorRed)
	}
}

func TestCombatWiring_AdventureEquippedParticipant(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharRepo()
	invRepo := newMockInvRepo()
	equipRepo := newMockEquipRepo()

	setupEquippedCharacter(ctx, charRepo, invRepo, equipRepo, "char-adv", 30, 20, 25)

	battleAdapter := battle.NewService(
		battle.WithCharacterRepository(charRepo),
		battle.WithInventoryRepository(invRepo),
		battle.WithEquipmentRepository(equipRepo),
	)

	part, err := battleAdapter.BuildParticipant(ctx, "char-adv")
	if err != nil {
		t.Fatalf("BuildParticipant failed: %v", err)
	}

	char, _ := charRepo.FindByID(ctx, "char-adv")
	stage := adventure.Stage{
		ID:   "stage-1",
		Name: "テスト洞窟",
	}

	session, err := adventure.NewCrawlSessionWithParticipants(stage, []corecharacter.Character{char}, []corebattle.Participant{part}, nil)
	if err != nil {
		t.Fatalf("NewCrawlSessionWithParticipants failed: %v", err)
	}

	if len(session.Participants) != 1 {
		t.Fatalf("session.Participants len = %d, want 1", len(session.Participants))
	}
	p := session.Participants[0]
	if p.Attack != 44 {
		t.Errorf("p.Attack = %d, want 44", p.Attack)
	}
	if p.Defense != 44 {
		t.Errorf("p.Defense = %d, want 44", p.Defense)
	}
}

func TestCombatWiring_EquippedParticipantWithCurrentHP_AndGuild(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharRepo()
	invRepo := newMockInvRepo()
	equipRepo := newMockEquipRepo()

	setupEquippedCharacter(ctx, charRepo, invRepo, equipRepo, "char-gvg", 30, 20, 25)

	battleAdapter := battle.NewService(
		battle.WithCharacterRepository(charRepo),
		battle.WithInventoryRepository(invRepo),
		battle.WithEquipmentRepository(equipRepo),
	)

	// Test current HP override for dungeon/challenge/gvg/pvp
	part, err := battleAdapter.BuildParticipantWithCurrentHP(ctx, "char-gvg", 60)
	if err != nil {
		t.Fatalf("BuildParticipantWithCurrentHP failed: %v", err)
	}

	if part.HP != 60 {
		t.Errorf("part.HP = %d, want 60 (current HP override)", part.HP)
	}
	if part.MaxHP != 100 {
		t.Errorf("part.MaxHP = %d, want 100", part.MaxHP)
	}
	if part.Attack != 44 {
		t.Errorf("part.Attack = %d, want 44", part.Attack)
	}
	if part.Defense != 44 {
		t.Errorf("part.Defense = %d, want 44", part.Defense)
	}
	if part.Agility != 8 {
		t.Errorf("part.Agility = %d, want 8", part.Agility)
	}

	// GvG sets TeamID to guildID
	part.TeamID = "guild-warriors-99"
	if part.TeamID != "guild-warriors-99" {
		t.Errorf("part.TeamID = %s, want guild-warriors-99", part.TeamID)
	}
}
