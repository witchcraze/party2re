package skill

import (
	"errors"
	"strings"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestPlayerCanonicalMetadataAndProxyResidues(t *testing.T) {
	s, c, d := playerDispatchFixture()
	f := corebattle.CallbackContext{TargetContext: c.TargetContext}
	for _, job := range []string{"job-01", "job-05", "job-06"} {
		entries, err := d.PlayerCallbackTable(&s, &f, job)
		if err != nil || len(entries) == 0 || entries[0].RequiredJobIDs[0] != job {
			t.Fatal(job, entries, err)
		}
	}
	if _, err := d.PlayerCallbackTable(&s, &f, "job-5"); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	for residue := range 73 {
		s.Actors[0].Attack, s.Actors[0].Defense, s.Actors[0].Agility = residue, 0, 0
		proxy, err := s.CallbackBindings(&f, d.Invocations, 73)
		if err != nil {
			t.Fatal(err)
		}
		job := residue
		if job == 0 {
			job = 8
		}
		if proxy[0].ID != d.Invocations.JobTables[job].Variants[0][0].ID {
			t.Fatal(residue, proxy)
		}
	}
}

func TestPlayerTableVariantsRetainCapturedIdentity(t *testing.T) {
	s, c, _ := baseGuardFixture()
	var d BattleDefinitions
	d.RegisterPlayerCallbacks(PlayerCallbackContexts{Transformations: &corebattle.TransformationContext{RequesterGender: "f"}}, 3)
	f := corebattle.CallbackContext{TargetContext: c.TargetContext}
	s.Actors[0].Temporary = corebattle.TemporaryDefense
	s.Actors[0].Icon = "assets/images/placeholders/battle/forms.svg#winged-f"
	dual, err := d.PlayerCallbackTable(&s, &f, "job-77")
	if err != nil {
		t.Fatal(err)
	}
	wing, err := d.PlayerCallbackTable(&s, &f, "job-79")
	if err != nil {
		t.Fatal(err)
	}
	s.Actors[0].Temporary, s.Actors[0].Icon = "", ""
	if dual[4].Invocation.EntryID != "job-77-entry-4-temporary" || wing[3].Invocation.EntryID != "job-79-speed" {
		t.Fatal(dual[4], wing[3])
	}
	dual, err = d.PlayerCallbackTable(&s, &f, "job-77")
	if err != nil || dual[4].ID != "job-77-entry-4" {
		t.Fatal(dual, err)
	}
}

// Ordinary item158 discovery/current and previous SP differ from gem86.
func TestPlayerShiftGearDiscoveryExecutionAndCaps(t *testing.T) {
	for _, previous := range []bool{false, true} {
		for _, attack := range []int{998, 999, 1000} {
			for _, missing := range []string{"", "sp", "item", "mp", "job"} {
				s, c, _ := baseGuardFixture()
				var d BattleDefinitions
				d.RegisterPlayerCallbacks(PlayerCallbackContexts{}, len(c.PartyIDs))
				a := &s.Actors[0]
				a.Attack, a.Defense, a.Agility, a.MP = attack, 0, 0, 1
				a.Jobs = corebattle.BattleJobs{JobID: "job-01", OldJobID: "job-01"}
				id := "job-73-gear-current"
				if previous {
					a.Jobs.OldJobID, a.Jobs.OldSP = "job-73", 1
					id = "job-73-gear-previous"
				} else {
					a.Jobs.JobID, a.Jobs.SP = "job-73", 1
				}
				a.ItemDefinitionIDs = []string{"item-158"}
				switch missing {
				case "sp":
					if previous {
						a.Jobs.OldSP = 0
					} else {
						a.Jobs.SP = 0
					}
				case "item":
					a.ItemDefinitionIDs = nil
				case "mp":
					a.MP = 0
				case "job":
					a.Jobs.JobID, a.Jobs.OldJobID = "job-01", "job-01"
				}
				f := corebattle.CallbackContext{TargetContext: c.TargetContext}
				entries, err := d.PlayerCallbackTable(&s, &f, "job-73")
				if err != nil {
					t.Fatal(err)
				}
				found := false
				for _, entry := range entries {
					if strings.HasPrefix(entry.ID, "job-73-gear-") {
						found = true
						if entry.ID != id || entry.MPCost != 1 || entry.RequiredSP != 0 {
							t.Fatal(entry)
						}
					}
				}
				if found != (missing == "") {
					t.Fatal(missing, found)
				}
				ops, err := d.InvocationAction(&s, &f, []string{id}, noArithmeticDraws{t})
				if missing != "" {
					if !errors.Is(err, corebattle.ErrUnsupportedEffect) {
						t.Fatal(missing, err)
					}
					continue
				}
				if err != nil {
					t.Fatal(err)
				}
				if err := s.ApplyInvocations(&f, d.Invocations, ops, noArithmeticDraws{t}); err != nil {
					t.Fatal(err)
				}
				want := attack
				if attack < 999 {
					want += len(c.PartyIDs)
				}
				if a.Attack != want || a.MP != 1 {
					t.Fatal(a.Attack, want, a.MP)
				}
			}
		}
	}
}
