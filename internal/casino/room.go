package casino

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/witchcraze/party2re/internal/id"
)

type GameType string

const (
	GameTypeIndian  GameType = "indian"
	GameTypeHighLow GameType = "highlow"
	GameTypeDoppel  GameType = "doppel"
)

func (g GameType) Valid() bool {
	switch g {
	case GameTypeIndian, GameTypeHighLow, GameTypeDoppel:
		return true
	default:
		return false
	}
}

type RoomStatus string

const (
	RoomStatusWaiting    RoomStatus = "waiting"
	RoomStatusInProgress RoomStatus = "in_progress"
	RoomStatusDisbanded  RoomStatus = "disbanded"
)

type RoomSpeed int

const (
	SpeedFast   RoomSpeed = 12 // さくさく
	SpeedNormal RoomSpeed = 18 // まったり
	SpeedSlow   RoomSpeed = 28 // じっくり
)

var ValidSpeeds = map[RoomSpeed]bool{
	SpeedFast:   true,
	SpeedNormal: true,
	SpeedSlow:   true,
}

var ValidRates = map[int64]bool{
	1: true, 5: true, 10: true, 20: true, 50: true, 100: true, 500: true, 1000: true, 5000: true,
}

var AuthenticCardNames = [13]string{
	"Ａ", "２", "３", "４", "５", "６", "７", "８", "９", "10", "Ｊ", "Ｑ", "Ｋ",
}

const (
	MaxRoomNameLen = 50
	MinPartyCount  = 2
	MaxPartyCount  = 8
	MinDoppelBet   = 10
)

var (
	ErrRoomNotFound             = errors.New("casino room not found")
	ErrRoomNameTaken            = errors.New("room name already exists")
	ErrInvalidRoomName          = errors.New("invalid room name: must be 1-50 chars without illegal characters")
	ErrInvalidGameType          = errors.New("invalid casino game type")
	ErrInvalidSpeed             = errors.New("invalid speed: must be 12, 18, or 28")
	ErrInvalidMaxPlayers        = errors.New("invalid max players: must be between 2 and 8")
	ErrInvalidRate              = errors.New("invalid rate: must be 1, 5, 10, 20, 50, 100, 500, 1000, or 5000")
	ErrInsufficientCoinsForRate = errors.New("insufficient coins for rate: creator needs rate * 5 coins")
	ErrCharacterExhausted       = errors.New("character is exhausted (tired >= 100) and cannot play")
	ErrRoomFull                 = errors.New("casino room is full")
	ErrAlreadyInRoom            = errors.New("character is already in this room")
	ErrInvalidPassword          = errors.New("invalid room password")
	ErrSpectatorsNotAllowed     = errors.New("spectators are not allowed in this room")
	ErrNotLeader                = errors.New("only room leader can perform this action")
	ErrCannotKickSelf           = errors.New("leader cannot kick self")
	ErrGameInProgress           = errors.New("cannot perform action while game is in progress")
	ErrMemberNotFound           = errors.New("character is not a member of this room")
	ErrNotEnoughPlayers         = errors.New("at least 2 participants required to start game")
)

var illegalNameChars = regexp.MustCompile(`[,;\"\'&<>\\\/@]`)

func ValidateRoomName(name string) error {
	name = strings.TrimSpace(name)
	l := utf8.RuneCountInString(name)
	if l < 1 || l > MaxRoomNameLen {
		return ErrInvalidRoomName
	}
	if strings.ContainsAny(name, " \t\n\r　") {
		return ErrInvalidRoomName
	}
	if illegalNameChars.MatchString(name) || strings.Contains(name, "＠") {
		return ErrInvalidRoomName
	}
	return nil
}

type Room struct {
	ID                string     `json:"id"`
	Name              string     `json:"name"`
	GameType          GameType   `json:"game_type"`
	LeaderCharacterID string     `json:"leader_character_id"`
	Speed             RoomSpeed  `json:"speed"`
	MaxPlayers        int        `json:"max_players"`
	Rate              int64      `json:"rate"`
	PasswordHash      string     `json:"-"`
	HasPassword       bool       `json:"has_password"`
	AllowSpectators   bool       `json:"allow_spectators"`
	Status            RoomStatus `json:"status"`
	Round             int        `json:"round"`
	CurrentBet        int64      `json:"current_bet"`
	MaxBet            int64      `json:"max_bet"`
	Pot               int64      `json:"pot"`
	WinnerCharacterID *string    `json:"winner_character_id,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

type RoomMember struct {
	RoomID        string    `json:"room_id"`
	CharacterID   string    `json:"character_id"`
	CharacterName string    `json:"character_name,omitempty"`
	IsSpectator   bool      `json:"is_spectator"`
	Action        string    `json:"action"` // '', 'しょうぶ', 'つづける', 'おりる', '待機中'
	Card          int       `json:"card"`   // -1 or 0..12
	CardDisplay   string    `json:"card_display,omitempty"`
	JoinedAt      time.Time `json:"joined_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type RoomDetail struct {
	Room    Room         `json:"room"`
	Members []RoomMember `json:"members"`
}

