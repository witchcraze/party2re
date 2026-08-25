package gvg

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/guild"
)

const (
	DefaultRating = 1000
	KFactor       = 32
	MaxRosterSize = 5
)

var (
	ErrInvalidGuildID          = errors.New("invalid guild ID")
	ErrGuildNotFound           = errors.New("guild not found")
	ErrCannotChallengeOwnGuild = errors.New("cannot challenge your own guild")
	ErrUnauthorized            = errors.New("only guild leader or officers can declare GvG matches")
	ErrInsufficientRoster      = errors.New("guild does not have enough active members to battle")
	ErrActorNotInGuild         = errors.New("actor is not in a guild")
	ErrMatchNotFound           = errors.New("gvg match not found")
	ErrCharacterNotFound       = errors.New("character not found")
)

type GvGStanding struct {
	GuildID          string    `json:"guild_id"`
	GuildName        string    `json:"guild_name,omitempty"`
	Rating           int       `json:"rating"`
	Wins             int       `json:"wins"`
	Losses           int       `json:"losses"`
	Draws            int       `json:"draws"`
	VictoryPoints    int64     `json:"victory_points"`
	BronzeMedals     int       `json:"bronze_medals"`
	SilverMedals     int       `json:"silver_medals"`
	GoldMedals       int       `json:"gold_medals"`
	Trophies         int       `json:"trophies"`
	ChampionshipCups int       `json:"championship_cups"`
	ChampionCups     int       `json:"champion_cups"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// PromoteMedals checks if medals reach 5 and promotes to next medal tier recursively.
func (s *GvGStanding) PromoteMedals() {
	for s.BronzeMedals >= 5 {
		s.BronzeMedals -= 5
		s.SilverMedals++
	}
	for s.SilverMedals >= 5 {
		s.SilverMedals -= 5
		s.GoldMedals++
	}
	for s.GoldMedals >= 5 {
		s.GoldMedals -= 5
		s.Trophies++
	}
	for s.Trophies >= 5 {
		s.Trophies -= 5
		s.ChampionshipCups++
	}
	for s.ChampionshipCups >= 5 {
		s.ChampionshipCups -= 5
		s.ChampionCups++
	}
}

type GuildCandidate struct {
	GuildID       string `json:"guild_id"`
	GuildName     string `json:"guild_name"`
	LeaderName    string `json:"leader_name"`
	Level         int    `json:"level"`
	MemberCount   int    `json:"member_count"`
	Rating        int    `json:"rating"`
	VictoryPoints int64  `json:"victory_points"`
	Wins          int    `json:"wins"`
	Losses        int    `json:"losses"`
}

type MatchRound struct {
	ID                      string    `json:"id"`
	MatchID                 string    `json:"match_id"`
	RoundIndex              int       `json:"round_index"`
	ChallengerCharacterID   string    `json:"challenger_character_id"`
	ChallengerCharacterName string    `json:"challenger_character_name"`
	DefenderCharacterID     string    `json:"defender_character_id"`
	DefenderCharacterName   string    `json:"defender_character_name"`
	WinnerCharacterID       string    `json:"winner_character_id,omitempty"`
	Turns                   int       `json:"turns"`
	CreatedAt               time.Time `json:"created_at"`
}

type MatchRecord struct {
	ID                     string       `json:"id"`
	ChallengerGuildID      string       `json:"challenger_guild_id"`
	ChallengerGuildName    string       `json:"challenger_guild_name,omitempty"`
	DefenderGuildID        string       `json:"defender_guild_id"`
	DefenderGuildName      string       `json:"defender_guild_name,omitempty"`
	WinnerGuildID          string       `json:"winner_guild_id,omitempty"`
	ChallengerScore        int          `json:"challenger_score"`
	DefenderScore          int          `json:"defender_score"`
	TotalRounds            int          `json:"total_rounds"`
	ChallengerRatingBefore int          `json:"challenger_rating_before"`
	ChallengerRatingAfter  int          `json:"challenger_rating_after"`
	DefenderRatingBefore   int          `json:"defender_rating_before"`
	DefenderRatingAfter    int          `json:"defender_rating_after"`
	Rounds                 []MatchRound `json:"rounds,omitempty"`
	CreatedAt              time.Time    `json:"created_at"`
}

type DeclareResult struct {
	Match                   MatchRecord `json:"match"`
	ChallengerRatingDelta   int         `json:"challenger_rating_delta"`
	DefenderRatingDelta     int         `json:"defender_rating_delta"`
	ChallengerGuildExp      int64       `json:"challenger_guild_exp"`
	DefenderGuildExp        int64       `json:"defender_guild_exp"`
	ChallengerVictoryPoints int64       `json:"challenger_victory_points"`
	DefenderVictoryPoints   int64       `json:"defender_victory_points"`
	ChallengerMedalAwarded  bool        `json:"challenger_medal_awarded"`
	DefenderMedalAwarded    bool        `json:"defender_medal_awarded"`
}

// CalculateEloDelta computes the rating change for challenger and defender.
func CalculateEloDelta(challengerRating, defenderRating int, challengerScore, defenderScore int) (int, int) {
	expChallenger := 1.0 / (1.0 + math.Pow(10, float64(defenderRating-challengerRating)/400.0))

	var actualChallenger float64
	if challengerScore > defenderScore {
		actualChallenger = 1.0
	} else if challengerScore < defenderScore {
		actualChallenger = 0.0
	} else {
		actualChallenger = 0.5
	}

	deltaChallenger := int(math.Round(float64(KFactor) * (actualChallenger - expChallenger)))
	if challengerScore > defenderScore && deltaChallenger < 1 {
		deltaChallenger = 1
	} else if challengerScore < defenderScore && deltaChallenger > -1 {
		deltaChallenger = -1
	}

	deltaDefender := -deltaChallenger
	return deltaChallenger, deltaDefender
}

// CalculateGuildRewards computes Guild EXP, Victory Points, and Medals based on match score.
func CalculateGuildRewards(challengerScore, defenderScore int) (challengerExp, defenderExp int64, challengerVP, defenderVP int64, challengerMedal, defenderMedal bool) {
	if challengerScore > defenderScore {
		// Challenger wins
		return 100, 20, 10, 1, true, false
	} else if challengerScore < defenderScore {
		// Defender wins
		return 20, 100, 1, 10, false, true
	}
	// Draw
	return 50, 50, 3, 3, false, false
}

// CalculateMemberRewards returns EXP and Gold for an individual round participant.
func CalculateMemberRewards(isWinner bool, isDraw bool) (exp int64, gold int64) {
	if isWinner {
		return 50, 100
	}
	if isDraw {
		return 25, 50
	}
	return 15, 20
}

type MemberReward struct {
	Experience int64 `json:"experience"`
	Gold       int64 `json:"gold"`
}

type Repository interface {
	GetOrCreateStanding(ctx context.Context, guildID string) (GvGStanding, error)
	FindOpponentGuilds(ctx context.Context, challengerGuildID string, limit int) ([]GuildCandidate, error)
	GetLeaderboard(ctx context.Context, limit int) ([]GvGStanding, error)
	GetMatchHistory(ctx context.Context, guildID string, limit int) ([]MatchRecord, error)
	GetMatchDetail(ctx context.Context, matchID string) (MatchRecord, error)
	RecordMatchAndUpdateStandings(
		ctx context.Context,
		match MatchRecord,
		challengerDelta, defenderDelta int,
		challengerExp, defenderExp int64,
		challengerVP, defenderVP int64,
		challengerMedal, defenderMedal bool,
		memberRewards map[string]MemberReward,
	) error
}

type GuildRepository interface {
	GetGuild(ctx context.Context, guildID string) (guild.Guild, []guild.Member, error)
	GetGuildByCharacter(ctx context.Context, characterID string) (guild.Guild, guild.Member, error)
}

type CharacterRepository interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
}

type Service struct {
	repo          Repository
	guildRepo     GuildRepository
	characterRepo CharacterRepository
	battleEngine  corebattle.Resolver
}

func NewService(
	repo Repository,
	guildRepo GuildRepository,
	characterRepo CharacterRepository,
	battleEngine corebattle.Resolver,
) (*Service, error) {
	if repo == nil {
		return nil, errors.New("gvg repository is required")
	}
	if guildRepo == nil {
		return nil, errors.New("guild repository is required")
	}
	if characterRepo == nil {
		return nil, errors.New("character repository is required")
	}
	if battleEngine == nil {
		return nil, errors.New("battle engine is required")
	}
	return &Service{
		repo:          repo,
		guildRepo:     guildRepo,
		characterRepo: characterRepo,
		battleEngine:  battleEngine,
	}, nil
}

func (s *Service) GetStanding(ctx context.Context, guildID string) (GvGStanding, error) {
	if guildID == "" {
		return GvGStanding{}, ErrInvalidGuildID
	}
	return s.repo.GetOrCreateStanding(ctx, guildID)
}

func (s *Service) FindOpponentGuilds(ctx context.Context, challengerGuildID string, limit int) ([]GuildCandidate, error) {
	if challengerGuildID == "" {
		return nil, ErrInvalidGuildID
	}
	if limit <= 0 {
		limit = 10
	}
	return s.repo.FindOpponentGuilds(ctx, challengerGuildID, limit)
}

func (s *Service) GetLeaderboard(ctx context.Context, limit int) ([]GvGStanding, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.repo.GetLeaderboard(ctx, limit)
}

func (s *Service) GetMatchHistory(ctx context.Context, guildID string, limit int) ([]MatchRecord, error) {
	if guildID == "" {
		return nil, ErrInvalidGuildID
	}
	if limit <= 0 {
		limit = 20
	}
	return s.repo.GetMatchHistory(ctx, guildID, limit)
}

func (s *Service) GetMatchDetail(ctx context.Context, matchID string) (MatchRecord, error) {
	if matchID == "" {
		return MatchRecord{}, ErrMatchNotFound
	}
	return s.repo.GetMatchDetail(ctx, matchID)
}

func (s *Service) DeclareMatch(ctx context.Context, actorCharacterID, defenderGuildID string) (DeclareResult, error) {
	if actorCharacterID == "" {
		return DeclareResult{}, ErrCharacterNotFound
	}
	if defenderGuildID == "" {
		return DeclareResult{}, ErrInvalidGuildID
	}

	// 1. Check actor guild membership and authority
	challengerGuild, challengerMember, err := s.guildRepo.GetGuildByCharacter(ctx, actorCharacterID)
	if err != nil {
		return DeclareResult{}, ErrActorNotInGuild
	}
	challengerGuildID := challengerGuild.ID
	if challengerGuildID == defenderGuildID {
		return DeclareResult{}, ErrCannotChallengeOwnGuild
	}

	if challengerMember.Role != guild.RoleLeader && challengerMember.Role != guild.RoleOfficer {
		return DeclareResult{}, ErrUnauthorized
	}

	// 2. Load challenger and defender member rosters
	_, challengerMembers, err := s.guildRepo.GetGuild(ctx, challengerGuildID)
	if err != nil {
		return DeclareResult{}, fmt.Errorf("load challenger guild: %w", err)
	}

	defenderGuild, defenderMembers, err := s.guildRepo.GetGuild(ctx, defenderGuildID)
	if err != nil {
		return DeclareResult{}, ErrGuildNotFound
	}

	// 3. Assemble rosters (up to MaxRosterSize members each)
	if len(challengerMembers) > MaxRosterSize {
		challengerMembers = challengerMembers[:MaxRosterSize]
	}
	if len(defenderMembers) > MaxRosterSize {
		defenderMembers = defenderMembers[:MaxRosterSize]
	}

	if len(challengerMembers) == 0 || len(defenderMembers) == 0 {
		return DeclareResult{}, ErrInsufficientRoster
	}

	// 4. Resolve sequential member matchups
	numRounds := len(challengerMembers)
	if len(defenderMembers) < numRounds {
		numRounds = len(defenderMembers)
	}

	matchID := generateMatchID()
	rounds := make([]MatchRound, 0, numRounds)
	challengerScore := 0
	defenderScore := 0
	memberRewards := make(map[string]MemberReward)

	for i := 0; i < numRounds; i++ {
		cMem := challengerMembers[i]
		dMem := defenderMembers[i]

		cChar, err := s.characterRepo.FindByID(ctx, cMem.CharacterID)
		if err != nil {
			return DeclareResult{}, fmt.Errorf("failed to load challenger character %s: %w", cMem.CharacterID, err)
		}
		dChar, err := s.characterRepo.FindByID(ctx, dMem.CharacterID)
		if err != nil {
			return DeclareResult{}, fmt.Errorf("failed to load defender character %s: %w", dMem.CharacterID, err)
		}

		req := corebattle.Request{
			Participants: []corebattle.Participant{
				corebattle.NewParticipantFromCharacter(cChar),
				corebattle.NewParticipantFromCharacter(dChar),
			},
		}

		res, err := s.battleEngine.Resolve(req)
		if err != nil {
			return DeclareResult{}, fmt.Errorf("battle resolution failed for round %d: %w", i+1, err)
		}

		round := MatchRound{
			ID:                      fmt.Sprintf("%s_r%d", matchID, i+1),
			MatchID:                 matchID,
			RoundIndex:              i + 1,
			ChallengerCharacterID:   cChar.ID,
			ChallengerCharacterName: cChar.Name,
			DefenderCharacterID:     dChar.ID,
			DefenderCharacterName:   dChar.Name,
			Turns:                   res.Turns,
			CreatedAt:               time.Now(),
		}

		var cExp, cGold, dExp, dGold int64
		if res.Outcome == corebattle.OutcomeWin && res.WinnerID == cChar.ID {
			challengerScore++
			round.WinnerCharacterID = cChar.ID
			cExp, cGold = CalculateMemberRewards(true, false)
			dExp, dGold = CalculateMemberRewards(false, false)
		} else if res.Outcome == corebattle.OutcomeWin && res.WinnerID == dChar.ID {
			defenderScore++
			round.WinnerCharacterID = dChar.ID
			cExp, cGold = CalculateMemberRewards(false, false)
			dExp, dGold = CalculateMemberRewards(true, false)
		} else {
			// Draw
			cExp, cGold = CalculateMemberRewards(false, true)
			dExp, dGold = CalculateMemberRewards(false, true)
		}

		memberRewards[cChar.ID] = MemberReward{Experience: cExp, Gold: cGold}
		memberRewards[dChar.ID] = MemberReward{Experience: dExp, Gold: dGold}
		rounds = append(rounds, round)
	}

	// 5. Compute Ratings and Standings
	challengerStanding, err := s.repo.GetOrCreateStanding(ctx, challengerGuildID)
	if err != nil {
		return DeclareResult{}, err
	}
	defenderStanding, err := s.repo.GetOrCreateStanding(ctx, defenderGuildID)
	if err != nil {
		return DeclareResult{}, err
	}

	cDelta, dDelta := CalculateEloDelta(challengerStanding.Rating, defenderStanding.Rating, challengerScore, defenderScore)
	cExp, dExp, cVP, dVP, cMedal, dMedal := CalculateGuildRewards(challengerScore, defenderScore)

	var winnerGuildID string
	if challengerScore > defenderScore {
		winnerGuildID = challengerGuildID
	} else if defenderScore > challengerScore {
		winnerGuildID = defenderGuildID
	}

	matchRecord := MatchRecord{
		ID:                     matchID,
		ChallengerGuildID:      challengerGuildID,
		ChallengerGuildName:    challengerGuild.Name,
		DefenderGuildID:        defenderGuildID,
		DefenderGuildName:      defenderGuild.Name,
		WinnerGuildID:          winnerGuildID,
		ChallengerScore:        challengerScore,
		DefenderScore:          defenderScore,
		TotalRounds:            numRounds,
		ChallengerRatingBefore: challengerStanding.Rating,
		ChallengerRatingAfter:  max(0, challengerStanding.Rating+cDelta),
		DefenderRatingBefore:   defenderStanding.Rating,
		DefenderRatingAfter:    max(0, defenderStanding.Rating+dDelta),
		Rounds:                 rounds,
		CreatedAt:              time.Now(),
	}

	// 6. Record match and update standings atomically
	if err := s.repo.RecordMatchAndUpdateStandings(
		ctx,
		matchRecord,
		cDelta, dDelta,
		cExp, dExp,
		cVP, dVP,
		cMedal, dMedal,
		memberRewards,
	); err != nil {
		return DeclareResult{}, err
	}

	return DeclareResult{
		Match:                   matchRecord,
		ChallengerRatingDelta:   cDelta,
		DefenderRatingDelta:     dDelta,
		ChallengerGuildExp:      cExp,
		DefenderGuildExp:        dExp,
		ChallengerVictoryPoints: cVP,
		DefenderVictoryPoints:   dVP,
		ChallengerMedalAwarded:  cMedal,
		DefenderMedalAwarded:    dMedal,
	}, nil
}

func generateMatchID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("gvg_%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
