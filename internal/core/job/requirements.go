package job

// legacyItemRequirements contains the catalog IDs used by special jobs. The
// IDs are stable content identifiers; callers still verify ownership through
// the inventory domain before consuming them.
var legacyItemRequirements = map[string]string{
	"job-33": "item-027", "job-34": "item-028", "job-35": "item-029",
	"job-36": "item-032", "job-37": "item-030", "job-38": "item-031",
	"job-39": "item-033", "job-40": "item-034", "job-41": "item-035",
	"job-42": "item-036", "job-44": "item-037", "job-45": "item-038",
	"job-46": "item-039", "job-47": "item-040", "job-48": "item-040",
	"job-54": "item-088", "job-55": "item-075", "job-56": "item-085",
	"job-57": "item-013", "job-58": "item-089", "job-59": "item-090",
	"job-60": "item-091", "job-61": "item-092", "job-62": "item-093",
	"job-63": "item-094", "job-64": "item-095", "job-65": "item-096",
	"job-66": "item-097", "job-67": "item-098", "job-68": "item-099",
	"job-69": "item-100", "job-71": "item-108", "job-72": "item-109",
	"job-80": "item-142", "job-81": "item-068", "job-87": "item-217",
	"job-84": "armor-29",
}

var legacyMasteryThresholds = map[string]int{
	"job-01": 80, "job-02": 100, "job-03": 80, "job-04": 100,
	"job-05": 100, "job-06": 90, "job-07": 80, "job-08": 50,
	"job-09": 90, "job-10": 100, "job-11": 110, "job-12": 110,
	"job-13": 90, "job-14": 100, "job-15": 130, "job-16": 140,
	"job-17": 160, "job-18": 120, "job-19": 110, "job-20": 100,
	"job-21": 110, "job-22": 140, "job-23": 130, "job-24": 130,
	"job-25": 130, "job-26": 110, "job-27": 110, "job-28": 140,
	"job-29": 150, "job-30": 150, "job-31": 155, "job-32": 150,
	"job-33": 160, "job-34": 180, "job-35": 180, "job-36": 100,
	"job-37": 100, "job-38": 130, "job-39": 99, "job-40": 99,
	"job-41": 120, "job-42": 150, "job-43": 150, "job-44": 120,
	"job-45": 150, "job-46": 140, "job-47": 160, "job-48": 200,
	"job-49": 300, "job-50": 145, "job-51": 160, "job-52": 150,
	"job-53": 120, "job-54": 130, "job-55": 80, "job-56": 96,
	"job-57": 120, "job-58": 160, "job-59": 150, "job-60": 150,
	"job-61": 150, "job-62": 150, "job-63": 150, "job-64": 150,
	"job-65": 150, "job-66": 150, "job-67": 150, "job-68": 150,
	"job-69": 150, "job-70": 200, "job-71": 150, "job-72": 180,
	"job-73": 0, "job-74": 150, "job-75": 140, "job-76": 130,
	"job-77": 130, "job-78": 200, "job-79": 99, "job-80": 150,
	"job-81": 130, "job-82": 111, "job-83": 100, "job-84": 140,
	"job-85": 150, "job-86": 150, "job-87": 150,
}

// RequiredItem returns the item consumed by a job change, if any.
func (d Definition) RequiredItem() string {
	if d.RequiredItemID != "" {
		return d.RequiredItemID
	}
	return legacyItemRequirements[d.ID]
}

// MasteryThreshold returns the final skill's SP requirement for this job.
func (d Definition) MasteryThreshold() int {
	if d.MasterySP > 0 {
		return d.MasterySP
	}
	return legacyMasteryThresholds[d.ID]
}

// ChangeContext holds character state, milestones, inventory, and equipment
// necessary to evaluate legacy job requirements (_data.cgi @jobs conditions).
type ChangeContext struct {
	Level            int
	Gender           string
	CurrentJobID     string
	OldJobID         string
	SP               int
	JobLevel         int
	PvPWins          int
	HeroCount        int
	CasinoWins       int
	MonsterKills     int
	MaoCount         int
	AllJobsMastered  bool
	HasRequiredItem  bool
	HasEquippedArmor bool
}

