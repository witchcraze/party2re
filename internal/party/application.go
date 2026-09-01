package party

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/progression"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/pagination"
)

type Option func(*Service)

func WithTransactionProvider(txProvider TransactionProvider) Option {
	return func(s *Service) {
		s.txProvider = txProvider
	}
}

func WithNewsPublisher(news NewsPublisher) Option {
	return func(s *Service) {
		s.news = news
	}
}

type Service struct {
	repo         Repository
	charRepo     CharacterRepository
	invRepo      InventoryRepository
	stages       StageProvider
	monsters     MonsterProvider
	battleEngine BattleEngine
	news         NewsPublisher
	txProvider   TransactionProvider
}

func NewService(
	repo Repository,
	charRepo CharacterRepository,
	invRepo InventoryRepository,
	stages StageProvider,
	monsters MonsterProvider,
	battleEngine BattleEngine,
	opts ...Option,
) (*Service, error) {
	if repo == nil {
		return nil, errors.New("party repository is nil")
	}
	if charRepo == nil {
		return nil, errors.New("character repository is nil")
	}
	if stages == nil {
		return nil, errors.New("stage provider is nil")
	}
	if monsters == nil {
		return nil, errors.New("monster provider is nil")
	}
	if battleEngine == nil {
		battleEngine = battle.Engine{}
	}

	s := &Service{
		repo:         repo,
		charRepo:     charRepo,
		invRepo:      invRepo,
		stages:       stages,
		monsters:     monsters,
		battleEngine: battleEngine,
	}

	for _, opt := range opts {
		opt(s)
	}

	return s, nil
}

func (s *Service) runInTx(ctx context.Context, fn func(txCtx context.Context) error) error {
	if s.txProvider != nil {
		return s.txProvider.RunInTx(ctx, fn)
	}
	return fn(ctx)
}

func hashPassword(pass string) string {
	pass = strings.TrimSpace(pass)
	if pass == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(pass))
	return hex.EncodeToString(sum[:])
}

// CreateParty creates a new party with the creator as leader.
func (s *Service) CreateParty(ctx context.Context, leaderCharID string, req CreatePartyRequest) (PartyDetail, error) {
	if err := ValidatePartyName(req.Name); err != nil {
		return PartyDetail{}, err
	}

	stage, err := s.stages.FindByID(req.StageID)
	if err != nil {
		return PartyDetail{}, ErrStageNotFound
	}

	speed := req.Speed
	if speed != 3 && speed != 18 && speed != 25 {
		speed = DefaultSpeed
	}

	maxMembers := req.MaxMembers
	if maxMembers <= 0 {
		maxMembers = MaxPartyMembers
	}
	if maxMembers < MinPartyMembers || maxMembers > MaxPartyMembers {
		return PartyDetail{}, ErrInvalidMaxMembers
	}

	minLevel := req.MinLevel
	if minLevel <= 0 {
		minLevel = 1
	}
	maxLevel := req.MaxLevel
	if maxLevel <= 0 {
		maxLevel = 999
	}
	minHP := req.MinHP
	if minHP < 0 {
		minHP = 0
	}

	var createdParty Party
	var createdMember Member

	err = s.runInTx(ctx, func(txCtx context.Context) error {
		// Check if leader already belongs to an active party
		_, _, err := s.repo.GetActivePartyByCharacter(txCtx, leaderCharID)
		if err == nil {
			return ErrAlreadyInParty
		}

		leaderChar, err := s.charRepo.FindByIDForUpdate(txCtx, leaderCharID)
		if err != nil {
			return err
		}
		if leaderChar.Stats.HP <= 0 {
			return ErrCharacterUnconscious
		}

		now := time.Now().UTC()
		partyID := id.New()
		passHash := hashPassword(req.Password)

		createdParty = Party{
			ID:                partyID,
			LeaderCharacterID: leaderChar.ID,
			Name:              strings.TrimSpace(req.Name),
			PasswordHash:      passHash,
			StageID:           stage.ID,
			Speed:             speed,
			MaxMembers:        maxMembers,
			MinLevel:          minLevel,
			MaxLevel:          maxLevel,
			MinHP:             minHP,
			Status:            StatusRecruiting,
			CreatedAt:         now,
			UpdatedAt:         now,
		}
		if err := s.repo.SaveParty(txCtx, createdParty); err != nil {
			return err
		}

		createdMember = Member{
			PartyID:       partyID,
			CharacterID:   leaderChar.ID,
			CharacterName: leaderChar.Name,
			JobID:         leaderChar.JobID,
			Level:         leaderChar.Level,
			HP:            leaderChar.Stats.HP,
			MaxHP:         leaderChar.Stats.MaxHP,
			IsLeader:      true,
			ReadyState:    true,
			JoinedAt:      now,
		}
		return s.repo.AddMember(txCtx, createdMember)
	})
	if err != nil {
		return PartyDetail{}, err
	}

	if s.news != nil {
		_ = s.news.PublishNews(
			ctx,
			"party",
			fmt.Sprintf("パーティー『%s』が結成されました！", createdParty.Name),
			fmt.Sprintf("冒険場所：%s、募集人数：%d人", stage.Name, createdParty.MaxMembers),
			createdMember.CharacterName,
			time.Now().UTC(),
		)
	}

	return PartyDetail{
		Party:   createdParty,
		Members: []Member{createdMember},
	}, nil
}

