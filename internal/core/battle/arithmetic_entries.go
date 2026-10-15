package battle

// ArithmeticEntry identifies a concrete callback, never a client-supplied
// effect payload. Entries describe this owner's subset, not a full job table.
type ArithmeticEntry struct {
	ID, Name           string
	RequiredSP, MPCost int
	Extended           bool
}

// ArithmeticEntries retains source order and generic native command names.
// Eligibility, ordinary costs and complete random pools belong to the caller.
func ArithmeticEntries(jobID string) []ArithmeticEntry {
	switch jobID {
	case "job-31":
		return []ArithmeticEntry{
			{ID: "job-31-gravity", Name: "ＭＰ４圧縮", RequiredSP: 94, MPCost: 24},
			{ID: "job-31-death", Name: "ＭＰ５終止", RequiredSP: 155, MPCost: 25},
			{ID: "job-31-reverse", Name: "体力桁反転", RequiredSP: 155, MPCost: 35, Extended: true},
		}
	case "job-72":
		return []ArithmeticEntry{
			{ID: "job-72-divisor-5", Name: "ＭＰ５", MPCost: 15},
			{ID: "job-72-divisor-4", Name: "ＭＰ４", RequiredSP: 100, MPCost: 14},
			{ID: "job-72-divisor-3", Name: "ＭＰ３", RequiredSP: 150, MPCost: 13},
			{ID: "job-72-damage", Name: "×魔力撃", RequiredSP: 20},
			{ID: "job-72-reflect", Name: "×反射", RequiredSP: 40},
			{ID: "job-72-heal", Name: "×回復", RequiredSP: 70},
			{ID: "job-72-revive", Name: "×蘇生", RequiredSP: 120},
			{ID: "job-72-death", Name: "×終止", RequiredSP: 180},
		}
	}
	return nil
}
