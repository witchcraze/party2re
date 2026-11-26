package party

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/pagination"
)

type Service struct {
	lobbyBoundary        func(context.Context, string, func(context.Context) error) error
	activityCheck        func(context.Context, string) error
	repo                 Repository
	charRepo             CharacterRepository
	invRepo              InventoryRepository
	advRepo              AdventureRepository
	stages               StageProvider
	monsters             MonsterProvider
	battleEngine         BattleEngine
	news                 NewsPublisher
	txProvider           TransactionProvider
	postAdventureHook    PostAdventureHook
	unsealHook           adventure.UnsealHook
	participantBuilder   ParticipantBuilder
	lifetimeActorBuilder LifetimeActorBuilder
	battleSettler        PostBattleSettler
	depotRepo            DepotRepository
	blessingProvider     BlessingProvider
	timer                TimerService
	rng                  func(n int) int
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
	for i := range items {
		items[i].StageName = s.stageName(items[i].StageID)
	}
	return pagination.NewPage(items, total, limit, offset), nil
}

// GetPartySummary returns one lobby's public list row.
func (s *Service) GetPartySummary(ctx context.Context, partyID string) (PartySummary, error) {
	detail, err := s.GetParty(ctx, partyID)
	if err != nil {
		return PartySummary{}, err
	}
	summary := newSummary(detail.Party, detail.Members)
	summary.StageName = s.stageName(summary.StageID)
	return summary, nil
}

func (s *Service) stageName(id string) string {
	if s.stages == nil {
		return ""
	}
	st, err := s.stages.FindByID(id)
	if err != nil {
		return ""
	}
	return st.Name
}

func (s *Service) buildBattleActor(ctx context.Context, characterID string) (*battle.LifetimeActor, error) {
	if s.lifetimeActorBuilder == nil {
		return nil, nil
	}
	actor, err := s.lifetimeActorBuilder.BuildLifetimeActor(ctx, characterID, "")
	if err != nil {
		return nil, err
	}
	return &actor, nil
}

// JoinParty joins a character into an existing recruiting party.
func (s *Service) JoinParty(ctx context.Context, partyID, characterID, password string) (PartyDetail, error) {
	var detail PartyDetail

	err := s.withLobbyMutation(ctx, partyID, func(txCtx context.Context) error {
		p, err := s.repo.GetPartyForUpdate(txCtx, partyID)
		if err != nil {
			return err
		}
		if err := s.joinWaitingLobby(txCtx, p, characterID, password); err != nil {
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

func (s *Service) joinWaitingLobby(txCtx context.Context, p Party, characterID, password string) error {
	if p.Status != StatusRecruiting {
		return ErrPartyNotRecruiting
	}

	count, err := s.repo.CountMembers(txCtx, p.ID)
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

	char, err := s.charRepo.FindByIDForUpdate(txCtx, characterID)
	if err != nil {
		return err
	}
	// Joins to different lobbies hold different lobby locks; the character row
	// lock serializes them, so membership is read only after taking it.
	if _, _, err := s.repo.GetActivePartyByCharacter(txCtx, characterID); err == nil {
		return ErrAlreadyInParty
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
	// Only the creator passes the stage gate (quest.cgi:518–524 vs add_member).
	if p.NeedJoin != "" {
		if err := ValidateNeedJoin(p.NeedJoin, char); err != nil {
			return err
		}
	}

	battleActor, err := s.buildBattleActor(txCtx, char.ID)
	if err != nil {
		return err
	}
	maxHP := char.Stats.MaxHP
	if battleActor != nil {
		maxHP = battleActor.MaxHP
	}

	return s.repo.AddMember(txCtx, Member{
		PartyID:       p.ID,
		CharacterID:   char.ID,
		CharacterName: char.Name,
		JobID:         char.JobID,
		Level:         char.Level,
		HP:            char.Stats.HP,
		MaxHP:         maxHP,
		IsLeader:      false,
		JoinedAt:      time.Now().UTC(),
		BattleActor:   battleActor,
	})
}

// LeaveParty removes the character from the lobby. When the leader leaves,
// the earliest remaining member leads; the last leave deletes the lobby
// (_battle.cgi:1330–1346).
func (s *Service) LeaveParty(ctx context.Context, partyID, characterID string) error {
	return s.withLobbyMutation(ctx, partyID, func(txCtx context.Context) error {
		if _, err := s.repo.GetPartyForUpdate(txCtx, partyID); err != nil {
			return err
		}
		members, err := s.repo.GetMembers(txCtx, partyID)
		if err != nil {
			return err
		}
		if !slices.ContainsFunc(members, func(m Member) bool { return m.CharacterID == characterID }) {
			return ErrCharacterNotInParty
		}
		if len(members) == 1 {
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

		// Absent targets are ErrCharacterNotInParty; storage failures propagate.
		if _, err := s.repo.GetMember(txCtx, partyID, targetCharID); err != nil {
			return err
		}

		return s.repo.RemoveMember(txCtx, partyID, targetCharID)
	})
}
