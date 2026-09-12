package gvg

import (
	"errors"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

const (
	DefaultColor       = "#FFFFFF"
	InitialPrizeGP     = 2
	JoinPrizeGP        = 1
	RoundWinGP         = 3
	MatchParticipantGP = 4
	MinMembers         = 2
	MaxMembers         = 8
	MinTargetWins      = 1
	MaxTargetWins      = 3
	MaxRounds          = 10
	DefaultSpeed       = 18
	DefaultLobbyTTL    = 30 * time.Minute
	StatusRecruiting   = "recruiting"
	StatusInProgress   = "in_progress"
	StatusCompleted    = "completed"
	StatusDisbanded    = "disbanded"
)

var (
	ErrRoomNotFound              = errors.New("gvg room not found")
	ErrRoomFull                  = errors.New("gvg room is already full")
	ErrRoomNotRecruiting         = errors.New("gvg room is not currently recruiting")
	ErrAlreadyInRoom             = errors.New("character is already in an active gvg room")
	ErrCharacterNotInRoom        = errors.New("character is not in this gvg room")
	ErrNotRoomLeader             = errors.New("only room leader can perform this action")
	ErrInvalidMaxMembers         = errors.New("max members must be between 2 and 8")
	ErrInvalidTargetWins         = errors.New("target wins must be between 1 and 3")
	ErrInvalidPassword           = errors.New("invalid gvg room password")
	ErrNeedJoinNotMet            = errors.New("character does not meet room join condition")
	ErrCharacterUnconscious      = errors.New("character is unconscious (HP <= 0)")
	ErrCharacterExhausted        = errors.New("character is exhausted (tired >= 100)")
	ErrActorNotInGuild           = errors.New("character is not in a guild")
	ErrFriendlyGuildCannotBattle = errors.New("仲良しギルドはギルド戦をすることはできません")
	ErrNeedAtLeastTwoGuilds      = errors.New("対戦するギルドがいません")
	ErrNotEnoughParticipants     = errors.New("at least 2 participants are required to start")
	ErrMatchAlreadyStarted       = errors.New("match has already started")
	ErrMatchNotInProgress        = errors.New("match is not in progress")
	ErrMatchCompleted            = errors.New("match is already completed")
	ErrInvalidDependencies       = errors.New("gvg dependencies cannot be nil")
	ErrInvalidRoomName           = errors.New("room name must be between 1 and 50 characters")
	ErrCharacterNotFound         = errors.New("character not found")
	ErrGuildNotFound             = errors.New("guild not found")
	ErrInvalidGuildID            = errors.New("invalid guild ID")
)

// GvGStanding represents the persistent standings and trophy decorations of a guild.
type GvGStanding struct {
	GuildID          string    `json:"guild_id"`
	GuildName        string    `json:"guild_name,omitempty"`
	Wins             int       `json:"wins"`
	Losses           int       `json:"losses"`
	Draws            int       `json:"draws"`
	VictoryPoints    int64     `json:"victory_points"` // GP
	BronzeMedals     int       `json:"bronze_medals"`
	SilverMedals     int       `json:"silver_medals"`
	GoldMedals       int       `json:"gold_medals"`
	Orders           int       `json:"orders"`
	Trophies         int       `json:"trophies"`
	ChampionshipCups int       `json:"championship_cups"`
	ChampionCups     int       `json:"champion_cups"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// PromoteMedals checks if medals reach 5 and promotes to next medal tier recursively across all 7 tiers.
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
		s.Orders++
	}
	for s.Orders >= 5 {
		s.Orders -= 5
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

// GvGRoom represents a real-time multiplayer guild battle room (quest.cgi:type=5, vs_guild.cgi).
type GvGRoom struct {
	ID                string         `json:"id"`
	Name              string         `json:"name"`
	LeaderCharacterID string         `json:"leader_character_id"`
	LeaderName        string         `json:"leader_name"`
	Speed             int            `json:"speed"`
	Stage             int            `json:"stage"`
	MaxMembers        int            `json:"max_members"`
	TargetWins        int            `json:"target_wins"`
	PrizePool         int            `json:"prize_pool"` // GP accumulated in room
	PasswordHash      string         `json:"-"`
	NeedJoin          string         `json:"need_join,omitempty"`
	Status            string         `json:"status"`
	Round             int            `json:"round"`
	GuildScores       map[string]int `json:"guild_scores"` // Keyed by guild ID
	WinnerGuildID     string         `json:"winner_guild_id,omitempty"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
}

// GvGMember represents a participant in a guild battle room.
type GvGMember struct {
	RoomID        string    `json:"room_id"`
	CharacterID   string    `json:"character_id"`
	CharacterName string    `json:"character_name"`
	JobID         string    `json:"job_id"`
	Level         int       `json:"level"`
	HP            int       `json:"hp"`
	MaxHP         int       `json:"max_hp"`
	GuildID       string    `json:"guild_id"`
	GuildName     string    `json:"guild_name"`
	GuildColor    string    `json:"guild_color"`
	IsLeader      bool      `json:"is_leader"`
	JoinedAt      time.Time `json:"joined_at"`
}

// RoomDetail contains composite room data and members.
type RoomDetail struct {
	Room    GvGRoom     `json:"room"`
	Members []GvGMember `json:"members"`
}

// RoomSummary provides concise listing information.
type RoomSummary struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	LeaderCharacterID string    `json:"leader_character_id"`
	LeaderName        string    `json:"leader_name"`
	Speed             int       `json:"speed"`
	Stage             int       `json:"stage"`
	CurrentMembers    int       `json:"current_members"`
	MaxMembers        int       `json:"max_members"`
	PrizePool         int       `json:"prize_pool"`
	TargetWins        int       `json:"target_wins"`
	HasPassword       bool      `json:"has_password"`
	Status            string    `json:"status"`
	Round             int       `json:"round"`
	CreatedAt         time.Time `json:"created_at"`
}

// RoundResolution represents the outcome of a GvG battle round.
type RoundResolution struct {
	Round                  int                  `json:"round"`
	Outcome                string               `json:"outcome"` // "round_win", "draw", "match_won", "match_draw"
	WinnerGuildID          string               `json:"winner_guild_id,omitempty"`
	WinnerGuildName        string               `json:"winner_guild_name,omitempty"`
	WinnerGuildColor       string               `json:"winner_guild_color,omitempty"`
	GuildScores            map[string]int       `json:"guild_scores"`
	MatchCompleted         bool                 `json:"match_completed"`
	OverallWinnerGuildID   string               `json:"overall_winner_guild_id,omitempty"`
	OverallWinnerGuildName string               `json:"overall_winner_guild_name,omitempty"`
	PrizeGP                int                  `json:"prize_gp,omitempty"`
	Turns                  int                  `json:"turns"`
	BattleLog              []corebattle.TurnLog `json:"battle_log,omitempty"`
}

// CreateRoomRequest specifies room initialization arguments.
type CreateRoomRequest struct {
	Name       string `json:"name"`
	Password   string `json:"password,omitempty"`
	Speed      int    `json:"speed,omitempty"`
	Stage      int    `json:"stage,omitempty"`
	MaxMembers int    `json:"max_members,omitempty"`
	TargetWins int    `json:"target_wins,omitempty"`
	NeedJoin   string `json:"need_join,omitempty"`
}
