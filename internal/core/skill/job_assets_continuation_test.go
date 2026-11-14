package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/economy"
)

func TestJobAssetReloadAndIndirectMPRestoration(t *testing.T) {
	s, c, f, d := jobAssetFixture()
	executeJobAsset(t, &s, &f, d, "job-7-entry-2", 0)
	type checkpoint struct {
		State       corebattle.LifetimeState
		Assets      corebattle.JobAssets
		Target      corebattle.TargetContext
		DamageCalls int
	}
	data, err := json.Marshal(checkpoint{s, *f.Assets, f.TargetContext, c.DamageCalls})
	if err != nil {
		t.Fatal(err)
	}
	var saved checkpoint
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	saved.Assets.AddWallet = economy.SafeAdd
	rebuilt := &corebattle.JobAssetContext{DamageContext: c.DamageContext}
	rebuilt.DamageCalls = saved.DamageCalls
	loaded := corebattle.CallbackContext{TargetContext: saved.Target, Assets: &saved.Assets, Arguments: []string{"enemy"}}
	f.Arguments = []string{"enemy"}
	var definitions BattleDefinitions
	definitions.RegisterPlayerCallbacks(PlayerCallbackContexts{JobAssets: rebuilt}, 2)
	executeJobAsset(t, &s, &f, d, "job-78-entry-5", 0, 0, 0)
	executeJobAsset(t, &saved.State, &loaded, definitions, "job-78-entry-5", 0, 0, 0)
	if !reflect.DeepEqual(s, saved.State) || f.Assets.Wallet != 108 || saved.Assets.Wallet != 108 || *f.Assets.HeldToolID != "item-001" || *saved.Assets.HeldToolID != "item-001" || c.DamageCalls != rebuilt.DamageCalls {
		t.Fatal(s, saved, c, rebuilt)
	}
	for _, kind := range []string{"entry", "tool-84", "random"} {
		s, c, f, d := jobAssetFixture()
		s.Actors[0].Abilities = []string{"seal_kotowari"}
		id := "job-7-entry-2"
		draws := []float64{0}
		if kind != "entry" {
			// Synthetic tables isolate the real body's random MP refund; they
			// do not certify the complete original random pool.
			d.Invocations.JobTables = map[int]corebattle.CallbackTable{}
			for job := 1; job <= 87; job++ {
				d.Invocations.JobTables[job] = corebattle.CallbackTable{Variants: [][]corebattle.CallbackEntry{{}}}
			}
			d.Invocations.JobTables[1] = corebattle.CallbackTable{Variants: [][]corebattle.CallbackEntry{{corebattle.IncidentalJobEntries(7)[2]}}}
			s.Actors[0].Jobs.JobID = "job-1"
			id = "indirect"
			d.Invocations.Entries[id] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: kind}}
			draws = append([]float64{0}, draws...)
			if kind == "random" {
				draws = append([]float64{0}, draws...)
			}
		}
		executeJobAsset(t, &s, &f, d, id, draws...)
		wantMP := 80
		if kind == "entry" {
			wantMP = 77
		}
		if s.Actors[0].MP != wantMP || f.Assets.Wallet <= 100 || c.DamageCalls != 1 {
			t.Fatal(kind, s, f.Assets, c)
		}
	}
}

func TestJobAssetReflectedDamageStillCreditsRequester(t *testing.T) {
	s, c, f, d := jobAssetFixture()
	s.Actors[1].Temporary = corebattle.TemporaryReflectAttack
	executeJobAsset(t, &s, &f, d, "job-7-entry-2", 0)
	if s.Actors[0].HP != 464 || s.Actors[1].HP != 1000 || f.Assets.Wallet != 108 || c.DamageCalls != 1 {
		t.Fatal(s, c, f.Assets)
	}
}

func TestJobAssetWalletWaveRetainsDebitAndAreaOrder(t *testing.T) {
	s, c, f, d := jobAssetFixture()
	second := s.Actors[1].Clone()
	second.ID = "second"
	s.Actors = append(s.Actors, second)
	f.EnemyIDs = append(f.EnemyIDs, second.ID)
	executeJobAsset(t, &s, &f, d, "job-28-entry-3", 0, 0)
	if s.Actors[1].HP != 838 || s.Actors[3].HP != 863 || f.Assets.Wallet != 0 || c.AreaDamageCalls != 1 || c.DamageCalls != 2 {
		t.Fatal(s, c, f.Assets)
	}
	s, _, f, d = jobAssetFixture()
	s.Actors[0].Status = corebattle.StatusPhysicalSeal
	executeJobAsset(t, &s, &f, d, "job-28-entry-3", .99)
	if f.Assets.Wallet != 0 || s.Actors[1].HP != 1000 {
		t.Fatal("sealed wave lost its debit", s, f.Assets)
	}
}

func TestJobAssetNativeDefeatCompletesBeforeCredit(t *testing.T) {
	s, c, f, d := jobAssetFixture()
	_, native, _ := counterResetFixture()
	f.Defeat = native.StatusDefeat
	c.Defeat = nil
	s.Actors[1].HP = 1
	executeJobAsset(t, &s, &f, d, "job-81-entry-2", 0, 0)
	if s.Actors[1].HP != 0 || s.Actors[0].HP != 536 || f.Assets.Wallet != 121 || c.HealCalls != 1 {
		t.Fatal(s, c, f.Assets)
	}
}

func TestJobAssetLethalClosureRejectsBeforeWalletMutation(t *testing.T) {
	for _, id := range []string{"job-7-entry-0", "job-7-entry-2", "job-8-entry-0", "job-28-entry-3", "job-78-entry-5", "job-81-entry-2"} {
		s, c, f, d := jobAssetFixture()
		s.Actors[1].HP = 1
		before, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := d.InvocationAction(&s, &f, []string{id}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(id, err)
		}
		if err := s.ApplyInvocations(&f, d.Invocations, []corebattle.CallbackInvocation{{Kind: "entry", EntryID: id}}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(id, err)
		}
		after, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		if string(before) != string(after) || f.Assets.Wallet != 100 || c.DamageCalls != 0 {
			t.Fatal(id, s, c, f.Assets)
		}
	}
}

func TestJobAssetNPCStatTailUsesActualActorRole(t *testing.T) {
	s, c, f, d := jobAssetFixture()
	level := 0
	c.RequesterLevel = &level
	s.Actors[2] = s.Actors[0].Clone()
	s.Actors[2].ID, s.Actors[2].NPC = "helper", true
	s.Actors[2].ItemDefinitionIDs = []string{"item-145", "item-205"}
	f.ActorID = "helper"
	f.PartyIDs = []string{"self", "helper"}
	executeJobAsset(t, &s, &f, d, "job-78-entry-5", 0, 0, 0, 0)
	if !c.ActorNPC || s.Actors[2].Agility != 50 || s.Actors[2].Tension != 1.7 || *f.Assets.HeldToolID != "" {
		t.Fatal(s, c, f.Assets)
	}
}
