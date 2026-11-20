package dungeon

import corebattle "github.com/witchcraze/party2re/internal/core/battle"

// NativeMap6 exposes the unlisted map6 definition to tests until #2417 lists it.
var NativeMap6 = nativeMap6

const NativeMap6ID = nativeMap6ID

func (m DungeonMonster) EnemyCombatForTest() *corebattle.LifetimeActor { return m.combat }

// NativeMap17 exposes the unlisted map17 definition to tests until #2454 lists it.
var NativeMap17 = nativeMap17
