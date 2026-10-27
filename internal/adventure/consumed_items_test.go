package adventure_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
)

// itemUseResolver records the ActionItems each ally holds when a floor starts
// and reports the scripted consumption for that floor.
type itemUseResolver struct {
	consumeByCall map[int]map[string][]corebattle.ConsumedItem
	defeatOnCall  int
	calls         int
	held          []map[string][]string // per call: ally ID -> ActionItem instance IDs
}

func (r *itemUseResolver) ResolvePartyBattle(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
	r.calls++
	held := make(map[string][]string)
	remHP := make(map[string]int)
	for _, a := range req.Allies {
		held[a.ID] = []string{}
		for _, it := range a.ActionItems {
			held[a.ID] = append(held[a.ID], it.InstanceID)
		}
		remHP[a.ID] = a.HP
	}
	r.held = append(r.held, held)
	outcome := corebattle.OutcomeWin
	if r.defeatOnCall == r.calls {
		outcome = corebattle.OutcomeDefeat
		for id := range remHP {
			remHP[id] = 0
		}
	}
	return corebattle.PartyBattleResult{
		Outcome:       outcome,
		Turns:         1,
		RemainingHP:   remHP,
		ConsumedItems: r.consumeByCall[r.calls],
	}, nil
}

func (r *itemUseResolver) Resolve(corebattle.Request) (corebattle.Result, error) {
	return corebattle.Result{Outcome: corebattle.OutcomeWin}, nil
}

func herb(instanceID string) corebattle.ActionItem {
	return corebattle.ActionItem{
		ID: "item-001", InstanceID: instanceID, Name: "薬草",
		UsageCategory: coreitem.UsageCategoryCombatOnly,
		Kind:          corebattle.ActionKindHeal, Power: 40, TargetScope: corebattle.TargetScopeSingleAlly,
	}
}

func participantWithItems(c corecharacter.Character, items ...corebattle.ActionItem) corebattle.Participant {
	p := corebattle.MustNewParticipant(c.ID, c.Stats.HP, c.Stats.Attack, c.Stats.Defense)
	p.MaxHP = c.Stats.MaxHP
	p.ActionItems = items
	return p
}

func TestCrawlSession_CarriesConsumedItemsAcrossFloors(t *testing.T) {
	stages, monsters := setupTestCatalogs(t)
	stage, err := stages.FindByID("stage-01")
	if err != nil {
		t.Fatal(err)
	}
	chars := createTestCharacters()[:2]
	parts := []corebattle.Participant{
		participantWithItems(chars[0], herb("herb-a1"), herb("herb-a2")),
		participantWithItems(chars[1], herb("herb-b1")),
	}
	resolver := &itemUseResolver{consumeByCall: map[int]map[string][]corebattle.ConsumedItem{
		1: {chars[0].ID: {{ID: "item-001", InstanceID: "herb-a1", Quantity: 1}}},
		2: {
			chars[0].ID: {{ID: "item-001", InstanceID: "herb-a2", Quantity: 1}},
			chars[1].ID: {{ID: "item-001", InstanceID: "herb-b1", Quantity: 1}},
		},
	}}
	session, err := adventure.NewCrawlSessionWithParticipants(stage, chars, parts, func(int) int { return 0 })
	if err != nil {
		t.Fatal(err)
	}
	for f := 1; f <= 3; f++ {
		if _, err := session.AdvanceFloor(stages, monsters, resolver); err != nil {
			t.Fatalf("floor %d: %v", f, err)
		}
	}

	wantHeld := []map[string][]string{
		{chars[0].ID: {"herb-a1", "herb-a2"}, chars[1].ID: {"herb-b1"}},
		{chars[0].ID: {"herb-a2"}, chars[1].ID: {"herb-b1"}},
		{chars[0].ID: {}, chars[1].ID: {}},
	}
	if !reflect.DeepEqual(resolver.held, wantHeld) {
		t.Fatalf("items held at floor start = %v, want %v", resolver.held, wantHeld)
	}

	wantConsumed := map[string][]corebattle.ConsumedItem{
		chars[0].ID: {{ID: "item-001", InstanceID: "herb-a1", Quantity: 1}, {ID: "item-001", InstanceID: "herb-a2", Quantity: 1}},
		chars[1].ID: {{ID: "item-001", InstanceID: "herb-b1", Quantity: 1}},
	}
	if got := session.Result().ConsumedItems; !reflect.DeepEqual(got, wantConsumed) {
		t.Fatalf("Result().ConsumedItems = %v, want %v", got, wantConsumed)
	}
	if len(parts[0].ActionItems) != 2 {
		t.Fatalf("caller-owned participant mutated: %v", parts[0].ActionItems)
	}
}

