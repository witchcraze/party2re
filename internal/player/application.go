package player

import (
	"context"
	"errors"
	"time"

	coreplayer "github.com/witchcraze/party2re/internal/core/player"
)

const SessionDuration = 24 * time.Hour

type PlayerRepository interface {
	Save(context.Context, coreplayer.Player) error
	FindByUsername(context.Context, string) (coreplayer.Player, error)
	FindByID(context.Context, string) (coreplayer.Player, error)
}

type SessionRepository interface {
	Save(context.Context, coreplayer.Session) error
	FindByID(context.Context, string) (coreplayer.Session, error)
	Revoke(context.Context, string, time.Time) error
}

type Service struct {
	players  PlayerRepository
	sessions SessionRepository
	now      func() time.Time
}

func NewService(players PlayerRepository, sessions SessionRepository) (*Service, error) {
	if players == nil || sessions == nil {
		return nil, errors.New("player dependencies are nil")
	}
	return &Service{players: players, sessions: sessions, now: time.Now}, nil
}

func (s *Service) Register(ctx context.Context, username, password string) (coreplayer.Player, error) {
	value, err := coreplayer.New(username, password, s.now())
	if err != nil {
		return coreplayer.Player{}, err
	}
	if err := s.players.Save(ctx, value); err != nil {
		return coreplayer.Player{}, err
	}
	return value, nil
}

func (s *Service) Login(ctx context.Context, username, password string) (coreplayer.Session, error) {
	value, err := s.players.FindByUsername(ctx, username)
	if err != nil || !value.Authenticate(password) {
		return coreplayer.Session{}, coreplayer.ErrAuthentication
	}
	session, err := coreplayer.NewSession(value.ID, s.now(), SessionDuration)
	if err != nil {
		return coreplayer.Session{}, err
	}
	if err := s.sessions.Save(ctx, session); err != nil {
		return coreplayer.Session{}, err
	}
	return session, nil
}

func (s *Service) Logout(ctx context.Context, sessionID string) error {
	return s.sessions.Revoke(ctx, sessionID, s.now())
}

func (s *Service) Authenticate(ctx context.Context, sessionID string) (coreplayer.Player, error) {
	session, err := s.sessions.FindByID(ctx, sessionID)
	if err != nil || !session.Active(s.now()) {
		return coreplayer.Player{}, coreplayer.ErrAuthentication
	}
	return s.players.FindByID(ctx, session.PlayerID)
}
