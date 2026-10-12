package battle_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/random"
	"github.com/witchcraze/party2re/internal/core/skill"
	"github.com/witchcraze/party2re/internal/customskill"
)

func TestBuilderNativeAdmission(t *testing.T) {
	char := corecharacter.Character{ID: "a", SP: 100, Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 20, Attack: 10}}
	// Synthetic native fixtures certify only the simple operations exercised here,
	// not the similarly named legacy catalog effects.
	native := skill.BattleDefinitions{
		JobSkills: map[string]corebattle.ActionSkill{
			"supported": {Kind: corebattle.ActionKindAttack, Power: 20, TargetScope: corebattle.TargetScopeSingleEnemy},
			"unknown":   {Kind: "unknown"},
			"random":    {Kind: "random"},
		},
		Gems: map[string]corebattle.GemEffect{
			"field":     {Kind: "field", Element: corebattle.ElementFire, Duration: 3},
			"attack":    {Kind: corebattle.ActionKindAttack, Power: 20, Element: corebattle.ElementFire},
			"unknown":   {Kind: "unknown"},
			"random":    {Kind: "random"},
			"delegated": {Kind: "delegated"},
		},
	}
	jobs := []skill.Definition{
		{ID: "supported", Name: "Strike", MPCost: 5},
		{ID: "unknown", Effect: corebattle.Effect{Kind: "unknown"}},
		{ID: "random", Effect: corebattle.Effect{Kind: "random"}},
		{ID: "uncertified", Effect: corebattle.Effect{Kind: "attack", Power: 20}},
	}
	for _, tc := range []struct {
		name     string
		slots    [3]string
		accepted bool
	}{
		{"empty", [3]string{}, false},
		{"missing payload", [3]string{"missing"}, false},
		{"unknown", [3]string{"unknown"}, false},
		{"valid first unsupported second", [3]string{"attack", "unknown"}, false},
		{"absent random descendant", [3]string{"random"}, false},
		{"unsupported delegated descendant", [3]string{"delegated"}, false},
		{"ordered duplicates", [3]string{"field", "attack", "attack"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cs := &customskill.CustomSkill{Name: "Blend", CMP: 5, Gems: tc.slots}
			p, err := battle.BuildParticipantFromData(char, coreinventory.Inventory{}, coreequipment.Equipment{}, jobs, cs, native)
			if err != nil {
				t.Fatal(err)
			}
			if len(p.Skills) != 1 || p.Skills[0].ID != "supported" {
				t.Fatalf("skills: %+v", p.Skills)
			}
			action, admissionErr := native.CustomAction("custom-skill-a", cs.Name, cs.Comment, cs.CMP, cs.Gems)
			if !tc.accepted {
				if len(p.CustomSkills) != 0 || !errors.Is(admissionErr, corebattle.ErrUnsupportedEffect) {
					t.Fatalf("discovery and admission disagree: %+v, %v", p.CustomSkills, admissionErr)
				}
				return
			}
			if admissionErr != nil || len(p.CustomSkills) != 1 || !reflect.DeepEqual(p.CustomSkills[0], action) {
				t.Fatalf("hydration: %+v, %v", p.CustomSkills, admissionErr)
			}
			p.CMP = 5
			enemy := corebattle.Participant{ID: "e", HP: 1000, Attack: 0, Defending: true}
			res, err := (corebattle.Engine{}).ResolvePartyBattle(corebattle.PartyBattleRequest{Allies: []corebattle.Participant{p}, Enemies: []corebattle.Participant{enemy}, RNG: random.NewDeterministic(1)})
			if err != nil {
				t.Fatal(err)
			}
			if len(res.Logs) < 3 || res.Logs[0].DamageDealt != 0 || res.Logs[1].DamageDealt <= 0 || res.Logs[2].DamageDealt <= 0 || res.RemainingCMP[p.ID] != 0 {
				t.Fatalf("ordered execution: %+v", res)
			}
		})
	}
	p, err := battle.BuildParticipantFromData(char, coreinventory.Inventory{}, coreequipment.Equipment{}, jobs, &customskill.CustomSkill{Name: "Blend", Gems: [3]string{"attack"}}, skill.BattleDefinitions{})
	if err != nil || len(p.Skills) != 0 || len(p.CustomSkills) != 0 {
		t.Fatalf("uncertified defaults: %+v, %v", p, err)
	}
}

func TestForgedUnsupportedBatchRejectedBeforeMutation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		alter func(*corebattle.Participant)
	}{
		{"job", func(p *corebattle.Participant) { p.Skills = []corebattle.ActionSkill{{Kind: "unknown", MPCost: 5}} }},
		{"empty custom", func(p *corebattle.Participant) { p.CustomSkills = []corebattle.ActionCustomSkill{{CMPCost: 5}} }},
		{"later slot", func(p *corebattle.Participant) {
			p.CustomSkills = []corebattle.ActionCustomSkill{{CMPCost: 5, Gems: []corebattle.GemEffect{{Kind: "attack"}, {Kind: "random"}}}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := corebattle.Participant{ID: "a", HP: 100, MP: 20, CMP: 20}
			tc.alter(&p)
			field := &corebattle.FieldState{}
			req := corebattle.PartyBattleRequest{Allies: []corebattle.Participant{p}, Enemies: []corebattle.Participant{{ID: "e", HP: 100}}, InitialField: field}
			res, err := (corebattle.Engine{}).ResolvePartyBattle(req)
			if !errors.Is(err, corebattle.ErrUnsupportedEffect) || !reflect.DeepEqual(res, corebattle.PartyBattleResult{}) || req.Allies[0].HP != 100 || req.Allies[0].MP != 20 || req.Allies[0].CMP != 20 || !reflect.DeepEqual(field, &corebattle.FieldState{}) {
				t.Fatalf("rejection: %+v %v", res, err)
			}
		})
	}
}
