package adventure

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/random"
	"github.com/witchcraze/party2re/internal/id"
)

const (
	BossFloor         = 10
	TreasureRoomFloor = 11
	//lint:ignore unused legacy floor limit constant
	MaxDungeonFloors = 11
)

var (
	ErrNoParticipants       = errors.New("adventure requires at least 1 participant")
	ErrTooManyParticipants  = errors.New("adventure supports at most 4 participants")
	ErrDungeonCrawlFinished = errors.New("dungeon crawl already concluded")
	ErrInvalidFloorAction   = errors.New("invalid floor action for current crawl state")
	ErrLeaderOnly           = errors.New("only the party leader can advance the floor")
	ErrTreasureLimitReached = errors.New("character has reached treasure inspection limit")
	ErrUnconsciousCharacter = errors.New("unconscious character cannot open treasure")
	ErrNotParticipant       = errors.New("character is not a participant in this adventure")
	ErrNoUnopenedTreasure   = errors.New("no unopened treasure boxes remaining")
)

// DungeonFloorResult records the outcome of a single dungeon floor.
type DungeonFloorResult struct {
	Floor          int                          `json:"floor"`
	IsBoss         bool                         `json:"is_boss"`
	IsTreasureRoom bool                         `json:"is_treasure_room"`
	Enemies        []corebattle.Participant     `json:"enemies,omitempty"`
	BattleResult   corebattle.PartyBattleResult `json:"battle_result,omitempty"`
	Cleared        bool                         `json:"cleared"`
}

// DungeonCrawlRequest specifies the parameters to initiate a 10-floor dungeon crawl.
type DungeonCrawlRequest struct {
	CharacterIDs []string
	StageID      string
	Mode         string
	Rng          func(n int) int
}

// DungeonCrawlResult contains the complete summary of a 10-floor dungeon crawl and treasure room.
type DungeonCrawlResult struct {
	AdventureIDs   map[string]string                     `json:"adventure_ids"` // characterID -> adventureID
	StageID        string                                `json:"stage_id"`
	StageName      string                                `json:"stage_name"`
	Mode           string                                `json:"mode,omitempty"`
	Round          int                                   `json:"round"`
	ExitFatigue    int                                   `json:"exit_fatigue"`
	FloorsCleared  int                                   `json:"floors_cleared"`
	StageCleared   bool                                  `json:"stage_cleared"`
	Outcome        corebattle.Outcome                    `json:"outcome"`
	TotalTurns     int                                   `json:"total_turns"`
	TotalEXP       int                                   `json:"total_exp"`
	TotalGold      int                                   `json:"total_gold"`
	TotalCrystals  int                                   `json:"total_crystals"`
	FloorResults   []DungeonFloorResult                  `json:"floor_results"`
	TreasureBoxes  []TreasureBox                         `json:"treasure_boxes,omitempty"`
	InspectedBoxes map[string]int                        `json:"inspected_boxes,omitempty"`
	PartySize      int                                   `json:"party_size"`
	ParticipantHPs map[string]int                        `json:"participant_hps"`
	ParticipantMPs map[string]int                        `json:"participant_mps,omitempty"`
	ConsumedItems  map[string][]corebattle.ConsumedItem  `json:"consumed_items,omitempty"`
	LostDrops      map[string][]string                   `json:"lost_drops,omitempty"`
	MonsterTames   map[string][]battle.MonsterTameResult `json:"monster_tames,omitempty"`
}

// CrawlSession tracks step-by-step state across the 10-floor dungeon crawl and Floor 11 treasure room.
type CrawlSession struct {
	ID                  string                               `json:"id"`
	StageID             string                               `json:"stage_id"`
	StageName           string                               `json:"stage_name"`
	Stage               Stage                                `json:"-"`
	Mode                string                               `json:"mode,omitempty"`
	LeaderID            string                               `json:"leader_id"`
	CharacterIDs        []string                             `json:"character_ids"`
	Characters          []corecharacter.Character            `json:"-"`
	Round               int                                  `json:"round"`
	CurrentFloor        int                                  `json:"current_floor"`
	FloorsCleared       int                                  `json:"floors_cleared"`
	StageCleared        bool                                 `json:"stage_cleared"`
	Concluded           bool                                 `json:"concluded"`
	Outcome             corebattle.Outcome                   `json:"outcome"`
	TotalTurns          int                                  `json:"total_turns"`
	TotalEXP            int                                  `json:"total_exp"`
	TotalGold           int                                  `json:"total_gold"`
	TotalCrystals       int                                  `json:"total_crystals"`
	FloorResults        []DungeonFloorResult                 `json:"floor_results"`
	TreasureBoxes       []TreasureBox                        `json:"treasure_boxes,omitempty"`
	InspectedBoxes      map[string]int                       `json:"inspected_boxes,omitempty"`
	Participants        []corebattle.Participant             `json:"participants"`
	ConsumedItems       map[string][]corebattle.ConsumedItem `json:"consumed_items,omitempty"`
	HasTreasureBlessing bool                                 `json:"has_treasure_blessing,omitempty"`
	Rng                 func(n int) int                      `json:"-"`
	nowFunc             func() time.Time                     `json:"-"`
}

