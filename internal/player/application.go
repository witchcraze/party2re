package player

import (
	"context"
	"errors"
	"log/slog"
	"time"

	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/logging"
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
	logger   logging.Logger
	now      func() time.Time
}

func NewService(players PlayerRepository, sessions SessionRepository, loggers ...logging.Logger) (*Service, error) {
	if players == nil || sessions == nil {
		return nil, errors.New("player dependencies are nil")
	}
	if len(loggers) > 1 {
		return nil, errors.New("player logger is configured more than once")
	}
	logger := logging.Nop()
	if len(loggers) == 1 {
		if loggers[0] == nil {
			return nil, errors.New("player logger is nil")
		}
		logger = loggers[0]
	}
	return &Service{players: players, sessions: sessions, logger: logger, now: time.Now}, nil
}

func (s *Service) Register(ctx context.Context, username, password string) (coreplayer.Player, error) {
	value, err := coreplayer.New(username, password, s.now())
	if err != nil {
		s.logger.Warn(ctx, "player.register", slog.String("username", username), slog.String("reason", "invalid_request"))
		return coreplayer.Player{}, err
	}
	if err := s.players.Save(ctx, value); err != nil {
		s.logger.Error(ctx, "player.register", err, slog.String("username", username))
		return coreplayer.Player{}, err
	}
	s.logger.Info(ctx, "player.register", slog.String("username", username), slog.String("player_id", value.ID))
	return value, nil
}

func (s *Service) Login(ctx context.Context, username, password string) (coreplayer.Session, error) {
	value, err := s.players.FindByUsername(ctx, username)
	if err != nil || !value.Authenticate(password) {
		s.logger.Warn(ctx, "player.login", slog.String("username", username), slog.String("reason", "authentication_failed"))
		return coreplayer.Session{}, coreplayer.ErrAuthentication
	}
	session, err := coreplayer.NewSession(value.ID, s.now(), SessionDuration)
	if err != nil {
		s.logger.Error(ctx, "player.login", err, slog.String("username", username))
		return coreplayer.Session{}, err
	}
	if err := s.sessions.Save(ctx, session); err != nil {
		s.logger.Error(ctx, "player.login", err, slog.String("username", username))
		return coreplayer.Session{}, err
	}
	s.logger.Info(ctx, "player.login", slog.String("username", username), slog.String("player_id", value.ID))
	return session, nil
}

func (s *Service) Logout(ctx context.Context, sessionID string) error {
	if err := s.sessions.Revoke(ctx, sessionID, s.now()); err != nil {
		s.logger.Error(ctx, "player.logout", err)
		return err
	}
	s.logger.Info(ctx, "player.logout")
	return nil
}

func (s *Service) Authenticate(ctx context.Context, sessionID string) (coreplayer.Player, error) {
	session, err := s.sessions.FindByID(ctx, sessionID)
	if err != nil || !session.Active(s.now()) {
		s.logger.Warn(ctx, "player.authenticate", slog.String("reason", "authentication_failed"))
		return coreplayer.Player{}, coreplayer.ErrAuthentication
	}
	value, err := s.players.FindByID(ctx, session.PlayerID)
	if err != nil {
		s.logger.Error(ctx, "player.authenticate", err)
		return coreplayer.Player{}, err
	}
	return value, nil
}
