package character

import (
	"fmt"
	"strconv"
	"strings"
)

// ValidateNeedJoin checks a room's max HP or job level requirement.
// Empty, zero and unrecognized conditions impose no restriction.
func ValidateNeedJoin(condition string, char Character) error {
	if condition == "" || condition == "0" {
		return nil
	}
	parts := strings.SplitN(condition, "_", 4)
	if len(parts) != 3 {
		return nil
	}
	threshold, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil
	}

	var label string
	var value int
	switch parts[0] {
	case "hp":
		label, value = "HP", char.Stats.MaxHP
	case "joblv":
		label, value = "Job level", char.JobLevel
	default:
		return nil
	}
	if parts[2] == "u" && value >= threshold {
		return fmt.Errorf("%s must be under %d", label, threshold)
	}
	if parts[2] == "o" && value < threshold {
		return fmt.Errorf("%s must be at least %d", label, threshold)
	}
	return nil
}
