package battle_test

import (
	"reflect"
	"testing"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestCountStrongPvPKills(t *testing.T) {
	// Strong(actor) = 100 + 20 + 40 + int(20*0.5) + 30 = 200; threshold = 100
	actor := corebattle.Participant{ID: "actor", HP: 100, MaxHP: 100, MP: 20, MaxMP: 20, Attack: 40, Defense: 20, Agility: 30}
	// Strong = 100 -> equal to actor * 0.5 -> NOT strong (must be strictly >)
	exactHalf := corebattle.Participant{ID: "half", HP: 100, MaxHP: 100}
	// Strong = 101 -> strictly > actor * 0.5 -> STRONG
	justAboveHalf := corebattle.Participant{ID: "strong-1", HP: 101, MaxHP: 101}
	// Fallback when MaxHP/MaxMP are 0
	fallbackStrong := corebattle.Participant{ID: "strong-2", HP: 120, MP: 10}

	participants := []corebattle.Participant{actor, exactHalf, justAboveHalf, fallbackStrong}
	defeats := []corebattle.PartyBattleDefeat{
		{ActorID: "actor", TargetID: "half"},
		{ActorID: "actor", TargetID: "strong-1"},
		{ActorID: "actor", TargetID: "strong-2"},
		{ActorID: "actor", TargetID: "actor"},   // self ignored
		{ActorID: "unknown", TargetID: "half"},  // unknown ignored
		{ActorID: "actor", TargetID: "missing"}, // missing target ignored
	}

	got := battle.CountStrongPvPKills(participants, defeats)
	want := map[string]int{"actor": 2}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CountStrongPvPKills = %+v, want %+v", got, want)
	}

	if got := battle.CountStrongPvPKills(participants, []corebattle.PartyBattleDefeat{{ActorID: "actor", TargetID: "half"}}); got != nil {
		t.Fatalf("expected nil map for weak-only defeat, got %+v", got)
	}
}

func TestMergeKillCountsAndSortedIDs(t *testing.T) {
	base := map[string]int{"c": 1, "a": 2}
	delta := map[string]int{"b": 3, "a": 1, "zero": 0}
	merged := battle.MergeKillCounts(base, delta)
	want := map[string]int{"a": 3, "b": 3, "c": 1}
	if !reflect.DeepEqual(merged, want) {
		t.Fatalf("MergeKillCounts = %+v, want %+v", merged, want)
	}
	if base["a"] != 2 {
		t.Fatalf("MergeKillCounts mutated base map: %+v", base)
	}
	ids := battle.SortedPositiveKillIDs(merged)
	if !reflect.DeepEqual(ids, []string{"a", "b", "c"}) {
		t.Fatalf("SortedPositiveKillIDs = %v, want [a b c]", ids)
	}
}
