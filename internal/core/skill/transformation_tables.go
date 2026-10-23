package skill

import corebattle "github.com/witchcraze/party2re/internal/core/battle"

// Full source order at _skill.cgi:2910–2966,6240–6304,6537–6612,7465–7532.
// Unbound positions prevent these contributions from narrowing random pools.
func transformationTables(c *corebattle.TransformationContext) map[int]corebattle.CallbackTable {
	breath := []corebattle.CallbackEntry{
		{ID: "job-79-entry-0", Name: "drain", RequiredSP: 11, MPCost: 2},
		{ID: "job-79-entry-1", Name: "bite", RequiredSP: 33, MPCost: 8},
		{ID: "job-79-entry-2", Name: "time", RequiredSP: 55, MPCost: 30},
		{ID: "job-79-breath", Name: "翼形変化", RequiredSP: 77, MPCost: 21},
		{ID: "job-79-entry-4", Name: "tension-bite", RequiredSP: 99, MPCost: 27},
	}
	speed := append([]corebattle.CallbackEntry(nil), breath...)
	speed[3].ID, speed[3].Name = "job-79-speed", "翼形解除"
	return map[int]corebattle.CallbackTable{
		36: {Variants: [][]corebattle.CallbackEntry{{
			{ID: "job-36-entry-0", Name: "dance-counter", RequiredSP: 10, MPCost: 5},
			{ID: "job-36-entry-1", Name: "breath-counter", RequiredSP: 20, MPCost: 5},
			{ID: "job-36-entry-2", Name: "magic-counter", RequiredSP: 40, MPCost: 5},
			{ID: "job-36-entry-3", Name: "attack-counter", RequiredSP: 60, MPCost: 5},
			{ID: "job-36-copy", Name: "姿写し", RequiredSP: 100, MPCost: 50},
		}}},
		79: {Variants: [][]corebattle.CallbackEntry{breath, speed}, Select: func(s *corebattle.LifetimeState, frame *corebattle.CallbackContext) int {
			if c == nil {
				return -1
			}
			for _, a := range s.Actors {
				if a.ID == frame.ActorID && a.TransformationEntries("job-79", c.RequesterGender)[0].ID == "job-79-speed" {
					return 1
				}
			}
			return 0
		}},
		82: {Variants: [][]corebattle.CallbackEntry{{
			{ID: "job-82-entry-0", Name: "generated", RequiredSP: 10},
			{ID: "job-82-entry-1", Name: "hammer", RequiredSP: 25, MPCost: 7},
			{ID: "job-82-entry-2", Name: "tea", RequiredSP: 50, MPCost: 23},
			{ID: "job-82-entry-3", Name: "tune", RequiredSP: 65, MPCost: 3},
			{ID: "job-82-entry-4", Name: "arrows", RequiredSP: 80, MPCost: 11},
			{ID: "job-82-entry-5", Name: "cannon", RequiredSP: 111, MPCost: 20},
			{ID: "job-82-form", Name: "形態切替", RequiredSP: 111, MPCost: 10},
		}}},
		300: {Variants: [][]corebattle.CallbackEntry{{
			{ID: "job-300-tune", Name: "機構調整", MPCost: 30},
			{ID: "job-300-strike", Name: "防護貫撃", MPCost: 48},
			{ID: "job-300-burst", Name: "連装放射", MPCost: 99},
			{ID: "job-300-form", Name: "形態切替"},
		}}},
	}
}