// isNeedJob reports whether CurrentJobID or OldJobID matches any of the specified job IDs.
// Matches legacy Party2 _data.cgi:303-309 sub _is_need_job.
func isNeedJob(ctx ChangeContext, jobIDs []string) bool {
	for _, id := range jobIDs {
		if ctx.CurrentJobID == id || ctx.OldJobID == id {
			return true
		}
	}
	return false
}

// ValidateRequirements checks whether a character satisfies all legacy conditions
// for changing to the target job (_data.cgi lines 14-301).
func ValidateRequirements(target Definition, ctx ChangeContext) error {
	if target.ID == "" || target.ID == "starter" {
		return ErrJobUnavailable
	}
	if ctx.Level < MinimumChangeLevel {
		return ErrJobUnavailable
	}
	if target.RequiredGender != "" && target.RequiredGender != ctx.Gender {
		return ErrJobUnavailable
	}

	switch target.ID {
	case "job-73": // すっぴん: sub { -f "$userdir/$id/comp_job_flag.cgi" }
		if !ctx.AllJobsMastered {
			return ErrJobUnavailable
		}
		return nil

	case "job-70": // 天竜人: sub { &_is_need_job(70) }
		if !isNeedJob(ctx, []string{"job-70"}) {
			return ErrJobUnavailable
		}
		return nil

	case "job-49": // たまねぎ剣士: sub { &_is_need_job(49) || $m{sp} >= 300 }
		if isNeedJob(ctx, []string{"job-49"}) || ctx.SP >= 300 {
			return nil
		}
		return ErrJobUnavailable

	case "job-21": // バーサーカー: sub { $m{kill_m} > 200 && &_is_need_job( 1, 4, 12, 21 ) }
		if ctx.MonsterKills > 200 && isNeedJob(ctx, target.RequiredJobIDs) {
			return nil
		}
		return ErrJobUnavailable

	case "job-22": // 暗黒騎士: sub { $m{kill_p} > 50 && &_is_need_job( 2, 3, 17, 20, 22, 52 ) }
		if ctx.PvPWins > 50 && isNeedJob(ctx, target.RequiredJobIDs) {
			return nil
		}
		return ErrJobUnavailable

	case "job-52": // 魔人: sub { $m{kill_m} > 1000 && &_is_need_job( 1, 21, 25, 52 ) }
		if ctx.MonsterKills > 1000 && isNeedJob(ctx, target.RequiredJobIDs) {
			return nil
		}
		return ErrJobUnavailable

	case "job-74": // 剣闘士: sub { &_is_need_job(74) || ( &_is_need_job( 2, 3, 4, 8 ) && $m{kill_p} > 30 ) }
		if isNeedJob(ctx, []string{"job-74"}) {
			return nil
		}
		if ctx.PvPWins > 30 && isNeedJob(ctx, []string{"job-02", "job-03", "job-04", "job-08"}) {
			return nil
		}
		return ErrJobUnavailable

	case "job-77": // 双剣士: sub { &_is_need_job(77) || ( $m{kill_m} > 1500 && &_is_need_job( 2, 24, 28 ) ) }
		if isNeedJob(ctx, []string{"job-77"}) {
			return nil
		}
		if ctx.MonsterKills > 1500 && isNeedJob(ctx, []string{"job-02", "job-24", "job-28"}) {
			return nil
		}
		return ErrJobUnavailable

	case "job-78": // トレジャーハンター: sub { &_is_need_job(78) || ( $m{job_lv} >= 50 && &_is_need_job( 7, 8, 9, 26, 50 ) ) }
		if isNeedJob(ctx, []string{"job-78"}) {
			return nil
		}
		if ctx.JobLevel >= 50 && isNeedJob(ctx, []string{"job-07", "job-08", "job-09", "job-26", "job-50"}) {
			return nil
		}
		return ErrJobUnavailable

	case "job-81": // 海賊: sub { &_is_need_job(81) || ( &_is_need_job( 1, 9, 21, 25, 43, 74 ) && $m{ite} eq '68' ) }
		if isNeedJob(ctx, []string{"job-81"}) {
			return nil
		}
		if ctx.HasRequiredItem && isNeedJob(ctx, []string{"job-01", "job-09", "job-21", "job-25", "job-43", "job-74"}) {
			return nil
		}
		return ErrJobUnavailable

	case "job-84": // 炎闘士: sub { &_is_need_job(84) || (&_is_need_job(1,4,25,30,) && $m{arm} eq '29') }
		if isNeedJob(ctx, []string{"job-84"}) {
			return nil
		}
		if ctx.HasEquippedArmor && isNeedJob(ctx, []string{"job-01", "job-04", "job-25", "job-30"}) {
			return nil
		}
		return ErrJobUnavailable

	case "job-34": // 勇者: sub { &_is_need_job(34) || ( $m{ite} eq '28' && $m{hero_c} >= 5 ) }
		if isNeedJob(ctx, []string{"job-34"}) {
			return nil
		}
		if ctx.HasRequiredItem && ctx.HeroCount >= 5 {
			return nil
		}
		return ErrJobUnavailable

	case "job-35": // 魔王: sub { &_is_need_job(35) || ( $m{ite} eq '29' && $m{mao_c} >= 1 ) }
		if isNeedJob(ctx, []string{"job-35"}) {
			return nil
		}
		if ctx.HasRequiredItem && ctx.MaoCount >= 1 {
			return nil
		}
		return ErrJobUnavailable

	case "job-46": // ギャンブラー: sub { &_is_need_job(46) || ( $m{ite} eq '39' && $m{cas_c} >= 10 ) }
		if isNeedJob(ctx, []string{"job-46"}) {
			return nil
		}
		if ctx.HasRequiredItem && ctx.CasinoWins >= 10 {
			return nil
		}
		return ErrJobUnavailable

	case "job-33": // 賢者: sub { &_is_need_job( 8, 33 ) || $m{ite} eq '27' }
		if isNeedJob(ctx, []string{"job-08", "job-33"}) || ctx.HasRequiredItem {
			return nil
		}
		return ErrJobUnavailable
	}

	// General item-gated job (sub { &_is_need_job(X) || $m{ite} eq '...' })
	if target.RequiredItem() != "" {
		if isNeedJob(ctx, []string{target.ID}) || ctx.HasRequiredItem {
			return nil
		}
		return ErrJobUnavailable
	}

	// General prerequisite tree jobs (sub { &_is_need_job(...) })
	if len(target.RequiredJobIDs) > 0 {
		if !isNeedJob(ctx, target.RequiredJobIDs) {
			return ErrJobUnavailable
		}
		return nil
	}

	// Base jobs with no requirements (sub { 1 })
	return nil
}

