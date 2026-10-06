package casino

import (
	"context"
	"errors"
	"time"
)

var ErrRoomViewForbidden = errors.New("casino room viewer is not an owned admitted character")

// RoomSummary is the explicit public lobby whitelist. No member turn data is included.
type RoomSummary struct {
	ID               string     `json:"id"`
	Name             string     `json:"name"`
	GameType         GameType   `json:"game_type"`
	Speed            RoomSpeed  `json:"speed"`
	MaxPlayers       int        `json:"max_players"`
	Rate             int64      `json:"rate"`
	HasPassword      bool       `json:"has_password"`
	AllowSpectators  bool       `json:"allow_spectators"`
	Status           RoomStatus `json:"status"`
	ParticipantCount int        `json:"participant_count"`
	SpectatorCount   int        `json:"spectator_count"`
}

type RoomObservation struct {
	RoomSummary
	LeaderCharacterID string  `json:"leader_character_id"`
	Round             int     `json:"round"`
	CurrentBet        int64   `json:"current_bet"`
	MaxBet            int64   `json:"max_bet"`
	Pot               int64   `json:"pot"`
	WinnerCharacterID *string `json:"winner_character_id,omitempty"`
}

type MemberObservation struct {
	CharacterID   string `json:"character_id"`
	CharacterName string `json:"character_name,omitempty"`
	IsSpectator   bool   `json:"is_spectator"`
	Action        string `json:"action"`
	Card          int    `json:"card"`
	CardDisplay   string `json:"card_display,omitempty"`
}

// RoomView contains game-specific masked facts for an admitted viewer.
type RoomView struct {
	Room    RoomObservation     `json:"room"`
	Members []MemberObservation `json:"members"`
}

func summarizeRoom(detail RoomDetail) RoomSummary {
	r := detail.Room
	summary := RoomSummary{ID: r.ID, Name: r.Name, GameType: r.GameType, Speed: r.Speed, MaxPlayers: r.MaxPlayers, Rate: r.Rate, HasPassword: r.HasPassword, AllowSpectators: r.AllowSpectators, Status: r.Status}
	for _, m := range detail.Members {
		if m.IsSpectator {
			summary.SpectatorCount++
		} else {
			summary.ParticipantCount++
		}
	}
	return summary
}

// GetRoomView authorizes both ownership and live admission at the service boundary.
// It does not refresh room lifetime or progress gameplay. Expired rooms are absent.
func (s *Service) GetRoomView(ctx context.Context, roomID, playerID, characterID string) (*RoomView, error) {
	if playerID == "" || characterID == "" {
		return nil, ErrRoomViewForbidden
	}
	if s.charRepo == nil {
		return nil, errors.New("character repository is required")
	}
	char, err := s.charRepo.FindByID(ctx, characterID)
	if err != nil {
		return nil, err
	}
	if char.PlayerID != playerID {
		return nil, ErrRoomViewForbidden
	}
	var view *RoomView
	err = s.withRoomLock(ctx, roomID, func(lockedCtx context.Context) error {
		detail, err := s.getRoomDetail(lockedCtx, roomID, characterID)
		if err != nil {
			return err
		}
		if detail.Room.Status == RoomStatusDisbanded || detail.Room.UpdatedAt.Before(time.Now().UTC().Add(-IdleRoomTimeout)) {
			return ErrRoomNotFound
		}
		admitted := false
		for _, m := range detail.Members {
			if m.CharacterID == characterID {
				admitted = true
			}
		}
		if !admitted {
			return ErrRoomViewForbidden
		}
		r := detail.Room
		view = &RoomView{Room: RoomObservation{RoomSummary: summarizeRoom(*detail), LeaderCharacterID: r.LeaderCharacterID, Round: r.Round, CurrentBet: r.CurrentBet, MaxBet: r.MaxBet, Pot: r.Pot, WinnerCharacterID: r.WinnerCharacterID}, Members: make([]MemberObservation, 0, len(detail.Members))}
		for _, m := range detail.Members {
			view.Members = append(view.Members, MemberObservation{CharacterID: m.CharacterID, CharacterName: m.CharacterName, IsSpectator: m.IsSpectator, Action: m.Action, Card: m.Card, CardDisplay: m.CardDisplay})
		}
		return nil
	})
	return view, err
}

// getRoomDetail builds a masked snapshot for trusted command callers.
// External observations must use GetRoomView to verify player ownership/admission.
func (s *Service) getRoomDetail(ctx context.Context, roomID string, viewingCharID string) (*RoomDetail, error) {
	var detail *RoomDetail
	err := s.withRoomLock(ctx, roomID, func(lockedCtx context.Context) error {
		var err error
		detail, err = s.readRoomDetail(lockedCtx, roomID, viewingCharID)
		return err
	})
	return detail, err
}

func (s *Service) readRoomDetail(ctx context.Context, roomID, viewingCharID string) (*RoomDetail, error) {
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

	admitted := false
	for _, m := range members {
		if m.CharacterID == viewingCharID && viewingCharID != "" {
			admitted = true
		}
	}
	maskedMembers := make([]RoomMember, len(members))
	for i, m := range members {
		masked := m
		masked.CardDisplay = ""
		if m.Card >= 0 {
			if room.GameType == GameTypeDoppel && m.Card < len(AuthenticDoppelMarks) {
				masked.CardDisplay = string(AuthenticDoppelMarks[m.Card])
			} else if m.Card < len(AuthenticCardNames) {
				masked.CardDisplay = AuthenticCardNames[m.Card]
			}
		}

		if m.IsSpectator {
			masked.Card = -1
			masked.CardDisplay = ""
			masked.Action = ""
		} else if !admitted {
			masked.Card = -1
			masked.CardDisplay = "？"
			masked.Action = ""
		} else if room.Round > 0 {
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
				if m.CharacterID != viewingCharID && m.Action != "待機中" {
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
