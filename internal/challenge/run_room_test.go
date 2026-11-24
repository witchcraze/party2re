package challenge

import (
	"slices"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/runhost"
)

// add_battle_action (vs_challenge.cgi:21–30): しらべる and no NPC response
// while a box remains, すすむ only once neither enemies nor boxes remain.
func TestTreasureWaveChoices(t *testing.T) {
	actions := func(r *Run) []string {
		var got []string
		for _, c := range (stageRoom{}).Choices(r, "hero") {
			got = append(got, c.Source)
		}
		return got
	}
	r := &Run{}
	r.Boxes = []runhost.TreasureBox{{ID: "box-1"}, {ID: "box-2", OpenedBy: "hero"}}
	if got := actions(r); !slices.Equal(got, []string{"しらべる"}) || (stageRoom{}).Responds(r) {
		t.Fatalf("box wave: %v responds=%v", got, (stageRoom{}).Responds(r))
	}
	r.Boxes[0].Defeated = true
	if got := actions(r); !slices.Equal(got, []string{"すすむ"}) || !(stageRoom{}).Responds(r) {
		t.Fatalf("emptied wave: %v", got)
	}
	r.Battle.Actors = []corebattle.LifetimeActor{{Participant: corebattle.Participant{ID: "slime", HP: 1}, NPC: true}}
	r.Enemies = []string{"slime"}
	if got := actions(r); len(got) != 0 || !(stageRoom{}).Responds(r) {
		t.Fatalf("monster wave: %v", got)
	}
}
