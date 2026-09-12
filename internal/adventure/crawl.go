package adventure

import (
	"errors"
	"fmt"
	"math/rand"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
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
	Rng          func(n int) int
}

// DungeonCrawlResult contains the complete summary of a 10-floor dungeon crawl and treasure room.
type DungeonCrawlResult struct {
	AdventureIDs   map[string]string    `json:"adventure_ids"` // characterID -> adventureID
	StageID        string               `json:"stage_id"`
	StageName      string               `json:"stage_name"`
	FloorsCleared  int                  `json:"floors_cleared"`
	StageCleared   bool                 `json:"stage_cleared"`
	Outcome        corebattle.Outcome   `json:"outcome"`
	TotalTurns     int                  `json:"total_turns"`
	TotalEXP       int                  `json:"total_exp"`
	TotalGold      int                  `json:"total_gold"`
	FloorResults   []DungeonFloorResult `json:"floor_results"`
	TreasureBoxes  []TreasureBox        `json:"treasure_boxes,omitempty"`
	PartySize      int                  `json:"party_size"`
	ParticipantHPs map[string]int       `json:"participant_hps"`
}

// CrawlSession tracks step-by-step state across the 10-floor dungeon crawl and Floor 11 treasure room.
type CrawlSession struct {
	ID            string                    `json:"id"`
	StageID       string                    `json:"stage_id"`
	StageName     string                    `json:"stage_name"`
	LeaderID      string                    `json:"leader_id"`
	CharacterIDs  []string                  `json:"character_ids"`
	Characters    []corecharacter.Character `json:"-"`
	CurrentFloor  int                       `json:"current_floor"`
	FloorsCleared int                       `json:"floors_cleared"`
	StageCleared  bool                      `json:"stage_cleared"`
	Concluded     bool                      `json:"concluded"`
	Outcome       corebattle.Outcome        `json:"outcome"`
	TotalTurns    int                       `json:"total_turns"`
	TotalEXP      int                       `json:"total_exp"`
	TotalGold     int                       `json:"total_gold"`
	FloorResults  []DungeonFloorResult      `json:"floor_results"`
	TreasureBoxes []TreasureBox             `json:"treasure_boxes,omitempty"`
	Participants  []corebattle.Participant  `json:"participants"`
	Rng           func(n int) int           `json:"-"`
}

// NewCrawlSession creates an initialized 10-floor crawl session at Floor 1.
func NewCrawlSession(
	stage Stage,
	characters []corecharacter.Character,
	rng func(n int) int,
) (*CrawlSession, error) {
	if len(characters) == 0 {
		return nil, ErrNoParticipants
	}
	if len(characters) > 4 {
		return nil, ErrTooManyParticipants
	}
	if rng == nil {
		rng = rand.Intn
	}

	charIDs := make([]string, len(characters))
	participants := make([]corebattle.Participant, len(characters))
	for i, c := range characters {
		charIDs[i] = c.ID
		participants[i] = corebattle.NewParticipantFromCharacter(c)
	}

	return &CrawlSession{
		ID:           id.New(),
		StageID:      stage.ID,
		StageName:    stage.Name,
		LeaderID:     characters[0].ID,
		CharacterIDs: charIDs,
		Characters:   characters,
		CurrentFloor: 1,
		Participants: participants,
		Rng:          rng,
	}, nil
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

	stage, err := stages.FindByID(s.StageID)
	if err != nil {
		return DungeonFloorResult{}, err
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
			floorEXP += m.ExperienceReward
			floorGold += m.GoldReward
			enemies = append(enemies, p)
		}
	}

	if len(enemies) == 0 {
		// Fallback placeholder if catalogs were missing entry
		enemies = append(enemies, corebattle.MustNewParticipant("stray-1", 10, 5, 2))
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
		s.TotalEXP = s.TotalEXP / 2
		s.TotalGold = 0
		return res, nil
	}

	// Victory on floor
	s.FloorsCleared = s.CurrentFloor
	s.TotalEXP += battleRes.TotalReward.Experience
	s.TotalGold += battleRes.TotalReward.Currency

	if isBoss {
		s.StageCleared = true
		s.spawnTreasureBoxes()
	}

	s.CurrentFloor++
	return res, nil
}

func (s *CrawlSession) spawnTreasureBoxes() {
	aliveCount := 0
	hasMerchant := false
	hasTreasureHunter := false
	hasLuckyPendant := false

	for _, c := range s.Characters {
		isAlive := true
		for _, p := range s.Participants {
			if p.ID == c.ID && p.HP <= 0 {
				isAlive = false
				break
			}
		}
		if !isAlive {
			continue
		}
		aliveCount++
		if c.JobID == "merchant" || c.JobID == "7" {
			hasMerchant = true
		}
		if c.JobID == "treasure_hunter" || c.JobID == "78" {
			hasTreasureHunter = true
		}
	}

	boxCount := CalculateTreasureCount(TreasureCalculationInput{
		AliveMembers:      aliveCount,
		HasMerchant:       hasMerchant,
		HasTreasureHunter: hasTreasureHunter,
		HasLuckyPendant:   hasLuckyPendant,
		Rng:               s.Rng,
	})

	s.TreasureBoxes = GenerateTreasureBoxes(boxCount, []string{"item-001", "item-002"}, s.Rng)
}

// ExamineTreasure opens a treasure box on Floor 11 (@しらべる).
func (s *CrawlSession) ExamineTreasure(characterID string) (*TreasureBox, error) {
	if s.CurrentFloor != TreasureRoomFloor {
		return nil, ErrInvalidFloorAction
	}
	// Find character and check alive
	for _, p := range s.Participants {
		if p.ID == characterID && p.HP <= 0 {
			return nil, errors.New("unconscious character cannot open treasure")
		}
	}

	for i := range s.TreasureBoxes {
		if s.TreasureBoxes[i].OpenedBy == "" {
			s.TreasureBoxes[i].OpenedBy = characterID
			s.TreasureBoxes[i].DeliveredTo = "inventory"
			return &s.TreasureBoxes[i], nil
		}
	}
	return nil, errors.New("no unopened treasure boxes remaining")
}

// Result compiles the final outcome of the dungeon crawl.
func (s *CrawlSession) Result() DungeonCrawlResult {
	participantHPs := make(map[string]int, len(s.Participants))
	for _, p := range s.Participants {
		participantHPs[p.ID] = p.HP
	}

	outcome := s.Outcome
	if outcome == "" {
		if s.StageCleared {
			outcome = corebattle.OutcomeWin
		} else {
			outcome = corebattle.OutcomeDefeat
		}
	}

	return DungeonCrawlResult{
		AdventureIDs:   make(map[string]string),
		StageID:        s.StageID,
		StageName:      s.StageName,
		FloorsCleared:  s.FloorsCleared,
		StageCleared:   s.StageCleared,
		Outcome:        outcome,
		TotalTurns:     s.TotalTurns,
		TotalEXP:       s.TotalEXP,
		TotalGold:      s.TotalGold,
		FloorResults:   s.FloorResults,
		TreasureBoxes:  s.TreasureBoxes,
		PartySize:      len(s.Characters),
		ParticipantHPs: participantHPs,
	}
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
