package job

// legacyItemRequirements contains the catalog IDs used by special jobs. The
// IDs are stable content identifiers; callers still verify ownership through
// the inventory domain before consuming them.
var legacyItemRequirements = map[string]string{
	"job-33": "item-027", "job-34": "item-028", "job-35": "item-029",
	"job-36": "item-032", "job-37": "item-030", "job-38": "item-031",
	"job-39": "item-033", "job-40": "item-034", "job-41": "item-035",
	"job-42": "item-036", "job-44": "item-037", "job-45": "item-038",
	"job-46": "item-039", "job-47": "item-040", "job-54": "item-088",
	"job-55": "item-075", "job-56": "item-085", "job-57": "item-013",
	"job-58": "item-089", "job-59": "item-090", "job-60": "item-091",
	"job-61": "item-092", "job-62": "item-093", "job-63": "item-094",
	"job-64": "item-095", "job-65": "item-096", "job-66": "item-097",
	"job-67": "item-098", "job-68": "item-099", "job-69": "item-100",
	"job-71": "item-108", "job-72": "item-109", "job-80": "item-142",
	"job-81": "item-068", "job-87": "item-217",
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