// GetParty retrieves detailed party information including all members.
func (s *Service) GetParty(ctx context.Context, partyID string) (PartyDetail, error) {
	p, err := s.repo.GetParty(ctx, partyID)
	if err != nil {
		return PartyDetail{}, err
	}
	members, err := s.repo.GetMembers(ctx, partyID)
	if err != nil {
		return PartyDetail{}, err
	}
	return PartyDetail{
		Party:   p,
		Members: members,
	}, nil
}

// ListParties lists recruiting or active parties.
func (s *Service) ListParties(ctx context.Context, status string, limit, offset int) (pagination.Page[PartySummary], error) {
	limit, offset = pagination.NormalizeWithDefaults(limit, offset, 50, 100)
	if status == "" {
		status = StatusRecruiting
	}
	items, total, err := s.repo.ListParties(ctx, status, limit, offset)
	if err != nil {
		return pagination.Page[PartySummary]{}, err
	}
	return pagination.NewPage(items, total, limit, offset), nil
}

// JoinParty joins a character into an existing recruiting party.
func (s *Service) JoinParty(ctx context.Context, partyID, characterID, password string) (PartyDetail, error) {
	var detail PartyDetail

	err := s.runInTx(ctx, func(txCtx context.Context) error {
		p, err := s.repo.GetPartyForUpdate(txCtx, partyID)
		if err != nil {
			return err
		}
		if p.Status != StatusRecruiting {
			return ErrPartyNotRecruiting
		}

		count, err := s.repo.CountMembers(txCtx, partyID)
		if err != nil {
			return err
		}
		if count >= p.MaxMembers {
			return ErrPartyFull
		}

		if p.PasswordHash != "" {
			if hashPassword(password) != p.PasswordHash {
				return ErrInvalidPassword
			}
		}

		// Check if already in party
		_, _, err = s.repo.GetActivePartyByCharacter(txCtx, characterID)
		if err == nil {
			return ErrAlreadyInParty
		}

		char, err := s.charRepo.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}
		if char.Stats.HP <= 0 {
			return ErrCharacterUnconscious
		}
		if char.Level < p.MinLevel || char.Level > p.MaxLevel {
			return ErrLevelRequirementNotMet
		}
		if char.Stats.MaxHP < p.MinHP {
			return ErrHPRequirementNotMet
		}

		newMember := Member{
			PartyID:       partyID,
			CharacterID:   char.ID,
			CharacterName: char.Name,
			JobID:         char.JobID,
			Level:         char.Level,
			HP:            char.Stats.HP,
			MaxHP:         char.Stats.MaxHP,
			IsLeader:      false,
			ReadyState:    false,
			JoinedAt:      time.Now().UTC(),
		}
		if err := s.repo.AddMember(txCtx, newMember); err != nil {
			return err
		}

		members, err := s.repo.GetMembers(txCtx, partyID)
		if err != nil {
			return err
		}
		detail = PartyDetail{
			Party:   p,
			Members: members,
		}
		return nil
	})
	if err != nil {
		return PartyDetail{}, err
	}

	return detail, nil
}

// LeaveParty leaves the party. If the leader leaves, the party is disbanded.
func (s *Service) LeaveParty(ctx context.Context, partyID, characterID string) error {
	return s.runInTx(ctx, func(txCtx context.Context) error {
		p, err := s.repo.GetPartyForUpdate(txCtx, partyID)
		if err != nil {
			return err
		}
		member, err := s.repo.GetMember(txCtx, partyID, characterID)
		if err != nil {
			return ErrCharacterNotInParty
		}

		if member.IsLeader {
			p.Status = StatusDisbanded
			_ = s.repo.UpdateParty(txCtx, p)
			return s.repo.DeleteParty(txCtx, partyID)
		}

		return s.repo.RemoveMember(txCtx, partyID, characterID)
	})
}

