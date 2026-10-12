package battle

import (
	"errors"
	"reflect"
	"testing"

	"github.com/witchcraze/party2re/internal/core/item"
)

func TestUnsupportedExecutionDoesNotMutate(t *testing.T) {
	for _, kind := range []string{"unknown", "random", "delegated", ""} {
		t.Run(kind, func(t *testing.T) {
			ctx := &battleContext{hpMap: map[string]int{"a": 50}, mpMap: map[string]int{"a": 20}, cmpMap: map[string]int{"a": 20}, itemsMap: map[string][]ActionItem{"a": {{ID: "tool"}}}}
			beforeHP, beforeMP, beforeCMP := copyHPMap(ctx.hpMap), copyHPMap(ctx.mpMap), copyHPMap(ctx.cmpMap)
			actor := Participant{ID: "a"}
			if err := ctx.executeJobSkill(actor, &ActionSkill{Kind: kind, MPCost: 5}, nil, nil, nil); !errors.Is(err, ErrUnsupportedEffect) {
				t.Fatalf("job: %v", err)
			}
			cs := ActionCustomSkill{CMPCost: 5, Gems: []GemEffect{{Kind: ActionKindHeal, Power: 10}, {Kind: kind}}}
			if err := ctx.executeCustomSkill(actor, &cs, nil, nil); !errors.Is(err, ErrUnsupportedEffect) {
				t.Fatalf("custom: %v", err)
			}
			if err := ctx.executeItem(actor, &ActionItem{ID: "tool", Kind: kind, UsageCategory: item.UsageCategoryCombatOnly}, nil, nil); !errors.Is(err, ErrUnsupportedEffect) {
				t.Fatalf("item: %v", err)
			}
			if !reflect.DeepEqual(ctx.hpMap, beforeHP) || !reflect.DeepEqual(ctx.mpMap, beforeMP) || !reflect.DeepEqual(ctx.cmpMap, beforeCMP) || len(ctx.logs) != 0 || ctx.field != nil || ctx.consumedItems != nil || len(ctx.itemsMap["a"]) != 1 {
				t.Fatalf("rejection changed state: %+v", ctx)
			}
		})
	}
}

func TestIncompleteNativePayloadsRejected(t *testing.T) {
	for _, sk := range []ActionSkill{
		{Kind: ActionKindStatus},
		{Kind: ActionKindStatus, Status: "unimplemented"},
		{Kind: ActionKindBuff, BuffStat: "mp"},
		{Kind: ActionKindAttack, TargetScope: "random"},
		{Kind: ActionKindAttack, MPCost: -1},
	} {
		if err := ValidateSkillAction(sk); !errors.Is(err, ErrUnsupportedEffect) {
			t.Errorf("incomplete skill accepted: %+v (%v)", sk, err)
		}
	}
	for _, gem := range []GemEffect{
		{Kind: "field"},
		{Kind: "field", Element: "unimplemented", Duration: 3},
		{Kind: "anti_field"},
		{Kind: ActionKindHeal, TargetScope: TargetScopeSelf},
		{Kind: ActionKindBuff, BuffStat: "mp"},
	} {
		if err := ValidateCustomSkillAction(ActionCustomSkill{Gems: []GemEffect{gem}}); !errors.Is(err, ErrUnsupportedEffect) {
			t.Errorf("incomplete gem accepted: %+v (%v)", gem, err)
		}
	}
}

func TestAdmissionEmptyAndNoEffect(t *testing.T) {
	if err := ValidateCustomSkillAction(ActionCustomSkill{CMPCost: 5}); !errors.Is(err, ErrUnsupportedEffect) {
		t.Fatalf("empty: %v", err)
	}
	ctx := &battleContext{cmpMap: map[string]int{"a": 10}, hpMap: map[string]int{}}
	// A supported heal with no eligible target is an accepted no-effect action.
	if err := ctx.executeCustomSkill(Participant{ID: "a"}, &ActionCustomSkill{CMPCost: 5, Gems: []GemEffect{{Kind: ActionKindHeal, Power: 10}}}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if ctx.cmpMap["a"] != 5 {
		t.Fatal("accepted no-effect action must charge CMP")
	}
}

func TestCustomSlotsStopOnActorDeath(t *testing.T) {
	actor := Participant{ID: "a", HP: 1, Attack: 100}
	ctx := &battleContext{
		hpMap: map[string]int{"a": 1}, mpMap: map[string]int{"a": 20}, cmpMap: map[string]int{"a": 20},
		statusMap: map[string]string{"a": StatusConfusion}, field: &FieldState{},
		allParticipants: []Participant{actor},
	}
	cs := ActionCustomSkill{CMPCost: 5, Gems: []GemEffect{
		{Kind: ActionKindAttack},
		{Kind: "field", Element: ElementFire, Duration: 3},
	}}
	if err := ctx.executeCustomSkill(actor, &cs, nil, nil); err != nil {
		t.Fatal(err)
	}
	if ctx.hpMap[actor.ID] != 0 || ctx.cmpMap[actor.ID] != 15 || len(ctx.logs) != 1 || !reflect.DeepEqual(ctx.field, &FieldState{}) {
		t.Fatalf("later slot executed after death: %+v", ctx)
	}
}
