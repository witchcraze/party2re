package boss

import _ "embed"

// bossDamageLua executes atomic damage application, overkill prevention, contributor tallying,
// and deterministic killer election for shared World Boss encounters.
//
// Keys:
//
//	KEYS[1]: party2:boss:{boss:<id>}:hp
//	KEYS[2]: party2:boss:{boss:<id>}:status
//	KEYS[3]: party2:boss:{boss:<id>}:contributors
//	KEYS[4]: party2:boss:{boss:<id>}:killer
//	KEYS[5]: party2:boss:{boss:<id>}:run_id
//
// Arguments:
//
//	ARGV[1]: attacker_id (string)
//	ARGV[2]: incoming_damage (integer)
//	ARGV[3]: ttl_seconds (integer)
//
//go:embed lua/boss_damage.lua
var bossDamageLua string
