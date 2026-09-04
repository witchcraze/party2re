package dungeon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/progression"
	"github.com/witchcraze/party2re/internal/id"
)

var (
	ErrDungeonNotFound           = errors.New("dungeon not found")
	ErrCharacterNotFound         = errors.New("character not found")
	ErrLevelRequirementNotMet    = errors.New("character level requirement not met for dungeon")
	ErrActiveExpeditionExists    = errors.New("an active expedition is already in progress")
	ErrNoActiveExpedition        = errors.New("no active dungeon expedition in progress")
	ErrInvalidDirection          = errors.New("invalid move direction")
	ErrImpassableWall            = errors.New("cannot move into wall or out of map bounds")
	ErrExpeditionAlreadyFinished = errors.New("expedition is already completed")
)

type ExpeditionStatus string

const (
	StatusExploring ExpeditionStatus = "exploring"
	StatusCleared   ExpeditionStatus = "cleared"
	StatusEscaped   ExpeditionStatus = "escaped"
	StatusWipedOut  ExpeditionStatus = "wiped_out"
)

type TileEventType string

const (
	EventMove     TileEventType = "move"
	EventBattle   TileEventType = "battle"
	EventTreasure TileEventType = "treasure"
	EventTrap     TileEventType = "trap"
	EventStairs   TileEventType = "stairs"
	EventBoss     TileEventType = "boss"
	EventEscape   TileEventType = "escape"
	EventWipeout  TileEventType = "wipeout"
)

type Direction string

const (
	DirectionNorth Direction = "north"
	DirectionSouth Direction = "south"
	DirectionEast  Direction = "east"
	DirectionWest  Direction = "west"
)

type DungeonMonster struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	HP         int    `json:"hp"`
	Attack     int    `json:"attack"`
	Defense    int    `json:"defense"`
	Agility    int    `json:"agility"`
	ExpReward  int    `json:"exp_reward"`
	GoldReward int    `json:"gold_reward"`
	DropItemID string `json:"drop_item_id,omitempty"`
}

type Floor struct {
	FloorNumber int              `json:"floor_number"`
	Width       int              `json:"width"`
	Height      int              `json:"height"`
	StartX      int              `json:"start_x"`
	StartY      int              `json:"start_y"`
	Grid        []string         `json:"grid"`
	Monsters    []DungeonMonster `json:"monsters"`
	Boss        *DungeonMonster  `json:"boss,omitempty"`
}

type Dungeon struct {
	ID               string  `json:"id"`
	Tier             int     `json:"tier"`
	Name             string  `json:"name"`
	Description      string  `json:"description"`
	MinLevel         int     `json:"min_level"`
	MaxTurnsPerFloor int     `json:"max_turns_per_floor"`
	Floors           []Floor `json:"floors"`
	ClearExpBonus    int     `json:"clear_exp_bonus"`
	ClearGoldBonus   int     `json:"clear_gold_bonus"`
}

type ActiveExpedition struct {
	ID                string           `json:"id"`
	CharacterID       string           `json:"character_id"`
	DungeonID         string           `json:"dungeon_id"`
	CurrentFloor      int              `json:"current_floor"`
	PosX              int              `json:"pos_x"`
	PosY              int              `json:"pos_y"`
	CurrentHP         int              `json:"current_hp"`
	TurnsRemaining    int              `json:"turns_remaining"`
	AccumulatedExp    int              `json:"accumulated_exp"`
	AccumulatedGold   int              `json:"accumulated_gold"`
	AccumulatedItems  []string         `json:"accumulated_items"`
	AccumulatedMedals int              `json:"accumulated_medals"`
	Status            ExpeditionStatus `json:"status"`
	StartedAt         time.Time        `json:"started_at"`
	UpdatedAt         time.Time        `json:"updated_at"`
}

