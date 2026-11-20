package dungeon

// DefaultDungeonCatalog lists the native maps; the synthetic catalog was retired (#1958).
func DefaultDungeonCatalog() []Dungeon {
	return []Dungeon{nativeMap0(), nativeMap1(), nativeMap2(), nativeMap3(), nativeMap4(), nativeMap5(), nativeMap7(), nativeMap8(), nativeMap9(), nativeMap10(), nativeMap11(), nativeMap12(), nativeMap14(), nativeMap15(), nativeMap17()}
}
