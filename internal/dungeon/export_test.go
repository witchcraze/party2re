package dungeon

import corebattle "github.com/witchcraze/party2re/internal/core/battle"

// NativeMap6 exposes the unlisted map6 definition to tests until #2417 lists it.
var NativeMap6 = nativeMap6

const NativeMap6ID = nativeMap6ID

// NativeMap13 exposes the unlisted map13 definition to tests until its destination units list it.
var NativeMap13 = nativeMap13

const NativeMap13ID = nativeMap13ID

var Map13Route = map13Route

func (m DungeonMonster) EnemyCombatForTest() *corebattle.LifetimeActor { return m.combat }

// NativeMap16 exposes the unlisted map16 definition to tests until #2943 lists it.
var NativeMap16 = nativeMap16

const NativeMap16ID = nativeMap16ID
