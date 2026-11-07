package skill

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

// Independent positional ledger: #1409, ff97fc5:_skill.cgi. Generated MP and
// party-sized MP are construction facts, not fixed scalar costs.
func TestPlayerCallbackManifest(t *testing.T) {
	data, err := os.ReadFile("testdata/player_callback_manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []struct {
		Job, Position int
		ID            string
		SP            int
		MP            string
		Extended      bool
	}
	if err := json.Unmarshal(data, &rows); err != nil {
		t.Fatal(err)
	}
	var d BattleDefinitions
	d.RegisterPlayerCallbacks(PlayerCallbackContexts{}, 3)
	if len(d.Invocations.JobTables) != 88 {
		t.Fatalf("tables = %d", len(d.Invocations.JobTables))
	}
	ordinary, extended := 0, 0
	for _, row := range rows {
		table := d.Invocations.JobTables[row.Job]
		if len(table.Variants) == 0 || len(table.Variants[0]) <= row.Position {
			t.Errorf("missing %d.%d", row.Job, row.Position)
			continue
		}
		entry := table.Variants[0][row.Position]
		if entry.ID != strings.Split(row.ID, " / ")[0] || entry.RequiredSP != row.SP || entry.Extended != row.Extended {
			t.Errorf("%d.%d: %+v; want %+v", row.Job, row.Position, entry, row)
		}
		if mp, err := strconv.Atoi(row.MP); err == nil && entry.MPCost != mp {
			t.Errorf("%d.%d MP=%d want %d", row.Job, row.Position, entry.MPCost, mp)
		}
		if row.MP == "12×party size" && entry.MPCost != 36 {
			t.Fatal(entry)
		}
		if row.Job >= 1 && row.Job <= 87 {
			if entry.Extended {
				extended++
			} else {
				ordinary++
			}
		}
	}
	if ordinary != 548 || extended != 51 {
		t.Fatal(ordinary, extended)
	}
	for job, table := range d.Invocations.JobTables {
		count := 0
		for _, row := range rows {
			if row.Job == job {
				count++
			}
		}
		if len(table.Variants[0]) != count {
			t.Fatal(fmt.Sprintf("job%d positions=%d want%d", job, len(table.Variants[0]), count))
		}
	}
}

func TestPlayerCallbackRegistrationKeepsAssetOwnersExplicit(t *testing.T) {
	var d BattleDefinitions
	d.RegisterPlayerCallbacks(PlayerCallbackContexts{}, 3)
	var missing []string
	for _, table := range d.Invocations.JobTables {
		for _, entry := range table.Variants[0] {
			if _, ok := d.Invocations.Entries[entry.ID]; !ok {
				missing = append(missing, entry.ID)
			}
		}
	}
	slices.Sort(missing)
	want := []string{
		"job-50-entry-7",
		"job-86-entry-0", "job-86-entry-2", "job-86-entry-3", "job-86-entry-5",
	}
	slices.Sort(want)
	if !slices.Equal(missing, want) {
		t.Fatalf("missing bindings=%v want explicit asset owners=%v", missing, want)
	}
}

// Exhaust a full joint residue period, retaining positions excluded only from
// generated selection. Every ordinary/random/tool table remains unchanged.
func TestPlayerGeneratedReachablePositions(t *testing.T) {
	s, c, d := playerDispatchFixture()
	f := corebattle.CallbackContext{TargetContext: c.TargetContext}
	maximum := 0
	for job, table := range d.Invocations.JobTables {
		if job > 0 && job < 87 && job != 73 && job != 82 {
			maximum = max(maximum, len(table.Variants[0]))
		}
	}
	reached := map[string]bool{}
	for sum := len(s.Actors); sum < len(s.Actors)+87*maximum; sum++ {
		s.Actors[0].Attack, s.Actors[0].Defense, s.Actors[0].Agility = sum-len(s.Actors), 0, 0
		entries, err := d.PlayerCallbackTable(&s, &f, "job-82")
		if err != nil {
			t.Fatal(sum, err)
		}
		job := sum % 87
		if job == 0 || job == 73 || job == 82 {
			job = 8
		}
		table := d.Invocations.JobTables[job].Variants[0]
		expected := table[sum%len(table)]
		if entries[0].Invocation.GeneratedChildID != expected.ID || entries[0].MPCost != expected.MPCost {
			t.Fatal(sum, entries[0], expected)
		}
		reached[expected.ID] = true
	}
	if len(reached) != 430 {
		t.Fatalf("generated reachable=%d want430", len(reached))
	}
	eligible := 0
	for job, table := range d.Invocations.JobTables {
		if job > 0 && job < 87 && job != 73 && job != 82 {
			eligible += len(table.Variants[0])
		}
	}
	if eligible != 586 || eligible-len(reached) != 156 {
		t.Fatal(eligible, len(reached))
	}
}
