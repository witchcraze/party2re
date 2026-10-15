package battle

import (
	"strconv"
	"strings"
)

// RosterEntries retains complete original calling-table positions. An entry's
// presence is evidence, not admission of its status/MP/field descendants.
func RosterEntries(job int) []CallbackEntry {
	if !isCallingJob(job) {
		return nil
	}
	prefix := "job-" + strconv.Itoa(job) + "-"
	entries := []CallbackEntry{
		{ID: prefix + "summon", Name: "summon", RequiredSP: 10, MPCost: 30},
		{ID: prefix + "dismiss", Name: "dismiss", RequiredSP: 11},
		{ID: prefix + "attack", Name: "attack", RequiredSP: 12},
		{ID: prefix + "special", Name: "special", RequiredSP: 30, MPCost: 30},
		{ID: prefix + "defend", Name: "defend", RequiredSP: 100, MPCost: 10},
	}
	name, cost := "merge", 100
	switch job {
	case 61:
		name, cost = "revive-party", 42
	case 62:
		name, cost = "rain", 37
	case 65:
		name, cost = "barrage", 40
	case 66:
		name, cost = "fallen-power", 50
	case 67:
		name, cost = "double-tail", 40
	case 68:
		name, cost = "party-heal", 50
	case 69:
		name, cost = "share-mp", 20
	case 80:
		name, cost = "distribution", 20
	case 87:
		name, cost = "venom", 58
	}
	entries = append(entries, CallbackEntry{ID: prefix + name, Name: name, RequiredSP: 150, MPCost: cost})
	ext, exCost := "", 0
	switch job {
	case 59:
		ext, exCost = "ride", 15
	case 62:
		ext, exCost = "drain-barrage", 53
	case 63:
		ext = "duplicate"
	case 64:
		ext, exCost = "grave", 1
	case 66:
		ext = "revive"
	case 69:
		ext, exCost = "small", 15
	}
	if ext != "" {
		entries = append(entries, CallbackEntry{ID: prefix + ext, Name: ext, RequiredSP: 150, MPCost: exCost, Extended: true})
	}
	return entries
}

func rosterCommand(id string) (int, string, bool) {
	parts := strings.SplitN(id, "-", 3)
	if len(parts) != 3 || parts[0] != "job" {
		return 0, "", false
	}
	job, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, "", false
	}
	for _, entry := range RosterEntries(job) {
		if entry.ID == id {
			return job, entry.Name, true
		}
	}
	return 0, "", false
}