// KickMember removes a member from the party (leader only).
func (s *Service) KickMember(ctx context.Context, partyID, leaderCharID, targetCharID string) error {
	if leaderCharID == targetCharID {
		return ErrCannotKickSelf
	}

	return s.runInTx(ctx, func(txCtx context.Context) error {
		p, err := s.repo.GetPartyForUpdate(txCtx, partyID)
		if err != nil {
			return err
		}
		if p.LeaderCharacterID != leaderCharID {
			return ErrNotPartyLeader
		}

		_, err = s.repo.GetMember(txCtx, partyID, targetCharID)
		if err != nil {
			return ErrCharacterNotInParty
		}

		return s.repo.RemoveMember(txCtx, partyID, targetCharID)
	})
}

// DisbandParty disbands the entire party (leader only).
func (s *Service) DisbandParty(ctx context.Context, partyID, leaderCharID string) error {
	return s.runInTx(ctx, func(txCtx context.Context) error {
		p, err := s.repo.GetPartyForUpdate(txCtx, partyID)
		if err != nil {
			return err
		}
		if p.LeaderCharacterID != leaderCharID {
			return ErrNotPartyLeader
		}
		p.Status = StatusDisbanded
		_ = s.repo.UpdateParty(txCtx, p)
		return s.repo.DeleteParty(txCtx, partyID)
	})
}

// SetReady toggles or sets member ready state.
func (s *Service) SetReady(ctx context.Context, partyID, characterID string, ready bool) (PartyDetail, error) {
	var detail PartyDetail

	err := s.runInTx(ctx, func(txCtx context.Context) error {
		p, err := s.repo.GetPartyForUpdate(txCtx, partyID)
		if err != nil {
			return err
		}
		_, err = s.repo.GetMember(txCtx, partyID, characterID)
		if err != nil {
			return ErrCharacterNotInParty
		}

		if err := s.repo.UpdateMemberReady(txCtx, partyID, characterID, ready); err != nil {
			return err
		}

		members, err := s.repo.GetMembers(txCtx, partyID)
		if err != nil {
			return err
		}
		detail = PartyDetail{
			Party:   p,
			Members: members,
		}
		return nil
	})
	if err != nil {
		return PartyDetail{}, err
	}

	return detail, nil
}

