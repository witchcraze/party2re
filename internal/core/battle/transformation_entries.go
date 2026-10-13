package battle

import "strings"

// TransformationEntry describes only this effect owner's subset of a job
// table. It never substitutes for a complete random/delegated closure.
type TransformationEntry struct {
	ID, Name           string
	RequiredSP, MPCost int
}

func nativeFormIcon(form, gender string) string {
	return "assets/images/placeholders/battle/forms.svg#" + form + "-" + gender
}

// TransformationEntries is rebuilt against the current actor appearance and
// requesting player's gender, rather than cached by original job or name.
// The initial general-random pool remains jobs1..87, excluding300 (#1357).
func (a LifetimeActor) TransformationEntries(jobID, requesterGender string) []TransformationEntry {
	switch jobID {
	case "job-36":
		return []TransformationEntry{{"job-36-copy", "姿写し", 100, 50}}
	case "job-79":
		if strings.Contains(a.Icon, nativeFormIcon("winged", requesterGender)) {
			return []TransformationEntry{{"job-79-speed", "翼形解除", 77, 21}}
		}
		return []TransformationEntry{{"job-79-breath", "翼形変化", 77, 21}}
	case "job-82":
		return []TransformationEntry{{"job-82-form", "形態切替", 111, 10}}
	case "job-300":
		return []TransformationEntry{
			{"job-300-tune", "機構調整", 0, 30},
			{"job-300-strike", "防護貫撃", 0, 48},
			{"job-300-burst", "連装放射", 0, 99},
			{"job-300-form", "形態切替", 0, 0},
		}
	}
	return nil
}

// BetweenEncounterForm follows name-keyed discovery: reverse is registered
// last and wins when both forms coexist. Callback execution still prefers the
// current matching job over the previous matching job, independently of SP.
func (a LifetimeActor) BetweenEncounterForm(round, enemies int) (TransformationEntry, bool) {
	if round <= 0 || enemies != 0 || hasAbility(a.Abilities, "cursed") {
		return TransformationEntry{}, false
	}
	if a.Jobs.JobID == "job-300" || a.Jobs.OldJobID == "job-300" {
		return a.TransformationEntries("job-300", "")[3], true
	}
	if (a.Jobs.JobID == "job-82" && a.Jobs.SP >= 111) || (a.Jobs.OldJobID == "job-82" && a.Jobs.OldSP >= 111) {
		return a.TransformationEntries("job-82", "")[0], true
	}
	return TransformationEntry{}, false
}
