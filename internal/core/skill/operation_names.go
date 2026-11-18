package skill

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strings"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

// OperationGearEntry describes an equipped-item combat command and its listed MP.
type OperationGearEntry struct {
	ID string `json:"id"`
	MP int    `json:"mp"`
}

// Source-name keys preserve exact equality without publishing original skill
// names. IDs follow the ordered tables in _skill.cgi; the name-based cost and
// second-submission lists come from _battle.cgi:761–785,1804–1825.
//
//go:embed operation_names.json
var operationNamesJSON []byte

var operationNames = func() (data struct {
	Names                 map[string]string
	Gear                  map[string]OperationGearEntry
	Magician, Second, Bow []string
	Call                  string
}) {
	if err := json.Unmarshal(operationNamesJSON, &data); err != nil {
		panic(err)
	}
	return data
}()

// OperationNameKey hashes a legacy command name into its source-equality key.
func OperationNameKey(name string) string {
	prefix := ""
	if child, generated := strings.CutPrefix(name, "カラクリウェポン_"); generated {
		prefix, name = "generated:", child
	}
	digest := sha256.Sum256([]byte(name))
	return prefix + hex.EncodeToString(digest[:8])
}

// EntrySourceKey returns the source-equality key for a native entry ID.
func EntrySourceKey(id string, burnStrike bool) string {
	if burnStrike && id == "job-84-entry-2" {
		id += "-burn"
	}
	return operationNames.Names[id]
}

// BindingSourceKey returns the source-equality key for a constructed callback binding.
func BindingSourceKey(binding corebattle.CallbackBinding, burnStrike bool) string {
	if child := binding.Invocation.GeneratedChildID; child != "" {
		if key := EntrySourceKey(child, burnStrike); key != "" {
			return "generated:" + key
		}
		return ""
	}
	return EntrySourceKey(binding.ID, burnStrike)
}

// OperationGear returns the equipped-item callback metadata for an item ID.
func OperationGear(itemID string) (OperationGearEntry, bool) {
	gear, ok := operationNames.Gear[itemID]
	return gear, ok
}

// IsMagicianOperationKey reports whether the source key halves MP under item249.
func IsMagicianOperationKey(key string) bool {
	return slices.Contains(operationNames.Magician, key)
}

// IsSecondOperationKey reports whether the source key is whitelisted for additional submission.
func IsSecondOperationKey(key string) bool {
	return slices.Contains(operationNames.Second, key)
}

// IsBowOperationKey reports whether the source key triggers bow bonus handling.
func IsBowOperationKey(key string) bool {
	return slices.Contains(operationNames.Bow, key) ||
		strings.HasPrefix(key, "generated:") && slices.Contains(operationNames.Bow, strings.TrimPrefix(key, "generated:"))
}

// IsCallOperationKey reports whether the source key is the calling summon command.
func IsCallOperationKey(key string) bool {
	return key == operationNames.Call
}
