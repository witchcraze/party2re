package adventure

// Generated job names can collide with ordinary NPC source names. These native
// aliases certify equality only, retaining no original names or user input.
// Evidence: ff97fc5:_data.cgi @jobs; stage15:47–73; ordinary NPC source positions.
func generatedEncounterIdentity(job string) string {
	switch job {
	case "job-06":
		return "ordinary-05-boss-28"
	case "job-35":
		return "ordinary-12-boss-29"
	case "job-38":
		return "ordinary-12-monster-162"
	case "job-39":
		return "ordinary-00-monster-38"
	case "job-40":
		return "ordinary-09-monster-297"
	case "job-41":
		return "ordinary-07-monster-165"
	case "job-56":
		return "ordinary-05-monster-120"
	case "job-68":
		return "ordinary-08-monster-38"
	case "job-69":
		return "ordinary-11-monster-58"
	case "job-79":
		return "ordinary-00-monster-65"
	}
	return "generated-job:" + job
}
