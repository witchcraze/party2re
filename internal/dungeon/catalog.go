package dungeon

func DefaultDungeonCatalog() []Dungeon {
	return []Dungeon{
		{
			ID:               "dungeon-01",
			Tier:             1,
			Name:             "ゴブリンの迷宮",
			Description:      "ゴブリンたちが潜む初心者冒険者向けの地下迷宮。",
			MinLevel:         5,
			MaxTurnsPerFloor: 25,
			ClearExpBonus:    300,
			ClearGoldBonus:   500,
			Floors: []Floor{
				{
					FloorNumber: 1,
					Width:       4,
					Height:      4,
					StartX:      0,
					StartY:      0,
					Grid: []string{
						"S00T",
						"1101",
						"0X0D",
						"1000",
					},
					Monsters: []DungeonMonster{
						{ID: "d1-m1", Name: "ゴブリン斥候", HP: 40, Attack: 15, Defense: 10, Agility: 12, ExpReward: 30, GoldReward: 40, DropItemID: "potion"},
						{ID: "d1-m2", Name: "ゴブリン戦士", HP: 60, Attack: 22, Defense: 14, Agility: 10, ExpReward: 50, GoldReward: 70, DropItemID: "potion"},
					},
				},
				{
					FloorNumber: 2,
					Width:       4,
					Height:      4,
					StartX:      0,
					StartY:      0,
					Grid: []string{
						"S0X0",
						"1010",
						"00T0",
						"101B",
					},
					Monsters: []DungeonMonster{
						{ID: "d1-m2", Name: "ゴブリン戦士", HP: 60, Attack: 22, Defense: 14, Agility: 10, ExpReward: 50, GoldReward: 70, DropItemID: "potion"},
					},
					Boss: &DungeonMonster{
						ID: "d1-boss", Name: "ゴブリンロード", HP: 150, Attack: 35, Defense: 25, Agility: 18, ExpReward: 200, GoldReward: 300, DropItemID: "high-potion",
					},
				},
			},
		},
		{
			ID:               "dungeon-02",
			Tier:             2,
			Name:             "忘れられた地下墓地",
			Description:      "アンデッドと罠が徘徊する暗黒のカタコンベ。",
			MinLevel:         20,
			MaxTurnsPerFloor: 30,
			ClearExpBonus:    800,
			ClearGoldBonus:   1500,
			Floors: []Floor{
				{
					FloorNumber: 1,
					Width:       4,
					Height:      4,
					StartX:      0,
					StartY:      0,
					Grid: []string{
						"S001",
						"010T",
						"0X0D",
						"1000",
					},
					Monsters: []DungeonMonster{
						{ID: "d2-m1", Name: "スケルトン兵", HP: 100, Attack: 45, Defense: 30, Agility: 25, ExpReward: 120, GoldReward: 160, DropItemID: "high-potion"},
					},
				},
				{
					FloorNumber: 2,
					Width:       4,
					Height:      4,
					StartX:      0,
					StartY:      0,
					Grid: []string{
						"S0X0",
						"1010",
						"T000",
						"110D",
					},
					Monsters: []DungeonMonster{
						{ID: "d2-m2", Name: "レイス", HP: 140, Attack: 65, Defense: 40, Agility: 40, ExpReward: 200, GoldReward: 250, DropItemID: "ether"},
					},
				},
				{
					FloorNumber: 3,
					Width:       4,
					Height:      4,
					StartX:      0,
					StartY:      0,
					Grid: []string{
						"S000",
						"0110",
						"0X0T",
						"100B",
					},
					Monsters: []DungeonMonster{
						{ID: "d2-m2", Name: "レイス", HP: 140, Attack: 65, Defense: 40, Agility: 40, ExpReward: 200, GoldReward: 250, DropItemID: "ether"},
					},
					Boss: &DungeonMonster{
						ID: "d2-boss", Name: "リッチキング", HP: 450, Attack: 110, Defense: 80, Agility: 50, ExpReward: 600, GoldReward: 1000, DropItemID: "high-ether",
					},
				},
			},
		},
		{
			ID:               "dungeon-03",
			Tier:             3,
			Name:             "灼熱の溶岩洞窟",
			Description:      "猛火のマグマとドラゴン眷属が支配する地下火口洞。",
			MinLevel:         40,
			MaxTurnsPerFloor: 35,
			ClearExpBonus:    2000,
			ClearGoldBonus:   4000,
			Floors: []Floor{
				{
					FloorNumber: 1,
					Width:       4,
					Height:      4,
					StartX:      0,
					StartY:      0,
					Grid: []string{
						"S0X1",
						"010T",
						"0000",
						"10XD",
					},
					Monsters: []DungeonMonster{
						{ID: "d3-m1", Name: "サラマンダー", HP: 280, Attack: 130, Defense: 90, Agility: 60, ExpReward: 350, GoldReward: 450, DropItemID: "elixir"},
					},
				},
				{
					FloorNumber: 2,
					Width:       4,
					Height:      4,
					StartX:      0,
					StartY:      0,
					Grid: []string{
						"S000",
						"1010",
						"T0X0",
						"100D",
					},
					Monsters: []DungeonMonster{
						{ID: "d3-m2", Name: "ファイアドレイク", HP: 360, Attack: 160, Defense: 110, Agility: 75, ExpReward: 500, GoldReward: 700, DropItemID: "crystal-01"},
					},
				},
				{
					FloorNumber: 3,
					Width:       4,
					Height:      4,
					StartX:      0,
					StartY:      0,
					Grid: []string{
						"S001",
						"0100",
						"0X0T",
						"100B",
					},
					Monsters: []DungeonMonster{
						{ID: "d3-m2", Name: "ファイアドレイク", HP: 360, Attack: 160, Defense: 110, Agility: 75, ExpReward: 500, GoldReward: 700, DropItemID: "crystal-01"},
					},
					Boss: &DungeonMonster{
						ID: "d3-boss", Name: "真紅の火炎竜", HP: 1200, Attack: 240, Defense: 170, Agility: 90, ExpReward: 1500, GoldReward: 3000, DropItemID: "crystal-02",
					},
				},
			},
		},
	}
}
