package pvp

import (
	"errors"
	"strings"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

// Legacy team colors (%colors in party2/lib/vs_player.cgi).
const (
	ColorRed    = "#FF3333" // レッド
	ColorPink   = "#FF33CC" // ピンク
	ColorOrange = "#FF9933" // オレンジ
	ColorYellow = "#FFFF33" // イエロー
	ColorGreen  = "#33FF33" // グリーン
	ColorAqua   = "#33CCFF" // アクア
	ColorBlue   = "#6666FF" // ブルー
	ColorPurple = "#CC66FF" // パープル
	ColorGray   = "#CCCCCC" // グレイ
)

// ValidTeamColors maps hexadecimal team colors to their authentic Japanese names.
var ValidTeamColors = map[string]string{
	ColorRed:    "レッド",
	ColorPink:   "ピンク",
	ColorOrange: "オレンジ",
	ColorYellow: "イエロー",
	ColorGreen:  "グリーン",
	ColorAqua:   "アクア",
	ColorBlue:   "ブルー",
	ColorPurple: "パープル",
	ColorGray:   "グレイ",
}

// IsValidTeamColor checks if a color is one of the 9 authentic team colors.
func IsValidTeamColor(color string) bool {
	_, ok := ValidTeamColors[strings.ToUpper(color)]
	return ok
}

// TeamColorName returns the Japanese name for a team color.
func TeamColorName(color string) string {
	if name, ok := ValidTeamColors[strings.ToUpper(color)]; ok {
		return name
	}
	return color
}

// Colosseum Room Status constants.
const (
	StatusRecruiting = "recruiting"
	StatusInProgress = "in_progress"
	StatusCompleted  = "completed"
	StatusDisbanded  = "disbanded"

	DefaultSpeed    = 18
	MinMembers      = 2
	MaxMembers      = 8
	MinBet          = 10
	MinTargetWins   = 1
	MaxTargetWins   = 3
	MaxRounds       = 10
	DefaultLobbyTTL = 30 * time.Minute
)

// Domain Errors.
var (
	ErrRoomNotFound          = errors.New("colosseum room not found")
	ErrRoomFull              = errors.New("colosseum room is already full")
	ErrRoomNotRecruiting     = errors.New("colosseum room is not currently recruiting")
	ErrAlreadyInRoom         = errors.New("character is already in an active colosseum room")
	ErrCharacterNotInRoom    = errors.New("character is not in this colosseum room")
	ErrNotRoomLeader         = errors.New("only room leader can perform this action")
	ErrInsufficientBetFunds  = errors.New("insufficient money to pay colosseum bet")
	ErrInvalidBet            = errors.New("bet must be at least 10 gold")
	ErrInvalidMaxMembers     = errors.New("max members must be between 2 and 8")
	ErrInvalidTargetWins     = errors.New("target wins must be between 1 and 3")
	ErrInvalidPassword       = errors.New("invalid colosseum room password")
	ErrNeedJoinNotMet        = errors.New("character does not meet room join condition")
	ErrCharacterUnconscious  = errors.New("character is unconscious (HP <= 0)")
	ErrCharacterExhausted    = errors.New("character is exhausted (tired >= 100)")
	ErrInvalidTeamColor      = errors.New("invalid team color, must be one of the 9 colosseum colors")
	ErrTeamsNotConfigured    = errors.New("all participants must select a team color before starting")
	ErrNeedAtLeastTwoTeams   = errors.New("at least 2 distinct team colors are required to start")
	ErrNotEnoughParticipants = errors.New("at least 2 participants are required to start")
	ErrMatchAlreadyStarted   = errors.New("match has already started")
	ErrMatchNotInProgress    = errors.New("match is not in progress")
	ErrMatchCompleted        = errors.New("match is already completed")
	ErrInvalidDependencies   = errors.New("pvp dependencies cannot be nil")
	ErrInvalidRoomName       = errors.New("room name must be between 1 and 50 characters")
)

// ColosseumRoom represents a real-time multiplayer 8-player colosseum room (quest.cgi:type=4, vs_player.cgi).
type ColosseumRoom struct {
	ID                string         `json:"id"`
	Name              string         `json:"name"`
	LeaderCharacterID string         `json:"leader_character_id"`
	LeaderName        string         `json:"leader_name"`
	Speed             int            `json:"speed"`
	Stage             int            `json:"stage"`
	MaxMembers        int            `json:"max_members"`
	TargetWins        int            `json:"target_wins"`
	Bet               int            `json:"bet"`
	PrizePool         int            `json:"prize_pool"`
	PasswordHash      string         `json:"-"`
	NeedJoin          string         `json:"need_join,omitempty"`
	Status            string         `json:"status"`
	Round             int            `json:"round"`
	TeamScores        map[string]int `json:"team_scores"`
	WinnerTeam        string         `json:"winner_team,omitempty"`
	PrizePerMember    int            `json:"prize_per_member,omitempty"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
}

// RoomMember represents a participant in a colosseum room.
type RoomMember struct {
	RoomID        string    `json:"room_id"`
	CharacterID   string    `json:"character_id"`
	CharacterName string    `json:"character_name"`
	JobID         string    `json:"job_id"`
	Level         int       `json:"level"`
	HP            int       `json:"hp"`
	MaxHP         int       `json:"max_hp"`
	TeamColor     string    `json:"team_color"`
	IsLeader      bool      `json:"is_leader"`
	ReadyState    bool      `json:"ready_state"`
	JoinedAt      time.Time `json:"joined_at"`
}

// RoomDetail contains composite room data and members.
type RoomDetail struct {
	Room    ColosseumRoom `json:"room"`
	Members []RoomMember  `json:"members"`
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
	Bet               int       `json:"bet"`
	PrizePool         int       `json:"prize_pool"`
	TargetWins        int       `json:"target_wins"`
	HasPassword       bool      `json:"has_password"`
	Status            string    `json:"status"`
	Round             int       `json:"round"`
	CreatedAt         time.Time `json:"created_at"`
}

// RoundResolution represents the outcome of a battle round.
type RoundResolution struct {
	Round               int                  `json:"round"`
	Outcome             string               `json:"outcome"` // "round_win", "draw", "match_won", "match_draw"
	WinnerTeam          string               `json:"winner_team,omitempty"`
	WinnerTeamName      string               `json:"winner_team_name,omitempty"`
	TeamScores          map[string]int       `json:"team_scores"`
	MatchCompleted      bool                 `json:"match_completed"`
	OverallWinnerTeam   string               `json:"overall_winner_team,omitempty"`
	PrizePerMember      int                  `json:"prize_per_member,omitempty"`
	AwardedCharacterIDs []string             `json:"awarded_character_ids,omitempty"`
	Turns               int                  `json:"turns"`
	BattleLog           []corebattle.TurnLog `json:"battle_log,omitempty"`
}

// CreateRoomRequest specifies room initialization arguments.
type CreateRoomRequest struct {
	Name       string `json:"name"`
	Password   string `json:"password,omitempty"`
	Speed      int    `json:"speed,omitempty"`
	Stage      int    `json:"stage,omitempty"`
	MaxMembers int    `json:"max_members,omitempty"`
	TargetWins int    `json:"target_wins,omitempty"`
	Bet        int    `json:"bet"`
	NeedJoin   string `json:"need_join,omitempty"`
}