func (s *CrawlSession) now() time.Time {
	if s.nowFunc != nil {
		return s.nowFunc()
	}
	return time.Now().UTC()
}

// NewCrawlSession creates an initialized 10-floor crawl session at Floor 1.
func NewCrawlSession(
	stage Stage,
	characters []corecharacter.Character,
	rng func(n int) int,
) (*CrawlSession, error) {
	return NewCrawlSessionWithParticipants(stage, characters, nil, rng)
}

// NewCrawlSessionWithParticipants creates a crawl session with pre-built participants.
func NewCrawlSessionWithParticipants(
	stage Stage,
	characters []corecharacter.Character,
	participants []corebattle.Participant,
	rng func(n int) int,
) (*CrawlSession, error) {
	if len(characters) == 0 {
		return nil, ErrNoParticipants
	}
	if len(characters) > 4 {
		return nil, ErrTooManyParticipants
	}
	if rng == nil {
		rng = random.Intn
	}

	stage = stage.SelectSeason(rng)

	charIDs := make([]string, len(characters))
	parts := make([]corebattle.Participant, len(characters))
	for i, c := range characters {
		charIDs[i] = c.ID
		if i < len(participants) && participants[i].ID != "" {
			parts[i] = participants[i]
		} else {
			parts[i] = buildFallbackParticipant(c)
		}
	}

	return &CrawlSession{
		ID:           id.New(),
		StageID:      stage.ID,
		StageName:    stage.Name,
		Stage:        stage,
		Mode:         CrawlModeNormal,
		LeaderID:     characters[0].ID,
		CharacterIDs: charIDs,
		Characters:   characters,
		Round:        0,
		CurrentFloor: 1,
		Participants: parts,
		Rng:          rng,
	}, nil
}

func buildFallbackParticipant(c corecharacter.Character) corebattle.Participant {
	hp := c.Stats.HP
	if hp <= 0 && c.Stats.MaxHP > 0 {
		hp = c.Stats.MaxHP
	}
	p := corebattle.MustNewParticipant(c.ID, hp, c.Stats.Attack, c.Stats.Defense)
	p.Name = c.Name
	p.MaxHP = c.Stats.MaxHP
	p.MP = c.Stats.MP
	p.MaxMP = c.Stats.MaxMP
	p.Agility = c.Stats.Agility
	return p
}

// StageFinder provides stage lookup for crawl advancement.
type StageFinder interface {
	FindByID(id string) (Stage, error)
}

// MonsterFinder provides monster lookup for crawl advancement.
type MonsterFinder interface {
	FindByID(id string) (Monster, error)
}

