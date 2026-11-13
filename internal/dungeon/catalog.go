package dungeon

// DefaultDungeonCatalog lists the native maps; the synthetic catalog was retired (#1958).
func DefaultDungeonCatalog() []Dungeon {
	return []Dungeon{nativeMap0()}
}
