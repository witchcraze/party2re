package skill

import (
	"encoding/json"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

func TestCallingSpecialCommittedFactsReloadWithoutCallbackReplay(t *testing.T) {
	for _, row := range []struct {
		job     int
		command string
	}{
		{61, "special"}, {62, "special"}, {62, "drain-barrage"}, {63, "special"},
		{64, "special"}, {66, "special"}, {67, "special"}, {71, "special"}, {80, "special"}, {87, "special"},
	} {
		entry := corebattle.RosterEntries(row.job)[3]
		if row.command == "drain-barrage" {
			entry = corebattle.RosterEntries(row.job)[6]
		}
		t.Run(entry.ID, func(t *testing.T) {
			s, c, d := callingManifestFixture()
			addCallingManifestHelper(&s, &c, row.job)
			d.RegisterCallingJobs(&c)
			f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "tail"}}
			ops := []corebattle.CallbackInvocation{{Kind: "entry", EntryID: entry.ID}}
			// The first intrinsic callback commits once; no executable closure is
			// serialized. The next operation starts from those resulting facts.
			if err := s.ApplyInvocations(&f, d.Invocations, ops, callingZeroDraw{}); err != nil {
				t.Fatal(err)
			}
			type snapshot struct {
				State            corebattle.LifetimeState
				Target           corebattle.TargetContext
				Arguments        []string
				Counts           [12]int
				AdditionalEffect bool
				RequesterLevel   int
				RequesterItemID  string
			}
			counts := func(c *corebattle.RosterContext) [12]int {
				return [12]int{c.DamageCalls, c.AreaDamageCalls, c.HealCalls, c.PartyHealCalls, c.MPHealCalls, c.StatUpCalls, c.StatDownCalls, c.StatusCalls, c.AreaStatusCalls, c.DeathCalls, c.AreaDeathCalls, c.PartyMPHealCalls}
			}
			frozen := snapshot{State: s, Target: f.TargetContext, Arguments: f.Arguments, Counts: counts(&c), AdditionalEffect: c.AdditionalEffect, RequesterLevel: *c.RequesterLevel, RequesterItemID: c.RequesterItemID}
			data, err := json.Marshal(frozen)
			if err != nil {
				t.Fatal(err)
			}
			var loaded snapshot
			if err := json.Unmarshal(data, &loaded); err != nil {
				t.Fatal(err)
			}
			_, resumed, _ := callingManifestFixture()
			resumed.TargetContext = loaded.Target
			resumed.RequesterLevel, resumed.RequesterItemID = &loaded.RequesterLevel, loaded.RequesterItemID
			resumed.AdditionalEffect = loaded.AdditionalEffect
			resumed.DamageCalls, resumed.AreaDamageCalls, resumed.HealCalls, resumed.PartyHealCalls = loaded.Counts[0], loaded.Counts[1], loaded.Counts[2], loaded.Counts[3]
			resumed.MPHealCalls, resumed.StatUpCalls, resumed.StatDownCalls = loaded.Counts[4], loaded.Counts[5], loaded.Counts[6]
			resumed.StatusCalls, resumed.AreaStatusCalls, resumed.DeathCalls, resumed.AreaDeathCalls, resumed.PartyMPHealCalls = loaded.Counts[7], loaded.Counts[8], loaded.Counts[9], loaded.Counts[10], loaded.Counts[11]
			rebuilt := BattleDefinitions{}
			rebuilt.RegisterCallingJobs(&resumed)
			rf := corebattle.CallbackContext{TargetContext: loaded.Target, Arguments: loaded.Arguments}
			// A different fixed operation seed rules out replaying the committed
			// callback to reconstruct its counters or growth.
			if err := s.ApplyInvocations(&f, d.Invocations, ops, random.NewDeterministic(1479)); err != nil {
				t.Fatal(err)
			}
			if err := loaded.State.ApplyInvocations(&rf, rebuilt.Invocations, ops, random.NewDeterministic(1479)); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(s, loaded.State) || !reflect.DeepEqual(f.TargetContext, rf.TargetContext) || counts(&c) != counts(&resumed) || c.AdditionalEffect != resumed.AdditionalEffect {
				t.Fatal(s, loaded.State, c, resumed)
			}
		})
	}
}