// AdvanceFloor advances the party through the current floor (vs_monster.cgi @すすむ).
func (s *CrawlSession) AdvanceFloor(
	stages StageFinder,
	monsters MonsterFinder,
	engine corebattle.PartyBattleResolver,
) (DungeonFloorResult, error) {
	if s.Concluded {
		return DungeonFloorResult{}, ErrDungeonCrawlFinished
	}
	if s.CurrentFloor > TreasureRoomFloor {
		s.Concluded = true
		return DungeonFloorResult{}, ErrDungeonCrawlFinished
	}

	s.Round = s.CurrentFloor

	// Floor 11 is the treasure room
	if s.CurrentFloor == TreasureRoomFloor {
		res := DungeonFloorResult{
			Floor:          TreasureRoomFloor,
			IsTreasureRoom: true,
			Cleared:        true,
		}
		s.FloorResults = append(s.FloorResults, res)
		s.Concluded = true
		s.Outcome = corebattle.OutcomeWin
		return res, nil
	}

	stage := s.Stage
	if stage.ID == "" {
		if stages == nil {
			return DungeonFloorResult{}, ErrStageNotFound
		}
		var err error
		stage, err = stages.FindByID(s.StageID)
		if err != nil {
			return DungeonFloorResult{}, err
		}
		stage = stage.SelectSeason(s.Rng)
		s.Stage = stage
	}

	isBoss := s.CurrentFloor == BossFloor
	var enemies []corebattle.Participant

	var floorEXP int
	var floorGold int
	if !isBoss {
		// Floors 1 to 9: spawn normal monsters
		normalIDs := stage.GetNormalMonsterIDs()
		partyCount := len(s.Participants)
		enemyCount := 1 + s.Rng(int(mathClamp(partyCount, 1, 3)))
		for i := 0; i < enemyCount; i++ {
			mID := normalIDs[s.Rng(len(normalIDs))]
			m, err := monsters.FindByID(mID)
			if err != nil {
				continue
			}
			p := corebattle.MustNewParticipant(
				fmt.Sprintf("%s-f%d-%d", m.ID, s.CurrentFloor, i+1),
				m.HP, m.Attack, m.Defense,
			)
			p.Name = m.Name
			p.Agility = m.Agility
			p.MaxHP = m.HP
			p.MP = m.MP
			p.MaxMP = m.MP
			p.ExperienceReward = m.ExperienceReward
			p.GoldReward = m.GoldReward
			floorEXP += m.ExperienceReward
			floorGold += m.GoldReward
			enemies = append(enemies, p)
		}
	} else {
		// Floor 10: spawn stage boss(es)
		bossIDs := stage.GetBossIDs()
		for i, bID := range bossIDs {
			m, err := monsters.FindByID(bID)
			if err != nil {
				continue
			}
			p := corebattle.MustNewParticipant(
				fmt.Sprintf("%s-boss-%d", m.ID, i+1),
				m.HP, m.Attack, m.Defense,
			)
			p.Name = m.Name
			p.Agility = m.Agility
			p.MaxHP = m.HP
			p.MP = m.MP
			p.MaxMP = m.MP
			p.ExperienceReward = m.ExperienceReward
			p.GoldReward = m.GoldReward
			floorEXP += m.ExperienceReward
			floorGold += m.GoldReward
			enemies = append(enemies, p)
		}
	}

	if len(enemies) == 0 {
		// Fallback placeholder if catalogs were missing entry
		p := corebattle.MustNewParticipant("stray-1", 10, 5, 2)
		p.MaxHP = 10
		enemies = append(enemies, p)
	}

	var livingAllies []corebattle.Participant
	for _, p := range s.Participants {
		if p.HP > 0 {
			livingAllies = append(livingAllies, p)
		}
	}
	if len(livingAllies) == 0 {
		s.Concluded = true
		s.Outcome = corebattle.OutcomeDefeat
		return DungeonFloorResult{
			Floor:   s.CurrentFloor,
			IsBoss:  isBoss,
			Cleared: false,
		}, nil
	}

	// Resolve battle for current floor
	req := corebattle.PartyBattleRequest{
		Allies:  livingAllies,
		Enemies: enemies,
		VictoryReward: corebattle.Reward{
			Experience: floorEXP,
			Currency:   floorGold,
		},
	}
	battleRes, err := engine.ResolvePartyBattle(req)
	if err != nil {
		return DungeonFloorResult{}, fmt.Errorf("resolve floor %d battle: %w", s.CurrentFloor, err)
	}

	// Update ally HP/MP state from battle outcome
	for i := range s.Participants {
		id := s.Participants[i].ID
		if hp, ok := battleRes.RemainingHP[id]; ok {
			s.Participants[i].HP = hp
		}
		if mp, ok := battleRes.RemainingMP[id]; ok {
			s.Participants[i].MP = mp
		}
	}
	s.recordConsumedItems(battleRes.ConsumedItems)

	s.TotalTurns += battleRes.Turns
	floorCleared := battleRes.Outcome == corebattle.OutcomeWin
	res := DungeonFloorResult{
		Floor:        s.CurrentFloor,
		IsBoss:       isBoss,
		Enemies:      enemies,
		BattleResult: battleRes,
		Cleared:      floorCleared,
	}
	s.FloorResults = append(s.FloorResults, res)

	if !floorCleared {
		// Defeat! Crawl terminates immediately
		s.Concluded = true
		s.Outcome = corebattle.OutcomeDefeat
		return res, nil
	}

	// Victory on floor
	if s.CurrentFloor > s.FloorsCleared {
		s.FloorsCleared = s.CurrentFloor
	}
	s.TotalEXP += battleRes.TotalReward.Experience
	s.TotalGold += battleRes.TotalReward.Currency
	s.TotalCrystals += battleRes.TotalReward.Crystals

	if isBoss {
		s.StageCleared = true
		s.spawnTreasureBoxes()
	}

	s.CurrentFloor++
	return res, nil
}

