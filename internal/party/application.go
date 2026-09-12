package party

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/pagination"
)

// VictoryHook is called when a party multiplayer co-op adventure concludes with a victory.
type VictoryHook func(ctx context.Context, characterIDs []string, monstersDefeated int, goldEarned int) error

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

func WithVictoryHook(hook VictoryHook) Option {
	return func(s *Service) {
		s.victoryHook = hook
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
	victoryHook  VictoryHook
}

func (s *Service) SetVictoryHook(hook VictoryHook) {
	s.victoryHook = hook
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

// runInTx wraps execution within a MariaDB transaction if a TransactionProvider is configured.
//
// Concurrency & Persistence Boundaries (RFC #356, Issue #368, Issue #380):
//  1. Ephemeral Waiting Lobby (Valkey Master):
//     Lobby metadata, recruitment lists, member rosters, and ready states live exclusively
//     in Valkey Master with automatic TTL (15m lobby, 60s ready check). Valkey operations
//     (including atomic Lua scripts for capacity and single-party membership checks) ensure
//     high-throughput, zero-lock-contention matchmaking without relational database tables.
//     In lobby operations (CreateParty, JoinParty, etc.), runInTx is utilized solely to lock and
//     validate the individual character row in MariaDB (Rank 2 lock hierarchy).
//  2. Adventure Settlement & Durable Audit (MariaDB Master):
//     Multiplayer adventure execution (StartPartyAdventure) executes within a MariaDB runInTx transaction.
//     All participating character records are locked deterministically in ascending ID order (Rank 2)
//     to prevent deadlocks across concurrent multi-character operations. Post-battle stat updates,
//     EXP/Gold gains, item drops, and permanent audit logs (party_adventure_logs) are atomically
//     committed to MariaDB before cleaning up the ephemeral Valkey lobby.
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
		if leaderChar.Tired >= 100 {
			return ErrCharacterExhausted
		}
		if err := s.stages.CanAccessStage(leaderChar, stage.ID); err != nil {
			return err
		}
		if req.NeedJoin != "" {
			if err := ValidateNeedJoin(req.NeedJoin, leaderChar); err != nil {
				return err
			}
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
			NeedJoin:          strings.TrimSpace(req.NeedJoin),
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
	if s.stages != nil {
		for i := range items {
			if st, err := s.stages.FindByID(items[i].StageID); err == nil {
				items[i].StageName = st.Name
			}
		}
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
		if char.Tired >= 100 {
			return ErrCharacterExhausted
		}
		if err := s.stages.CanAccessStage(char, p.StageID); err != nil {
			return err
		}
		if p.NeedJoin != "" {
			if err := ValidateNeedJoin(p.NeedJoin, char); err != nil {
				return err
			}
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