type CharacterDungeonRecord struct {
	CharacterID           string    `json:"character_id"`
	HighestDungeonCleared int       `json:"highest_dungeon_cleared"`
	TotalExpeditions      int       `json:"total_expeditions"`
	TotalFloorsCleared    int       `json:"total_floors_cleared"`
	TotalChestsOpened     int       `json:"total_chests_opened"`
	TotalMonstersSlain    int       `json:"total_monsters_slain"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

type DungeonExpeditionHistory struct {
	ID               string           `json:"id"`
	CharacterID      string           `json:"character_id"`
	DungeonID        string           `json:"dungeon_id"`
	FloorsReached    int              `json:"floors_reached"`
	Outcome          ExpeditionStatus `json:"outcome"`
	ExpReward        int              `json:"exp_reward"`
	GoldReward       int              `json:"gold_reward"`
	MedalsReward     int              `json:"medals_reward"`
	ItemsRewardCount int              `json:"items_reward_count"`
	CreatedAt        time.Time        `json:"created_at"`
}

type DungeonOverview struct {
	Dungeon    Dungeon `json:"dungeon"`
	IsUnlocked bool    `json:"is_unlocked"`
	IsCleared  bool    `json:"is_cleared"`
	LockReason string  `json:"lock_reason,omitempty"`
}

type ExpeditionStepResult struct {
	Expedition   ActiveExpedition   `json:"expedition"`
	EventType    TileEventType      `json:"event_type"`
	Message      string             `json:"message"`
	BattleResult *corebattle.Result `json:"battle_result,omitempty"`
	DamageTaken  int                `json:"damage_taken,omitempty"`
	GoldFound    int                `json:"gold_found,omitempty"`
	MedalsFound  int                `json:"medals_found,omitempty"`
	ItemFound    string             `json:"item_found,omitempty"`
	ExpEarned    int                `json:"exp_earned,omitempty"`
	IsFinished   bool               `json:"is_finished"`
}

type Repository interface {
	GetRecord(ctx context.Context, characterID string) (CharacterDungeonRecord, error)
	GetActiveExpedition(ctx context.Context, characterID string) (*ActiveExpedition, error)
	SaveActiveExpedition(ctx context.Context, exp ActiveExpedition) error
	DeleteActiveExpedition(ctx context.Context, characterID string) error
	FinalizeExpedition(
		ctx context.Context,
		history DungeonExpeditionHistory,
		record CharacterDungeonRecord,
		character *corecharacter.Character,
		rewardItems []coreitem.Instance,
	) error
	GetHistory(ctx context.Context, characterID string, limit int) ([]DungeonExpeditionHistory, error)
}

type CharacterRepository interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
}

// MonsterDefeatedHook is called when a monster or boss is slain during dungeon exploration.
type MonsterDefeatedHook func(ctx context.Context, characterID string, count int) error

type Service struct {
	repo                Repository
	characterRepo       CharacterRepository
	battleEngine        corebattle.Resolver
	dungeons            []Dungeon
	dungeonMap          map[string]Dungeon
	monsterDefeatedHook MonsterDefeatedHook
}

func (s *Service) SetMonsterDefeatedHook(hook MonsterDefeatedHook) {
	s.monsterDefeatedHook = hook
}

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

func NewService(
	repo Repository,
	characterRepo CharacterRepository,
	battleEngine corebattle.Resolver,
	customDungeons ...Dungeon,
) (*Service, error) {
	if repo == nil {
		return nil, errors.New("dungeon repository is required")
	}
	if characterRepo == nil {
		return nil, errors.New("character repository is required")
	}
	if battleEngine == nil {
		return nil, errors.New("battle engine is required")
	}

	catalog := DefaultDungeonCatalog()
	if len(customDungeons) > 0 {
		catalog = customDungeons
	}

	dMap := make(map[string]Dungeon, len(catalog))
	for _, d := range catalog {
		dMap[d.ID] = d
	}

	return &Service{
		repo:          repo,
		characterRepo: characterRepo,
		battleEngine:  battleEngine,
		dungeons:      catalog,
		dungeonMap:    dMap,
	}, nil
}

func (s *Service) ListDungeons(ctx context.Context, characterID string) ([]DungeonOverview, error) {
	if characterID == "" {
		return nil, ErrCharacterNotFound
	}
	char, err := s.characterRepo.FindByID(ctx, characterID)
	if err != nil {
		return nil, ErrCharacterNotFound
	}
	rec, err := s.repo.GetRecord(ctx, characterID)
	if err != nil {
		return nil, err
	}

	overviews := make([]DungeonOverview, 0, len(s.dungeons))
	for _, d := range s.dungeons {
		isUnlocked := true
		lockReason := ""

		if char.Level < d.MinLevel {
			isUnlocked = false
			lockReason = fmt.Sprintf("Requires Character Level %d", d.MinLevel)
		}
		if isUnlocked && d.Tier > 1 {
			prereqTier := d.Tier - 1
			if rec.HighestDungeonCleared < prereqTier {
				isUnlocked = false
				lockReason = fmt.Sprintf("Requires clearing Tier %d Dungeon first", prereqTier)
			}
		}

		isCleared := rec.HighestDungeonCleared >= d.Tier
		overviews = append(overviews, DungeonOverview{
			Dungeon:    d,
			IsUnlocked: isUnlocked,
			IsCleared:  isCleared,
			LockReason: lockReason,
		})
	}

	return overviews, nil
}

func (s *Service) GetActiveExpedition(ctx context.Context, characterID string) (*ActiveExpedition, error) {
	if characterID == "" {
		return nil, ErrCharacterNotFound
	}
	return s.repo.GetActiveExpedition(ctx, characterID)
}

func (s *Service) StartExpedition(ctx context.Context, characterID, dungeonID string) (*ActiveExpedition, error) {
	if characterID == "" {
		return nil, ErrCharacterNotFound
	}
	dungeon, ok := s.dungeonMap[dungeonID]
	if !ok {
		return nil, ErrDungeonNotFound
	}

	char, err := s.characterRepo.FindByID(ctx, characterID)
	if err != nil {
		return nil, ErrCharacterNotFound
	}

	if char.Level < dungeon.MinLevel {
		return nil, ErrLevelRequirementNotMet
	}

	existing, err := s.repo.GetActiveExpedition(ctx, characterID)
	if err != nil {
		return nil, err
	}
	if existing != nil && existing.Status == StatusExploring {
		return nil, ErrActiveExpeditionExists
	}

	if len(dungeon.Floors) == 0 {
		return nil, errors.New("dungeon has no floors")
	}

	firstFloor := dungeon.Floors[0]
	expID := id.New()

	now := time.Now().UTC()
	exp := ActiveExpedition{
		ID:               expID,
		CharacterID:      char.ID,
		DungeonID:        dungeon.ID,
		CurrentFloor:     1,
		PosX:             firstFloor.StartX,
		PosY:             firstFloor.StartY,
		CurrentHP:        char.Stats.HP,
		TurnsRemaining:   dungeon.MaxTurnsPerFloor,
		AccumulatedExp:   0,
		AccumulatedGold:  0,
		AccumulatedItems: []string{},
		Status:           StatusExploring,
		StartedAt:        now,
		UpdatedAt:        now,
	}

	if err := s.repo.SaveActiveExpedition(ctx, exp); err != nil {
		return nil, err
	}

	return &exp, nil
}

func (s *Service) Move(ctx context.Context, characterID string, dir Direction) (ExpeditionStepResult, error) {
	if characterID == "" {
		return ExpeditionStepResult{}, ErrCharacterNotFound
	}

	exp, err := s.repo.GetActiveExpedition(ctx, characterID)
	if err != nil {
		return ExpeditionStepResult{}, err
	}
	if exp == nil || exp.Status != StatusExploring {
		return ExpeditionStepResult{}, ErrNoActiveExpedition
	}

	dungeon, ok := s.dungeonMap[exp.DungeonID]
	if !ok {
		return ExpeditionStepResult{}, ErrDungeonNotFound
	}

	char, err := s.characterRepo.FindByID(ctx, characterID)
	if err != nil {
		return ExpeditionStepResult{}, ErrCharacterNotFound
	}

	floorIdx := exp.CurrentFloor - 1
	if floorIdx < 0 || floorIdx >= len(dungeon.Floors) {
		return ExpeditionStepResult{}, errors.New("invalid floor index")
	}
	floor := dungeon.Floors[floorIdx]

	newX, newY := exp.PosX, exp.PosY
	switch dir {
	case DirectionNorth:
		newY--
	case DirectionSouth:
		newY++
	case DirectionEast:
		newX++
	case DirectionWest:
		newX--
	default:
		return ExpeditionStepResult{}, ErrInvalidDirection
	}

	if newX < 0 || newX >= floor.Width || newY < 0 || newY >= floor.Height {
		return ExpeditionStepResult{}, ErrImpassableWall
	}

	tileChar := floor.Grid[newY][newX]
	if tileChar == '1' {
		return ExpeditionStepResult{}, ErrImpassableWall
	}

	exp.PosX = newX
	exp.PosY = newY
	exp.TurnsRemaining--
	exp.UpdatedAt = time.Now().UTC()

	// Check turn exhaustion -> wipeout/timeout
	if exp.TurnsRemaining <= 0 {
		return s.handleWipeout(ctx, exp, &char, "行動限界（ターン切れ）により意識を失い、探索に失敗した…")
	}

	// Dispatch Tile Event
	switch tileChar {
	case 'S', '0': // Start or Normal Path
		// 50% monster encounter chance on normal path
		if tileChar == '0' && len(floor.Monsters) > 0 {
			monster := floor.Monsters[newX%len(floor.Monsters)]
			return s.resolveMonsterCombat(ctx, exp, &char, monster, EventBattle)
		}
		if err := s.repo.SaveActiveExpedition(ctx, *exp); err != nil {
			return ExpeditionStepResult{}, err
		}
		return ExpeditionStepResult{
			Expedition: *exp,
			EventType:  EventMove,
			Message:    "静かな通路を進んだ。",
		}, nil

	case 'T': // Treasure Chest
		goldFound := 100 * exp.CurrentFloor
		itemFound := "potion"
		if len(floor.Monsters) > 0 && floor.Monsters[0].DropItemID != "" {
			itemFound = floor.Monsters[0].DropItemID
		}
		medalsFound := 1
		exp.AccumulatedGold += goldFound
		exp.AccumulatedMedals += medalsFound
		exp.AccumulatedItems = append(exp.AccumulatedItems, itemFound)

		rec, _ := s.repo.GetRecord(ctx, char.ID)
		rec.TotalChestsOpened++

		if err := s.repo.SaveActiveExpedition(ctx, *exp); err != nil {
			return ExpeditionStepResult{}, err
		}
		return ExpeditionStepResult{
			Expedition:  *exp,
			EventType:   EventTreasure,
			GoldFound:   goldFound,
			MedalsFound: medalsFound,
			ItemFound:   itemFound,
			Message:     fmt.Sprintf("宝箱を発見した！ %d G と %s 、ちいさなメダル %d枚を手に入れた！", goldFound, itemFound, medalsFound),
		}, nil

	case 'X': // Hazard Trap
		trapDamage := int(math.Max(10, float64(char.Stats.MaxHP)*0.15))
		exp.CurrentHP -= trapDamage
		if exp.CurrentHP <= 0 {
			exp.CurrentHP = 0
			return s.handleWipeout(ctx, exp, &char, fmt.Sprintf("罠が作動し %d の猛烈なダメージを受けた！力尽きて倒れた…", trapDamage))
		}
		if err := s.repo.SaveActiveExpedition(ctx, *exp); err != nil {
			return ExpeditionStepResult{}, err
		}
		return ExpeditionStepResult{
			Expedition:  *exp,
			EventType:   EventTrap,
			DamageTaken: trapDamage,
			Message:     fmt.Sprintf("罠を踏んでしまった！ %d のダメージを受けた！", trapDamage),
		}, nil

	case 'D': // Down Stairs
		if exp.CurrentFloor < len(dungeon.Floors) {
			exp.CurrentFloor++
			nextFloor := dungeon.Floors[exp.CurrentFloor-1]
			exp.PosX = nextFloor.StartX
			exp.PosY = nextFloor.StartY
			exp.TurnsRemaining = dungeon.MaxTurnsPerFloor

			rec, _ := s.repo.GetRecord(ctx, char.ID)
			rec.TotalFloorsCleared++

			if err := s.repo.SaveActiveExpedition(ctx, *exp); err != nil {
				return ExpeditionStepResult{}, err
			}
			return ExpeditionStepResult{
				Expedition: *exp,
				EventType:  EventStairs,
				Message:    fmt.Sprintf("階段を発見し、地下 %d 階へ降りた！", exp.CurrentFloor),
			}, nil
		}
		// If last floor has no boss, stair completes dungeon
		return s.handleDungeonClear(ctx, exp, &char, dungeon)

	case 'B': // Boss Battle
		bossMonster := floor.Boss
		if bossMonster == nil && len(floor.Monsters) > 0 {
			bossMonster = &floor.Monsters[0]
		}
		if bossMonster == nil {
			return s.handleDungeonClear(ctx, exp, &char, dungeon)
		}
		return s.resolveBossCombat(ctx, exp, &char, dungeon, *bossMonster)

	case 'E': // Safe Escape Exit
		return s.handleEscape(ctx, exp, &char, "脱出の魔法陣を発見し、無事に帰還した！")

	default:
		if err := s.repo.SaveActiveExpedition(ctx, *exp); err != nil {
			return ExpeditionStepResult{}, err
		}
		return ExpeditionStepResult{
			Expedition: *exp,
			EventType:  EventMove,
			Message:    "通路を進んだ。",
		}, nil
	}
}

func (s *Service) Escape(ctx context.Context, characterID string) (ExpeditionStepResult, error) {
	if characterID == "" {
		return ExpeditionStepResult{}, ErrCharacterNotFound
	}
	exp, err := s.repo.GetActiveExpedition(ctx, characterID)
	if err != nil {
		return ExpeditionStepResult{}, err
	}
	if exp == nil || exp.Status != StatusExploring {
		return ExpeditionStepResult{}, ErrNoActiveExpedition
	}
	char, err := s.characterRepo.FindByID(ctx, characterID)
	if err != nil {
		return ExpeditionStepResult{}, ErrCharacterNotFound
	}
	return s.handleEscape(ctx, exp, &char, "探索を中断し、戦利品を持って無事に脱出した！")
}

func (s *Service) resolveMonsterCombat(
	ctx context.Context,
	exp *ActiveExpedition,
	char *corecharacter.Character,
	monster DungeonMonster,
	eventType TileEventType,
) (ExpeditionStepResult, error) {
	req := corebattle.Request{
		Participants: []corebattle.Participant{
			corebattle.NewParticipantFromCharacterWithHP(*char, exp.CurrentHP),
			corebattle.MustNewParticipant(monster.ID, monster.HP, monster.Attack, monster.Defense),
		},
	}

	battleRes, err := s.battleEngine.Resolve(req)
	if err != nil {
		return ExpeditionStepResult{}, err
	}

	if battleRes.Outcome == corebattle.OutcomeWin && battleRes.WinnerID == char.ID {
		exp.AccumulatedExp += monster.ExpReward
		exp.AccumulatedGold += monster.GoldReward
		if monster.DropItemID != "" {
			exp.AccumulatedItems = append(exp.AccumulatedItems, monster.DropItemID)
		}
		if err := s.repo.SaveActiveExpedition(ctx, *exp); err != nil {
			return ExpeditionStepResult{}, err
		}
		if s.monsterDefeatedHook != nil {
			_ = s.monsterDefeatedHook(ctx, char.ID, 1)
		}
		return ExpeditionStepResult{
			Expedition:   *exp,
			EventType:    eventType,
			BattleResult: &battleRes,
			ExpEarned:    monster.ExpReward,
			GoldFound:    monster.GoldReward,
			ItemFound:    monster.DropItemID,
			Message:      fmt.Sprintf("%s との戦闘に勝利した！ (EXP: +%d, G: +%d)", monster.Name, monster.ExpReward, monster.GoldReward),
		}, nil
	}

	// Player Defeated
	exp.CurrentHP = 0
	return s.handleWipeout(ctx, exp, char, fmt.Sprintf("%s との戦いに敗れ、全滅してしまった…", monster.Name))
}

func (s *Service) resolveBossCombat(
	ctx context.Context,
	exp *ActiveExpedition,
	char *corecharacter.Character,
	dungeon Dungeon,
	bossMonster DungeonMonster,
) (ExpeditionStepResult, error) {
	req := corebattle.Request{
		Participants: []corebattle.Participant{
			corebattle.NewParticipantFromCharacterWithHP(*char, exp.CurrentHP),
			corebattle.MustNewParticipant(bossMonster.ID, bossMonster.HP, bossMonster.Attack, bossMonster.Defense),
		},
	}

	battleRes, err := s.battleEngine.Resolve(req)
	if err != nil {
		return ExpeditionStepResult{}, err
	}

	if battleRes.Outcome == corebattle.OutcomeWin && battleRes.WinnerID == char.ID {
		exp.AccumulatedExp += bossMonster.ExpReward
		exp.AccumulatedGold += bossMonster.GoldReward
		if bossMonster.DropItemID != "" {
			exp.AccumulatedItems = append(exp.AccumulatedItems, bossMonster.DropItemID)
		}
		if s.monsterDefeatedHook != nil {
			_ = s.monsterDefeatedHook(ctx, char.ID, 1)
		}
		return s.handleDungeonClear(ctx, exp, char, dungeon)
	}

	// Defeat by Boss
	exp.CurrentHP = 0
	return s.handleWipeout(ctx, exp, char, fmt.Sprintf("フロアボス %s の圧倒的な力の前に敗れ去った…", bossMonster.Name))
}

func (s *Service) handleDungeonClear(
	ctx context.Context,
	exp *ActiveExpedition,
	char *corecharacter.Character,
	dungeon Dungeon,
) (ExpeditionStepResult, error) {
	now := time.Now().UTC()
	exp.Status = StatusCleared
	exp.AccumulatedExp += dungeon.ClearExpBonus
	exp.AccumulatedGold += dungeon.ClearGoldBonus
	exp.UpdatedAt = now

	// 1. Commit accumulated rewards to character
	if exp.AccumulatedExp > 0 {
		_, _ = progression.ApplyExperience(char, exp.AccumulatedExp)
	}
	_ = char.AddMoney(exp.AccumulatedGold)
	medalsBonus := dungeon.Tier
	exp.AccumulatedMedals += medalsBonus
	_ = char.AddSmallMedals(exp.AccumulatedMedals)

	rewardItems := make([]coreitem.Instance, 0, len(exp.AccumulatedItems))
	for _, defID := range exp.AccumulatedItems {
		itemID := id.New()
		rewardItems = append(rewardItems, coreitem.Instance{
			ID:               itemID,
			DefinitionID:     defID,
			Quantity:         1,
			EnhancementLevel: 0,
		})
	}

	// 2. Update Dungeon Record
	rec, _ := s.repo.GetRecord(ctx, char.ID)
	rec.TotalExpeditions++
	rec.TotalFloorsCleared += exp.CurrentFloor
	if dungeon.Tier > rec.HighestDungeonCleared {
		rec.HighestDungeonCleared = dungeon.Tier
	}

	// 3. Save History
	histID := id.New()
	history := DungeonExpeditionHistory{
		ID:               histID,
		CharacterID:      char.ID,
		DungeonID:        dungeon.ID,
		FloorsReached:    exp.CurrentFloor,
		Outcome:          StatusCleared,
		ExpReward:        exp.AccumulatedExp,
		GoldReward:       exp.AccumulatedGold,
		MedalsReward:     exp.AccumulatedMedals,
		ItemsRewardCount: len(rewardItems),
		CreatedAt:        now,
	}

	if err := s.repo.FinalizeExpedition(ctx, history, rec, char, rewardItems); err != nil {
		return ExpeditionStepResult{}, err
	}

	_ = s.repo.DeleteActiveExpedition(ctx, char.ID)

	return ExpeditionStepResult{
		Expedition:  *exp,
		EventType:   EventBoss,
		ExpEarned:   exp.AccumulatedExp,
		GoldFound:   exp.AccumulatedGold,
		MedalsFound: exp.AccumulatedMedals,
		IsFinished:  true,
		Message:     fmt.Sprintf("ダンジョン「%s」を踏破・完全制覇した！ (EXP: +%d, Gold: +%d, メダル: %d枚, アイテム: %d個)", dungeon.Name, exp.AccumulatedExp, exp.AccumulatedGold, exp.AccumulatedMedals, len(rewardItems)),
	}, nil
}

func (s *Service) handleEscape(
	ctx context.Context,
	exp *ActiveExpedition,
	char *corecharacter.Character,
	msg string,
) (ExpeditionStepResult, error) {
	now := time.Now().UTC()
	exp.Status = StatusEscaped
	exp.UpdatedAt = now

	// Transfer accumulated EXP & Gold to character
	if exp.AccumulatedExp > 0 {
		_, _ = progression.ApplyExperience(char, exp.AccumulatedExp)
	}
	_ = char.AddMoney(exp.AccumulatedGold)
	_ = char.AddSmallMedals(exp.AccumulatedMedals)

	rewardItems := make([]coreitem.Instance, 0, len(exp.AccumulatedItems))
	for _, defID := range exp.AccumulatedItems {
		itemID := id.New()
		rewardItems = append(rewardItems, coreitem.Instance{
			ID:               itemID,
			DefinitionID:     defID,
			Quantity:         1,
			EnhancementLevel: 0,
		})
	}

	rec, _ := s.repo.GetRecord(ctx, char.ID)
	rec.TotalExpeditions++
	rec.TotalFloorsCleared += exp.CurrentFloor

	histID := id.New()
	history := DungeonExpeditionHistory{
		ID:               histID,
		CharacterID:      char.ID,
		DungeonID:        exp.DungeonID,
		FloorsReached:    exp.CurrentFloor,
		Outcome:          StatusEscaped,
		ExpReward:        exp.AccumulatedExp,
		GoldReward:       exp.AccumulatedGold,
		MedalsReward:     exp.AccumulatedMedals,
		ItemsRewardCount: len(rewardItems),
		CreatedAt:        now,
	}

	if err := s.repo.FinalizeExpedition(ctx, history, rec, char, rewardItems); err != nil {
		return ExpeditionStepResult{}, err
	}

	_ = s.repo.DeleteActiveExpedition(ctx, char.ID)

	return ExpeditionStepResult{
		Expedition:  *exp,
		EventType:   EventEscape,
		ExpEarned:   exp.AccumulatedExp,
		GoldFound:   exp.AccumulatedGold,
		MedalsFound: exp.AccumulatedMedals,
		IsFinished:  true,
		Message:     msg,
	}, nil
}

func (s *Service) handleWipeout(
	ctx context.Context,
	exp *ActiveExpedition,
	char *corecharacter.Character,
	msg string,
) (ExpeditionStepResult, error) {
	now := time.Now().UTC()
	exp.Status = StatusWipedOut
	exp.UpdatedAt = now

	rec, _ := s.repo.GetRecord(ctx, char.ID)
	rec.TotalExpeditions++

	histID := id.New()
	history := DungeonExpeditionHistory{
		ID:               histID,
		CharacterID:      char.ID,
		DungeonID:        exp.DungeonID,
		FloorsReached:    exp.CurrentFloor,
		Outcome:          StatusWipedOut,
		ExpReward:        0,
		GoldReward:       0,
		ItemsRewardCount: 0,
		CreatedAt:        now,
	}

	// Wiping out forfeits unbanked ledger rewards (0 EXP, 0 Gold, 0 items awarded)
	if err := s.repo.FinalizeExpedition(ctx, history, rec, char, nil); err != nil {
		return ExpeditionStepResult{}, err
	}

	_ = s.repo.DeleteActiveExpedition(ctx, char.ID)

	return ExpeditionStepResult{
		Expedition: *exp,
		EventType:  EventWipeout,
		IsFinished: true,
		Message:    msg,
	}, nil
}

func (s *Service) GetHistory(ctx context.Context, characterID string, limit int) ([]DungeonExpeditionHistory, error) {
	if characterID == "" {
		return nil, ErrCharacterNotFound
	}
	if limit <= 0 {
		limit = 20
	}
	return s.repo.GetHistory(ctx, characterID, limit)
}

func (s *Service) GetRecord(ctx context.Context, characterID string) (CharacterDungeonRecord, error) {
	if characterID == "" {
		return CharacterDungeonRecord{}, ErrCharacterNotFound
	}
	return s.repo.GetRecord(ctx, characterID)
}

func EncodeItems(items []string) string {
	b, _ := json.Marshal(items)
	return string(b)
}

func DecodeItems(data string) []string {
	var items []string
	_ = json.Unmarshal([]byte(data), &items)
	if items == nil {
		items = []string{}
	}
	return items
}
