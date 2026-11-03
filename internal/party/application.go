package party

import (
	"context"
	"errors"
	"time"

	"github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/pagination"
)

// VictoryHook is called when a party multiplayer co-op adventure concludes with a victory.
type VictoryHook func(ctx context.Context, characterIDs []string, monstersDefeated int, goldEarned int) error

type Service struct {
	lobbyBoundary      func(context.Context, string, func(context.Context) error) error
	activityCheck      func(context.Context, string) error
	repo               Repository
	charRepo           CharacterRepository
	invRepo            InventoryRepository
	advRepo            AdventureRepository
	stages             StageProvider
	monsters           MonsterProvider
	battleEngine       BattleEngine
	news               NewsPublisher
	txProvider         TransactionProvider
	victoryHook        VictoryHook
	postAdventureHook  PostAdventureHook
	participantBuilder ParticipantBuilder
	battleSettler      PostBattleSettler
	depotRepo          DepotRepository
	blessingProvider   BlessingProvider
	timer              TimerService
	rng                func(n int) int
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
// Lobby metadata lives in Valkey Master; adventure settlement and audit logs commit atomically in MariaDB
// with Rank 2 character locks acquired in ascending ID order.
func (s *Service) runInTx(ctx context.Context, fn func(txCtx context.Context) error) error {
	if s.txProvider != nil {
		return s.txProvider.RunInTx(ctx, fn)
	}
	return fn(ctx)
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

	err := s.withLobbyMutation(ctx, partyID, func(txCtx context.Context) error {
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
		if s.activityCheck != nil {
			if err := s.activityCheck(txCtx, characterID); err != nil {
				return err
			}
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
	return s.withLobbyMutation(ctx, partyID, func(txCtx context.Context) error {
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
			if err := s.repo.UpdateParty(txCtx, p); err != nil {
				return err
			}
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

	return s.withLobbyMutation(ctx, partyID, func(txCtx context.Context) error {
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
	return s.withLobbyMutation(ctx, partyID, func(txCtx context.Context) error {
		p, err := s.repo.GetPartyForUpdate(txCtx, partyID)
		if err != nil {
			return err
		}
		if p.LeaderCharacterID != leaderCharID {
			return ErrNotPartyLeader
		}
		p.Status = StatusDisbanded
		if err := s.repo.UpdateParty(txCtx, p); err != nil {
			return err
		}
		return s.repo.DeleteParty(txCtx, partyID)
	})
}

// SetReady toggles or sets member ready state.
func (s *Service) SetReady(ctx context.Context, partyID, characterID string, ready bool) (PartyDetail, error) {
	var detail PartyDetail

	err := s.withLobbyMutation(ctx, partyID, func(txCtx context.Context) error {
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
