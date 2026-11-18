package adventure

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"strings"
)

// Source-name keys preserve exact equality without publishing original skill
// names. IDs follow the ordered tables in _skill.cgi; the name-based cost and
// second-submission lists come from _battle.cgi:761–785,1804–1825.
//
//go:embed normal_operation_names.json
var normalOperationNamesJSON []byte

var normalOperationNames = func() (data struct {
	Names map[string]string
	Gear  map[string]struct {
		ID string
		MP int
	}
	Magician, Second, Bow []string
	Call                  string
}) {
	if err := json.Unmarshal(normalOperationNamesJSON, &data); err != nil {
		panic(err)
	}
	return data
}()

func operationNameKey(name string) string {
	prefix := ""
	if child, generated := strings.CutPrefix(name, "カラクリウェポン_"); generated {
		prefix, name = "generated:", child
	}
	digest := sha256.Sum256([]byte(name))
	return prefix + hex.EncodeToString(digest[:8])
}