type CreateRoomRequest struct {
	Name            string    `json:"name"`
	GameType        GameType  `json:"game_type"`
	Speed           RoomSpeed `json:"speed"`
	MaxPlayers      int       `json:"max_players"`
	Rate            int64     `json:"rate"`
	Password        string    `json:"password,omitempty"`
	AllowSpectators bool      `json:"allow_spectators"`
}

type RoomLifecycleRepository interface {
	CreateRoom(ctx context.Context, room Room, leaderMember RoomMember) error
	GetRoom(ctx context.Context, roomID string) (*Room, error)
	GetRoomByName(ctx context.Context, name string) (*Room, error)
	GetRoomForUpdate(ctx context.Context, roomID string) (*Room, error)
	ListActiveRooms(ctx context.Context) ([]RoomDetail, error)
	UpdateRoom(ctx context.Context, room Room) error
	DeleteRoom(ctx context.Context, roomID string) error
}

type RoomMemberRepository interface {
	AddMember(ctx context.Context, member RoomMember) error
	GetMember(ctx context.Context, roomID string, characterID string) (*RoomMember, error)
	GetMemberForUpdate(ctx context.Context, roomID string, characterID string) (*RoomMember, error)
	ListMembers(ctx context.Context, roomID string) ([]RoomMember, error)
	ListMembersForUpdate(ctx context.Context, roomID string) ([]RoomMember, error)
	UpdateMember(ctx context.Context, member RoomMember) error
	RemoveMember(ctx context.Context, roomID string, characterID string) error
}

type RoomRepository interface {
	RoomLifecycleRepository
	RoomMemberRepository
}

func hashPassword(pass string) string {
	if pass == "" {
		return ""
	}
	h := sha256.Sum256([]byte(pass))
	return hex.EncodeToString(h[:])
}

func verifyPassword(pass, hash string) bool {
	if hash == "" {
		return true
	}
	return hashPassword(pass) == hash
}