// StartPartyAdventure starts and resolves a multiplayer co-op adventure for the party.
func (s *Service) StartPartyAdventure(ctx context.Context, partyID, leaderCharID string) (PartyAdventureResult, error) {
	var result PartyAdventureResult

	err := s.runInTx(ctx, func(txCtx context.Context) error {
		// 1. Lock party
		p, err := s.repo.GetPartyForUpdate(txCtx, partyID)
		if err != nil {
			return err
		}
		if p.LeaderCharacterID != leaderCharID {
			return ErrNotPartyLeader
		}
		if p.Status != StatusRecruiting {
			return ErrPartyNotRecruiting
		}

		// 2. Fetch members
		members, err := s.repo.GetMembers(txCtx, partyID)
		if err != nil || len(members) == 0 {
			return ErrPartyNotReady
		}

		// All members must be ready
		for _, m := range members {
			if !m.ReadyState {
				return ErrPartyNotReady
			}
		}

		// 3. Lock all character rows in ascending order to prevent deadlocks
		charIDs := make([]string, len(members))
		for i, m := range members {
			charIDs[i] = m.CharacterID
		}
		sort.Strings(charIDs)

		charMap := make(map[string]corecharacter.Character, len(charIDs))
		for _, cID := range charIDs {
			c, err := s.charRepo.FindByIDForUpdate(txCtx, cID)
			if err != nil {
				return err
			}
			if c.Stats.HP <= 0 {
				return ErrCharacterUnconscious
			}
			charMap[c.ID] = c
		}

		// 4. Resolve stage and monsters
		stage, err := s.stages.FindByID(p.StageID)
		if err != nil {
			return ErrStageNotFound
		}

		var enemies []battle.Participant
		totalMonsterEXP := 0
		totalMonsterGold := 0
		var dropPool []string

		for _, mID := range stage.MonsterIDs {
			m, err := s.monsters.FindByID(mID)
			if err == nil {
				enemies = append(enemies, battle.Participant{
					ID:      m.Name,
					HP:      m.HP,
					Attack:  m.Attack,
					Defense: m.Defense,
				})
				totalMonsterEXP += m.ExperienceReward
				totalMonsterGold += m.GoldReward
				dropPool = append(dropPool, m.DropItemIDs...)
			}
		}
		if len(enemies) == 0 {
			// Fallback placeholder enemy
			enemies = append(enemies, battle.Participant{
				ID:      "野良モンスター",
				HP:      30,
				Attack:  10,
				Defense: 5,
			})
			totalMonsterEXP = 50
			totalMonsterGold = 30
		}

		// 5. Build allies
		var allies []battle.Participant
		for _, m := range members {
			c := charMap[m.CharacterID]
			allies = append(allies, battle.Participant{
				ID:      c.Name,
				HP:      c.Stats.HP,
				Attack:  c.Stats.Attack,
				Defense: c.Stats.Defense,
			})
		}

		// 6. Execute Battle
		battleReq := battle.PartyBattleRequest{
			Allies:  allies,
			Enemies: enemies,
			VictoryReward: battle.Reward{
				Experience: totalMonsterEXP,
				Currency:   totalMonsterGold,
			},
			DefeatReward: battle.Reward{
				Experience: totalMonsterEXP / 4,
				Currency:   0,
			},
		}

		battleRes, err := s.battleEngine.ResolvePartyBattle(battleReq)
		if err != nil {
			return err
		}

		// 7. Distribute Rewards and update each character
		var rewardSummaries []MemberRewardSummary
		outcome := string(battleRes.Outcome)

		for _, m := range members {
			c := charMap[m.CharacterID]
			levelBefore := c.Level
			gainedEXP := battleRes.TotalReward.Experience
			gainedGold := battleRes.TotalReward.Currency

			c.Money += gainedGold
			c.Experience += gainedEXP

			// Process progression level-ups
			reqExp, err := progression.ExperienceForNextLevel(c.Level)
			for err == nil && c.Experience >= reqExp && c.Level < progression.MaxLevelForCharacter(&c) {
				c.Level++
				c.Stats.MaxHP += 3
				c.Stats.Attack += 1
				c.Stats.Defense += 1
				c.Stats.Agility += 1
				reqExp, err = progression.ExperienceForNextLevel(c.Level)
			}

			// Apply HP changes if any fallen
			for _, fallen := range battleRes.AlliesFallen {
				if fallen == c.Name {
					c.Stats.HP = 1 // Survive with 1 HP
				}
			}

			if err := s.charRepo.Update(txCtx, c); err != nil {
				return err
			}

			// Award item drops if victorious
			var drops []coreitem.Instance
			if battleRes.Outcome == battle.OutcomeWin && len(dropPool) > 0 && s.invRepo != nil {
				itemDefID := dropPool[0]
				inst, err := coreitem.NewInstance(itemDefID, 1)
				if err == nil {
					inv, err := s.invRepo.FindByCharacterIDForUpdate(txCtx, c.ID)
					if err == nil {
						_ = inv.Add(inst)
						_ = s.invRepo.Save(txCtx, inv)
						drops = append(drops, inst)
					}
				}
			}

			rewardSummaries = append(rewardSummaries, MemberRewardSummary{
				CharacterID: c.ID,
				Name:        c.Name,
				GainedEXP:   gainedEXP,
				GainedGold:  gainedGold,
				LevelBefore: levelBefore,
				LevelAfter:  c.Level,
				Drops:       drops,
			})
		}

		// 8. Save Adventure Log
		detailsJSON, _ := json.Marshal(battleRes)
		advLog := PartyAdventureLog{
			ID:                  id.New(),
			PartyID:             partyID,
			StageID:             p.StageID,
			Outcome:             outcome,
			Turns:               battleRes.Turns,
			TotalEXP:            battleRes.TotalReward.Experience,
			TotalGold:           battleRes.TotalReward.Currency,
			SynergyBonusPercent: battleRes.BonusPercent,
			DetailsJSON:         string(detailsJSON),
			CreatedAt:           time.Now().UTC(),
		}
		_ = s.repo.SaveAdventureLog(txCtx, advLog)

		// 9. Reset party status & ready states for members
		for _, m := range members {
			_ = s.repo.UpdateMemberReady(txCtx, partyID, m.CharacterID, false)
		}

		result = PartyAdventureResult{
			PartyID:             partyID,
			StageID:             p.StageID,
			Outcome:             outcome,
			Turns:               battleRes.Turns,
			TotalEXP:            battleRes.TotalReward.Experience,
			TotalGold:           battleRes.TotalReward.Currency,
			SynergyBonusPercent: battleRes.BonusPercent,
			Rewards:             rewardSummaries,
			BattleResult:        battleRes,
		}

		return nil
	})
	if err != nil {
		return PartyAdventureResult{}, err
	}

	return result, nil
}