// IsItemConsumed returns whether the job change consumes an item from inventory.
// Matches legacy Party2 job_change.cgi:164-179:
// Special jobs requiring items consume them UNLESS:
// 1. Current or old job is the target job (_is_need_job($i)), OR
// 2. Target is 賢者 or ギャンブラー AND current or old job is 遊び人 (job-08).
func IsItemConsumed(targetID, currentJobID, oldJobID string) bool {
	reqItem := (Definition{ID: targetID}).RequiredItem()
	if reqItem == "" || targetID == "job-84" {
		// job-84 uses armor, not standard item
		return false
	}
	if currentJobID == targetID || oldJobID == targetID {
		return false
	}
	if (targetID == "job-33" || targetID == "job-46") && (currentJobID == "job-08" || oldJobID == "job-08") {
		return false
	}
	return true
}

// IsItemExempt reports whether item consumption is exempted for the given job change.
// This is the inverse of the non-item-existence part of IsItemConsumed and is used by the
// application layer when the item's definition comes from the catalog (not legacyItemRequirements).
// Matches legacy job_change.cgi:164-179 exemption conditions.
func IsItemExempt(targetID, currentJobID, oldJobID string) bool {
	if currentJobID == targetID || oldJobID == targetID {
		return true
	}
	if (targetID == "job-33" || targetID == "job-46") && (currentJobID == "job-08" || oldJobID == "job-08") {
		return true
	}
	return false
}

// IsArmorConsumed returns whether changing to 炎闘士 (job-84) consumes equipped armor-29.
// Matches legacy Party2 job_change.cgi:181-188:
// Unless already 炎闘士 (_is_need_job(84)), equipped armor 29 is consumed.
func IsArmorConsumed(targetID, currentJobID, oldJobID string) bool {
	if targetID != "job-84" {
		return false
	}
	if currentJobID == "job-84" || oldJobID == "job-84" {
		return false
	}
	return true
}