// ExamineTreasure opens a treasure box on Floor 11 (@しらべる).
// Each surviving participant has a base allowance of 1 box.
// Opening additional boxes requires and consumes 1 Treasure Pocket (ItemTreasurePocket = "item-215").
func (s *CrawlSession) ExamineTreasure(characterID string) (*TreasureBox, error) {
	if s.CurrentFloor != TreasureRoomFloor {
		return nil, ErrInvalidFloorAction
	}

	var actor *corebattle.Participant
	for i := range s.Participants {
		if s.Participants[i].ID == characterID {
			actor = &s.Participants[i]
			break
		}
	}
	if actor == nil {
		return nil, ErrNotParticipant
	}
	if actor.HP <= 0 {
		return nil, ErrUnconsciousCharacter
	}

	var targetBox *TreasureBox
	for i := range s.TreasureBoxes {
		if s.TreasureBoxes[i].OpenedBy == "" {
			targetBox = &s.TreasureBoxes[i]
			break
		}
	}
	if targetBox == nil {
		return nil, ErrNoUnopenedTreasure
	}

	if s.InspectedBoxes == nil {
		s.InspectedBoxes = make(map[string]int)
	}

	inspected := s.InspectedBoxes[characterID]
	if inspected >= 1 {
		keyIdx := slices.Index(actor.ItemDefinitionIDs, ItemTreasurePocket)
		if keyIdx < 0 {
			return nil, ErrTreasureLimitReached
		}
		actor.ItemDefinitionIDs = slices.Clone(actor.ItemDefinitionIDs)
		actor.ItemDefinitionIDs = slices.Delete(actor.ItemDefinitionIDs, keyIdx, keyIdx+1)
		s.addConsumedItem(characterID, corebattle.ConsumedItem{ID: ItemTreasurePocket, Quantity: 1})
	}

	s.InspectedBoxes[characterID]++
	targetBox.OpenedBy = characterID
	targetBox.DeliveredTo = "inventory"
	return targetBox, nil
}

// Result compiles the final outcome of the dungeon crawl.
func (s *CrawlSession) Result() DungeonCrawlResult {
	participantHPs := make(map[string]int, len(s.Participants))
	participantMPs := make(map[string]int, len(s.Participants))
	for _, p := range s.Participants {
		participantHPs[p.ID] = p.HP
		participantMPs[p.ID] = p.MP
	}

	outcome := s.Outcome
	if outcome == "" {
		if s.StageCleared {
			outcome = corebattle.OutcomeWin
		} else {
			outcome = corebattle.OutcomeDefeat
		}
	}

	mode := s.Mode
	if mode == "" {
		mode = CrawlModeNormal
	}

	return DungeonCrawlResult{
		AdventureIDs:   make(map[string]string),
		StageID:        s.StageID,
		StageName:      s.StageName,
		Mode:           mode,
		Round:          s.Round,
		ExitFatigue:    CalculateExitFatigue(s.Round, mode),
		FloorsCleared:  s.FloorsCleared,
		StageCleared:   s.StageCleared,
		Outcome:        outcome,
		TotalTurns:     s.TotalTurns,
		TotalEXP:       s.TotalEXP,
		TotalGold:      s.TotalGold,
		TotalCrystals:  s.TotalCrystals,
		FloorResults:   s.FloorResults,
		TreasureBoxes:  s.TreasureBoxes,
		InspectedBoxes: cloneInspectedBoxes(s.InspectedBoxes),
		PartySize:      len(s.Characters),
		ParticipantHPs: participantHPs,
		ParticipantMPs: participantMPs,
		ConsumedItems:  cloneConsumedItems(s.ConsumedItems),
	}
}

func cloneInspectedBoxes(m map[string]int) map[string]int {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]int, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func mathClamp(val, min, max int) int {
	if val < min {
		return min
	}
	if val > max {
		return max
	}
	return val
}