// CreateRoom creates a new casino multiplayer room and adds creator as leader (party2/lib/casino.cgi:335-377).
func (s *Service) CreateRoom(ctx context.Context, characterID string, req CreateRoomRequest) (*RoomDetail, error) {
	if characterID == "" {
		return nil, ErrInvalidCharacterID
	}
	if err := ValidateRoomName(req.Name); err != nil {
		return nil, err
	}
	if !req.GameType.Valid() {
		return nil, ErrInvalidGameType
	}
	if !ValidSpeeds[req.Speed] {
		return nil, ErrInvalidSpeed
	}
	if req.MaxPlayers < MinPartyCount || req.MaxPlayers > MaxPartyCount {
		return nil, ErrInvalidMaxPlayers
	}
	if s.roomRepo == nil {
		return nil, errors.New("room repository is required")
	}

	if req.GameType == GameTypeDoppel {
		if req.Rate < MinDoppelBet {
			return nil, ErrInvalidRate
		}
	} else {
		if !ValidRates[req.Rate] {
			return nil, ErrInvalidRate
		}
	}

	var detail *RoomDetail
	err := s.runInTx(ctx, func(txCtx context.Context) error {
		existing, err := s.roomRepo.GetRoomByName(txCtx, req.Name)
		if err != nil && !errors.Is(err, ErrRoomNotFound) {
			return err
		}
		if existing != nil && existing.Status != RoomStatusDisbanded {
			return ErrRoomNameTaken
		}

		acc, err := s.repo.GetAccount(txCtx, characterID)
		if err != nil {
			return err
		}
		requiredCoins := req.Rate * 5
		if req.GameType == GameTypeDoppel {
			requiredCoins = req.Rate
		}
		if acc.Coins < requiredCoins {
			return ErrInsufficientCoinsForRate
		}

		now := time.Now().UTC()
		roomID := id.New()
		r := Room{
			ID:                roomID,
			Name:              req.Name,
			GameType:          req.GameType,
			LeaderCharacterID: characterID,
			Speed:             req.Speed,
			MaxPlayers:        req.MaxPlayers,
			Rate:              req.Rate,
			PasswordHash:      hashPassword(req.Password),
			HasPassword:       req.Password != "",
			AllowSpectators:   req.AllowSpectators,
			Status:            RoomStatusWaiting,
			Round:             0,
			CurrentBet:        req.Rate,
			MaxBet:            req.Rate * 5,
			Pot:               0,
			CreatedAt:         now,
			UpdatedAt:         now,
		}

		leader := RoomMember{
			RoomID:      roomID,
			CharacterID: characterID,
			IsSpectator: false,
			Action:      "待機中",
			Card:        -1,
			JoinedAt:    now,
			UpdatedAt:   now,
		}

		if err := s.roomRepo.CreateRoom(txCtx, r, leader); err != nil {
			return err
		}

		detail = &RoomDetail{
			Room:    r,
			Members: []RoomMember{leader},
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return detail, nil
}

// ListRooms returns all active rooms.
func (s *Service) ListRooms(ctx context.Context) ([]RoomDetail, error) {
	if s.roomRepo == nil {
		return nil, errors.New("room repository is required")
	}
	return s.roomRepo.ListActiveRooms(ctx)
}

// GetRoomDetail returns room details with appropriate card masking according to Party2 rules.
func (s *Service) GetRoomDetail(ctx context.Context, roomID string, viewingCharID string) (*RoomDetail, error) {
	if s.roomRepo == nil {
		return nil, errors.New("room repository is required")
	}
	room, err := s.roomRepo.GetRoom(ctx, roomID)
	if err != nil {
		return nil, err
	}
	members, err := s.roomRepo.ListMembers(ctx, roomID)
	if err != nil {
		return nil, err
	}

	maskedMembers := make([]RoomMember, len(members))
	for i, m := range members {
		masked := m
		if m.Card >= 0 {
			if room.GameType == GameTypeDoppel && m.Card < len(AuthenticDoppelMarks) {
				masked.CardDisplay = string(AuthenticDoppelMarks[m.Card])
			} else if m.Card < len(AuthenticCardNames) {
				masked.CardDisplay = AuthenticCardNames[m.Card]
			}
		}

		if room.Round > 0 && viewingCharID != "" {
			switch room.GameType {
			case GameTypeIndian:
				// Forehead card rule (party2/lib/casino_indian.cgi:28):
				// A player cannot see their own card while active in an in-progress round, but sees others.
				if m.CharacterID == viewingCharID && m.Action != string(ActionFold) && m.Action != "おりる" && m.Action != "待機中" {
					masked.Card = -1
					masked.CardDisplay = "？"
				}
			case GameTypeHighLow:
				// High-Low rule (party2/lib/casino_highlow.cgi:33-50):
				// Player sees own card and own action; others' cards are hidden, and other actions (high/low/fold) masked as "？？？"
				if m.CharacterID != viewingCharID {
					masked.Card = -1
					masked.CardDisplay = "？"
					if m.Action != "" && m.Action != "待機中" && m.Action != "つづける" && m.Action != string(HighLowActionCall) {
						masked.Action = "？？？"
					}
				}
			case GameTypeDoppel:
				// Doppel rule (party2/lib/casino_doppel.cgi:23-36):
				// Player sees own chosen mark; others' marks and actions are hidden
				if m.CharacterID != viewingCharID {
					masked.Card = -1
					masked.CardDisplay = "？"
					if m.Action != "" && m.Action != "待機中" {
						masked.Action = "？？？"
					}
				}
			}
		}
		maskedMembers[i] = masked
	}

	return &RoomDetail{
		Room:    *room,
		Members: maskedMembers,
	}, nil
}

// StartGame starts the round for the room's configured GameType (party2/lib/_casino.cgi:24-32).
func (s *Service) StartGame(ctx context.Context, roomID string, leaderID string) (*RoomDetail, error) {
	if s.roomRepo == nil {
		return nil, errors.New("room repository is required")
	}
	room, err := s.roomRepo.GetRoom(ctx, roomID)
	if err != nil {
		return nil, err
	}
	switch room.GameType {
	case GameTypeIndian:
		return s.StartIndianPoker(ctx, roomID, leaderID)
	case GameTypeHighLow:
		return s.StartHighLow(ctx, roomID, leaderID)
	case GameTypeDoppel:
		return s.StartDoppel(ctx, roomID, leaderID)
	default:
		return nil, ErrInvalidGameType
	}
}

type RoomActionRequest struct {
	Action string `json:"action"`         // "call", "high", "low", "fold", "showdown"
	Mark   string `json:"mark,omitempty"` // For doppel: "★".."▼" or "0".."7"
}

// PlayRoomAction routes a player's action according to the room's GameType.
func (s *Service) PlayRoomAction(ctx context.Context, roomID string, characterID string, req RoomActionRequest) (*RoomDetail, error) {
	if s.roomRepo == nil {
		return nil, errors.New("room repository is required")
	}
	room, err := s.roomRepo.GetRoom(ctx, roomID)
	if err != nil {
		return nil, err
	}
	switch room.GameType {
	case GameTypeIndian:
		act := Action(req.Action)
		if !act.Valid() {
			return nil, ErrInvalidAction
		}
		return s.PlayIndianPokerAction(ctx, roomID, characterID, act)
	case GameTypeHighLow:
		act, err := ParseHighLowAction(req.Action)
		if err != nil {
			return nil, err
		}
		return s.PlayHighLowAction(ctx, roomID, characterID, act)
	case GameTypeDoppel:
		input := req.Mark
		if input == "" {
			input = req.Action
		}
		markIdx, err := ParseDoppelMark(input)
		if err != nil {
			return nil, err
		}
		return s.PlayDoppelAction(ctx, roomID, characterID, markIdx)
	default:
		return nil, ErrInvalidGameType
	}
}
