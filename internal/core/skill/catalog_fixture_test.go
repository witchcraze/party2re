package skill

import corebattle "github.com/witchcraze/party2re/internal/core/battle"

// fillEmptyJobTables gives a partial fixture the full player/form domain that
// later-slot registration checks; production registers every table.
func fillEmptyJobTables(d *BattleDefinitions) {
	if d.Invocations.JobTables == nil {
		d.Invocations.JobTables = make(map[int]corebattle.CallbackTable)
	}
	for _, job := range append([]int{300}, jobRange()...) {
		if _, ok := d.Invocations.JobTables[job]; !ok {
			d.Invocations.JobTables[job] = corebattle.CallbackTable{Variants: [][]corebattle.CallbackEntry{nil}}
		}
	}
}

func jobRange() []int {
	jobs := make([]int, 0, 86)
	for job := 1; job <= 87; job++ {
		if job != 73 {
			jobs = append(jobs, job)
		}
	}
	return jobs
}
