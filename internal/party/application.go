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
	actionWait           ActionWaitWriter
	tavernExit           TavernExitSettler
	clock                func() time.Time
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
		members, err := s.joinWaitingLobby(txCtx, p, characterID, password)
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

func (s *Service) joinWaitingLobby(txCtx context.Context, p Party, characterID, password string) ([]Member, error) {
	if p.Status != StatusRecruiting {
		return nil, ErrPartyNotRecruiting
	}

	members, err := s.repo.GetMembers(txCtx, p.ID)
	if err != nil {
		return nil, err
	}
	if len(members) >= p.MaxMembers {
		return nil, ErrPartyFull
	}

	if p.PasswordHash != "" {
		if hashPassword(password) != p.PasswordHash {
			return nil, ErrInvalidPassword
		}
	}

	char, err := s.charRepo.FindByIDForUpdate(txCtx, characterID)
	if err != nil {
		return nil, err
	}
	// Joins to different lobbies hold different lobby locks; the character row
	// lock serializes them, so membership is read only after taking it.
	if err := s.ensureCharacterNotInParty(txCtx, characterID); err != nil {
		return nil, err
	}
	if s.activityCheck != nil {
		if err := s.activityCheck(txCtx, characterID); err != nil {
			return nil, err
		}
	}
	if char.Stats.HP <= 0 {
		return nil, ErrCharacterUnconscious
	}
	if char.Tired >= 100 {
		return nil, ErrCharacterExhausted
	}
	// Only the creator passes the stage gate (quest.cgi:518–524 vs add_member).
	if p.NeedJoin != "" {
		if err := ValidateNeedJoin(p.NeedJoin, char); err != nil {
			return nil, err
		}
	}

	battleActor, err := s.buildBattleActor(txCtx, char.ID)
	if err != nil {
		return nil, err
	}
	maxHP := char.Stats.MaxHP
	if battleActor != nil {
		maxHP = battleActor.MaxHP
	}

	// Build the reply from the locked pre-write roster and accepted member so a
	// post-write read failure never disguises an applied join as an unknown outcome.
	joined := Member{
		PartyID:       p.ID,
		CharacterID:   char.ID,
		CharacterName: char.Name,
		JobID:         char.JobID,
		Level:         char.Level,
		HP:            char.Stats.HP,
		MaxHP:         maxHP,
		IsLeader:      false,
		JoinedAt:      s.now(),
		BattleActor:   battleActor,
	}
	if err := s.repo.AddMember(txCtx, joined); err != nil {
		return nil, err
	}
	return mergeJoinedMember(members, joined), nil
}

func (s *Service) ensureCharacterNotInParty(ctx context.Context, characterID string) error {
	_, _, err := s.repo.GetActivePartyByCharacter(ctx, characterID)
	if err == nil {
		return ErrAlreadyInParty
	}
	if !errors.Is(err, ErrNotFound) {
		return err
	}
	return nil
}

func mergeJoinedMember(existing []Member, joined Member) []Member {
	merged := make([]Member, 0, len(existing)+1)
	replaced := false
	for _, m := range existing {
		if m.CharacterID == joined.CharacterID {
			merged = append(merged, joined)
			replaced = true
			continue
		}
		merged = append(merged, m)
	}
	if !replaced {
		merged = append(merged, joined)
	}
	slices.SortStableFunc(merged, func(a, b Member) int {
		if a.IsLeader != b.IsLeader {
			if a.IsLeader {
				return -1
			}
			return 1
		}
		return a.JoinedAt.Compare(b.JoinedAt)
	})
	return merged
}

// LeaveParty removes the character from the lobby and applies the legacy
// round-0 にげる effects (_battle.cgi:1184–1222, 1330–1375). When the leader
// leaves, the earliest remaining member leads; the last leave deletes the lobby.
// Transaction: withLobbyMutation (RunInTx); Lock Order: party(0) -> characters(2) -> tavern(8).
func (s *Service) LeaveParty(ctx context.Context, partyID, characterID string) (LeaveResult, error) {
	var result LeaveResult
	err := s.withLobbyMutation(ctx, partyID, func(txCtx context.Context) error {
		p, err := s.repo.GetPartyForUpdate(txCtx, partyID)
		if err != nil {
			return err
		}
		members, err := s.repo.GetMembers(txCtx, partyID)
		if err != nil {
			return err
		}
		idx := slices.IndexFunc(members, func(m Member) bool { return m.CharacterID == characterID })
		if idx < 0 {
			return ErrCharacterNotInParty
		}
		leaving := members[idx]

		char, err := s.charRepo.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}
		hp, mp := leaving.HP, -1
		if leaving.BattleActor != nil {
			hp, mp = leaving.BattleActor.HP, leaving.BattleActor.MP
		}
		char.ApplyCombatSurvival(hp, mp, hp <= 0)
		char.AddTired(adventure.CalculateExitFatigue(0, p.Mode))
		if err := s.charRepo.Update(txCtx, char); err != nil {
			return err
		}

		if s.tavernExit != nil {
			if err := s.tavernExit.ResetFullness(txCtx, characterID); err != nil {
				return err
			}
			delivery, err := s.tavernExit.SettleDelivery(txCtx, characterID)
			if err != nil {
				return err
			}
			result.Delivery = delivery
		}

		if len(members) == 1 {
			return s.repo.DeleteParty(txCtx, partyID)
		}
		return s.repo.RemoveMember(txCtx, partyID, characterID)
	})
	if err != nil {
		return LeaveResult{}, err
	}
	return result, nil
}

func (s *Service) now() time.Time {
	if s.clock != nil {
		return s.clock().UTC()
	}
	return time.Now().UTC()
}

// KickMember removes a member from the recruiting lobby (leader only) and
// imposes the legacy 60-second action wait (_skill.cgi:796-837).
func (s *Service) KickMember(ctx context.Context, partyID, leaderCharID, targetCharID string) (KickResult, error) {
	if leaderCharID == targetCharID {
		return KickResult{}, ErrCannotKickSelf
	}

	var result KickResult
	err := s.withLobbyMutation(ctx, partyID, func(txCtx context.Context) error {
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

		// Absent targets are ErrCharacterNotInParty; storage failures propagate.
		target, err := s.repo.GetMember(txCtx, partyID, targetCharID)
		if err != nil {
			return err
		}
		leader, err := s.repo.GetMember(txCtx, partyID, leaderCharID)
		if err != nil && !errors.Is(err, ErrCharacterNotInParty) {
			return err
		}

		// Impose the 60s wait before removing the member (lib/_skill.cgi:815-817),
		// so a wait write failure never removes the target without a wait or
		// requires a compensatory AddMember that could overwrite another lobby.
		if s.actionWait != nil {
			if err := s.actionWait.ImposeActionWaitUntil(txCtx, targetCharID, s.now().Add(KickActionWait)); err != nil {
				return err
			}
		}
		if err := s.repo.RemoveMember(txCtx, partyID, targetCharID); err != nil {
			return err
		}
		result = KickResult{PartyName: p.Name, LeaderName: leader.CharacterName, TargetName: target.CharacterName}
		return nil
	})
	if err != nil {
		return KickResult{}, err
	}
	return result, nil
}