func TestCrawlSession_RecordsConsumptionOnDefeatFloor(t *testing.T) {
	stages, monsters := setupTestCatalogs(t)
	stage, _ := stages.FindByID("stage-01")
	chars := createTestCharacters()[:1]
	resolver := &itemUseResolver{
		defeatOnCall: 2,
		consumeByCall: map[int]map[string][]corebattle.ConsumedItem{
			1: {chars[0].ID: {{ID: "item-001", InstanceID: "herb-1", Quantity: 1}}},
			2: {chars[0].ID: {{ID: "item-001", InstanceID: "herb-2", Quantity: 1}}},
		},
	}
	session, err := adventure.NewCrawlSessionWithParticipants(stage, chars,
		[]corebattle.Participant{participantWithItems(chars[0], herb("herb-1"), herb("herb-2"))}, func(int) int { return 0 })
	if err != nil {
		t.Fatal(err)
	}
	for f := 1; f <= 2; f++ {
		if _, err := session.AdvanceFloor(stages, monsters, resolver); err != nil {
			t.Fatal(err)
		}
	}
	want := []corebattle.ConsumedItem{{ID: "item-001", InstanceID: "herb-1", Quantity: 1}, {ID: "item-001", InstanceID: "herb-2", Quantity: 1}}
	if got := session.Result().ConsumedItems[chars[0].ID]; !reflect.DeepEqual(got, want) {
		t.Fatalf("defeat result ConsumedItems = %v, want %v", got, want)
	}
}

func TestCrawlSession_ConsumedDokuroAmuletIsNotReusedOnLaterFloors(t *testing.T) {
	stages, monsters := setupTestCatalogs(t)
	stage, _ := stages.FindByID("stage-01")
	chars := createTestCharacters()[:1]
	p := participantWithItems(chars[0])
	p.Abilities = []string{"dokuro_amulet", "touki_shield"}
	p.ItemDefinitionIDs = []string{battle.ItemDokuroAmulet}
	var abilitiesSeen [][]string
	resolver := abilityProbeResolver{inner: &itemUseResolver{consumeByCall: map[int]map[string][]corebattle.ConsumedItem{
		1: {chars[0].ID: {{ID: battle.ItemDokuroAmulet, Quantity: 1}}},
	}}, seen: &abilitiesSeen}
	session, err := adventure.NewCrawlSessionWithParticipants(stage, chars, []corebattle.Participant{p}, func(int) int { return 0 })
	if err != nil {
		t.Fatal(err)
	}
	for f := 1; f <= 2; f++ {
		if _, err := session.AdvanceFloor(stages, monsters, resolver); err != nil {
			t.Fatal(err)
		}
	}
	want := [][]string{{"dokuro_amulet", "touki_shield"}, {"touki_shield"}}
	if !reflect.DeepEqual(abilitiesSeen, want) {
		t.Fatalf("abilities at floor start = %v, want %v", abilitiesSeen, want)
	}
}

type abilityProbeResolver struct {
	inner *itemUseResolver
	seen  *[][]string
}

func (r abilityProbeResolver) ResolvePartyBattle(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
	*r.seen = append(*r.seen, append([]string{}, req.Allies[0].Abilities...))
	return r.inner.ResolvePartyBattle(req)
}

func TestAdventure_SoloSettlementReceivesAccumulatedConsumedItems(t *testing.T) {
	char, err := corecharacter.New("Hero")
	if err != nil {
		t.Fatal(err)
	}
	settler := &mockPostBattleSettler{}
	svc, _ := setupTestAdventureServiceWithBattle(t, char, settler, &itemUseResolver{
		defeatOnCall: 3,
		consumeByCall: map[int]map[string][]corebattle.ConsumedItem{
			1: {char.ID: {{ID: "item-001", InstanceID: "herb-1", Quantity: 1}}},
			3: {char.ID: {{ID: "item-002", InstanceID: "herb-2", Quantity: 1}}},
		},
	})
	if _, err := svc.ExecuteCrawl(context.Background(), adventure.DungeonCrawlRequest{
		CharacterIDs: []string{char.ID}, StageID: adventure.StarterAdventure, Rng: func(int) int { return 0 },
	}); err != nil {
		t.Fatal(err)
	}
	want := map[string][]corebattle.ConsumedItem{
		char.ID: {{ID: "item-001", InstanceID: "herb-1", Quantity: 1}, {ID: "item-002", InstanceID: "herb-2", Quantity: 1}},
	}
	if got := settler.lastReq.BattleResult.ConsumedItems; !reflect.DeepEqual(got, want) {
		t.Fatalf("settlement ConsumedItems = %v, want %v", got, want)
	}
}
