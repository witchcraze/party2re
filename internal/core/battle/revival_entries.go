package battle

// RevivalEntries is this owner's concrete subset, not a complete job table.
// IDs retain source positions; names are generic native display names.
func RevivalEntries() []CallbackEntry {
	return []CallbackEntry{
		{ID: "job-3-entry-6", Name: "献命回復", RequiredSP: 60, MPCost: 1},
		{ID: "job-5-entry-7", Name: "半蘇生", RequiredSP: 60, MPCost: 20},
		{ID: "job-7-entry-8", Name: "献命終止", RequiredSP: 80, MPCost: 1},
		{ID: "job-14-entry-4", Name: "献命の舞", RequiredSP: 45, MPCost: 1},
		{ID: "job-16-entry-6", Name: "小蘇生", RequiredSP: 100, MPCost: 40},
		{ID: "job-17-entry-4", Name: "献命回復", RequiredSP: 100, MPCost: 1},
		{ID: "job-17-entry-6", Name: "完全蘇生", RequiredSP: 160, MPCost: 80},
		{ID: "job-18-entry-4", Name: "蘇生の声", RequiredSP: 70, MPCost: 27},
		{ID: "job-18-entry-6", Name: "完全蘇生", RequiredSP: 120, MPCost: 80},
		{ID: "job-32-entry-4", Name: "転生の炎", RequiredSP: 100, MPCost: 40},
		{ID: "job-33-entry-6", Name: "完全蘇生", RequiredSP: 160, MPCost: 80},
		{ID: "job-39-entry-5", Name: "半蘇生", RequiredSP: 50, MPCost: 20},
		{ID: "job-43-entry-6", Name: "完全蘇生", RequiredSP: 150, MPCost: 70},
		{ID: "job-50-entry-6", Name: "献命全快", RequiredSP: 145, MPCost: 1},
		{ID: "job-51-entry-5", Name: "希望の光", RequiredSP: 130, MPCost: 30},
		{ID: "job-57-entry-4", Name: "半蘇生", RequiredSP: 80, MPCost: 20},
		{ID: "job-66-revive", Name: "従者の蘇生", RequiredSP: 150, Extended: true},
		{ID: "job-70-entry-3", Name: "蘇生の声", RequiredSP: 200, MPCost: 27},
		{ID: "job-75-entry-5", Name: "魔力転生", RequiredSP: 120},
		{ID: "job-76-entry-3", Name: "献命の舞", RequiredSP: 90, MPCost: 1},
		{ID: "tool-6", Name: "蘇生の葉"},
		{ID: "book-173", Name: "半蘇生の書", MPCost: 80},
		{ID: "book-196", Name: "完全蘇生の書", MPCost: 180},
		{ID: "gem-63", Name: "半蘇生の珠"},
		{ID: "gem-64", Name: "完全蘇生の珠"},
		{ID: "gem-65", Name: "蘇生の声の珠"},
		{ID: "gem-66", Name: "献命の珠"},
		{ID: "extension-261", Name: "呪いの蘇生"},
	}
}

type revivalBody struct {
	mode             string
	fraction, chance float64
}

func revivalSpec(id string) (revivalBody, bool) {
	b := revivalBody{mode: "single", fraction: 1, chance: 1}
	switch id {
	case "tool-6", "book-196", "gem-64", "job-17-entry-6", "job-18-entry-6", "job-33-entry-6", "job-43-entry-6":
	case "book-173", "job-5-entry-7", "job-39-entry-5", "job-57-entry-4":
		b.fraction, b.chance = .5, .5
	case "gem-63":
		b.fraction = .5
	case "job-16-entry-6":
		b.fraction = .25
	case "gem-65", "job-18-entry-4", "job-70-entry-3":
		b.mode, b.fraction, b.chance = "party", .5, .5
	case "job-32-entry-4":
		b.mode, b.fraction = "party", .15
	case "job-51-entry-5":
		b.mode, b.chance = "party", .2
	case "extension-261":
		b.mode = "party"
	case "gem-66", "job-3-entry-6", "job-14-entry-4", "job-17-entry-4", "job-76-entry-3":
		b.mode = "sacrifice"
	case "job-7-entry-8":
		b.mode = "death-sacrifice"
	case "job-50-entry-6":
		b.mode = "last"
	case "job-75-entry-5":
		b.mode = "mp"
	case "job-66-revive":
		b.mode = "helper"
	default:
		return b, false
	}
	return b, true
}
