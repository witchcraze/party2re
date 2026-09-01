package player

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/logging"
)

const SessionDuration = 24 * time.Hour

type PlayerRepository interface {
	Save(context.Context, coreplayer.Player) error
	FindByUsername(context.Context, string) (coreplayer.Player, error)
	FindByID(context.Context, string) (coreplayer.Player, error)
	Delete(context.Context, string) error
}

type SessionRepository interface {
	Save(context.Context, coreplayer.Session) error
	FindByID(context.Context, string) (coreplayer.Session, error)
	Revoke(context.Context, string, time.Time) error
}

type CharacterService interface {
	FindByPlayerID(ctx context.Context, playerID string) ([]corecharacter.Character, error)
	Delete(ctx context.Context, playerID, characterID string) error
}

type TransactionProvider interface {
	RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type Option func(*Service)

// WithLogger configures a logger for the player service.
func WithLogger(logger logging.Logger) Option {
	return func(s *Service) {
		if logger != nil {
			s.logger = logger
		}
	}
}

// WithTransactionProvider sets the transaction provider.
func WithTransactionProvider(txProvider TransactionProvider) Option {
	return func(s *Service) {
		s.txProvider = txProvider
	}
}

// WithCharacterService sets the character service for cascading deletions.
func WithCharacterService(charService CharacterService) Option {
	return func(s *Service) {
		s.characters = charService
	}
}

type Service struct {
	players    PlayerRepository
	sessions   SessionRepository
	characters CharacterService
	txProvider TransactionProvider
	logger     logging.Logger
	now        func() time.Time
}

func NewService(players PlayerRepository, sessions SessionRepository, opts ...Option) (*Service, error) {
	if players == nil || sessions == nil {
		return nil, errors.New("player dependencies are nil")
	}
	s := &Service{
		players:  players,
		sessions: sessions,
		logger:   logging.Nop(),
		now:      time.Now,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}
	return s, nil
}

func (s *Service) runInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if s.txProvider != nil {
		return s.txProvider.RunInTx(ctx, fn)
	}
	return fn(ctx)
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

// DeleteAccount deletes a player account and cascades through all characters and resources.
func (s *Service) DeleteAccount(ctx context.Context, playerID, password string) error {
	playerID = strings.TrimSpace(playerID)
	if playerID == "" {
		return errors.New("player ID is required")
	}

	p, err := s.players.FindByID(ctx, playerID)
	if err != nil {
		return err
	}

	if password != "" && !p.Authenticate(password) {
		s.logger.Warn(ctx, "player.delete", slog.String("player_id", playerID), slog.String("reason", "authentication_failed"))
		return coreplayer.ErrAuthentication
	}

	// Delete all player characters first
	if s.characters != nil {
		chars, err := s.characters.FindByPlayerID(ctx, playerID)
		if err == nil {
			for _, char := range chars {
				if err := s.characters.Delete(ctx, playerID, char.ID); err != nil {
					s.logger.Error(ctx, "player.delete.character", err, slog.String("player_id", playerID), slog.String("character_id", char.ID))
					return err
				}
			}
		}
	}

	// Delete player and player-level resources
	err = s.runInTx(ctx, func(txCtx context.Context) error {
		return s.players.Delete(txCtx, playerID)
	})
	if err != nil {
		s.logger.Error(ctx, "player.delete", err, slog.String("player_id", playerID))
		return err
	}

	s.logger.Info(ctx, "player.delete", slog.String("player_id", playerID))
	return nil
}
