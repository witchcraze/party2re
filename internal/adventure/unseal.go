package adventure

import "strings"

// IsDemonKingUnsealStage returns true if the stage corresponds to the authentic Stage EX / 封印の地
// where defeating the stage boss unseals the demon kings (vs_monster.cgi:105, 218).
func IsDemonKingUnsealStage(stageID, stageName string) bool {
	id := strings.ToLower(strings.TrimSpace(stageID))
	name := strings.TrimSpace(stageName)
	return id == "stage-20" || id == "20" || id == "stage-19" || id == "19" || name == "封印の地"
}
