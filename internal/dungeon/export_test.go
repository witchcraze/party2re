package dungeon

import corebattle "github.com/witchcraze/party2re/internal/core/battle"

// NativeMap6 exposes the unlisted map6 definition to tests until #2417 lists it.
var NativeMap6 = nativeMap6

const NativeMap6ID = nativeMap6ID

func (m DungeonMonster) EnemyCombatForTest() *corebattle.LifetimeActor { return m.combat }

// NativeMap15 exposes the unlisted map15 definition to tests until #2443 lists it.
var NativeMap15 = nativeMap15
