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

const SessionDuration = 7 * 24 * time.Hour

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
	DeleteByPlayerID(context.Context, string) error
}

type APITokenRepository interface {
	Save(context.Context, coreplayer.APIToken) error
	FindByTokenHash(context.Context, string) (coreplayer.APIToken, error)
	FindByPlayerID(context.Context, string) ([]coreplayer.APIToken, error)
	TouchLastUsed(context.Context, string, time.Time) error
	Revoke(context.Context, string, string) error
	DeleteByPlayerID(context.Context, string) error
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

// WithAPITokenRepository sets the API token repository for Personal Access Tokens.
func WithAPITokenRepository(tokenRepo APITokenRepository) Option {
	return func(s *Service) {
		s.apiTokens = tokenRepo
	}
}

type Service struct {
	players    PlayerRepository
	sessions   SessionRepository
	apiTokens  APITokenRepository
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

func (s *Service) Authenticate(ctx context.Context, token string) (coreplayer.Player, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return coreplayer.Player{}, coreplayer.ErrAuthentication
	}

	// 1. Dual authentication: Check if token is a Personal Access Token (API Key)
	if strings.HasPrefix(token, coreplayer.APITokenPrefix) {
		if s.apiTokens == nil {
			s.logger.Warn(ctx, "player.authenticate", slog.String("reason", "api_tokens_unsupported"))
			return coreplayer.Player{}, coreplayer.ErrAuthentication
		}
		hash := coreplayer.HashAPIToken(token)
		apiToken, err := s.apiTokens.FindByTokenHash(ctx, hash)
		if err != nil || !apiToken.Active(s.now()) {
			s.logger.Warn(ctx, "player.authenticate", slog.String("reason", "api_token_invalid_or_expired"))
			return coreplayer.Player{}, coreplayer.ErrAuthentication
		}
		// Update last_used_at
		_ = s.apiTokens.TouchLastUsed(ctx, apiToken.ID, s.now())

		player, err := s.players.FindByID(ctx, apiToken.PlayerID)
		if err != nil {
			s.logger.Error(ctx, "player.authenticate", err)
			return coreplayer.Player{}, err
		}
		return player, nil
	}

	// 2. Interactive session token
	session, err := s.sessions.FindByID(ctx, token)
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

// CreateAPIToken creates a new Personal Access Token for the specified player.
func (s *Service) CreateAPIToken(ctx context.Context, playerID string, name string, expiresAt *time.Time) (coreplayer.APIToken, string, error) {
	if s.apiTokens == nil {
		return coreplayer.APIToken{}, "", errors.New("api token repository not configured")
	}
	token, plaintext, err := coreplayer.NewAPIToken(playerID, name, expiresAt, s.now())
	if err != nil {
		s.logger.Warn(ctx, "player.create_token", slog.String("player_id", playerID), slog.String("reason", err.Error()))
		return coreplayer.APIToken{}, "", err
	}
	if err := s.apiTokens.Save(ctx, token); err != nil {
		s.logger.Error(ctx, "player.create_token", err, slog.String("player_id", playerID))
		return coreplayer.APIToken{}, "", err
	}
	s.logger.Info(ctx, "player.create_token", slog.String("player_id", playerID), slog.String("token_id", token.ID))
	return token, plaintext, nil
}

// ListAPITokens returns all API tokens belonging to the specified player.
func (s *Service) ListAPITokens(ctx context.Context, playerID string) ([]coreplayer.APIToken, error) {
	if s.apiTokens == nil {
		return nil, errors.New("api token repository not configured")
	}
	return s.apiTokens.FindByPlayerID(ctx, playerID)
}

// RevokeAPIToken revokes (deletes) a specific API token belonging to the player.
func (s *Service) RevokeAPIToken(ctx context.Context, playerID, tokenID string) error {
	if s.apiTokens == nil {
		return errors.New("api token repository not configured")
	}
	if err := s.apiTokens.Revoke(ctx, playerID, tokenID); err != nil {
		s.logger.Warn(ctx, "player.revoke_token", slog.String("player_id", playerID), slog.String("token_id", tokenID), slog.String("reason", err.Error()))
		return err
	}
	s.logger.Info(ctx, "player.revoke_token", slog.String("player_id", playerID), slog.String("token_id", tokenID))
	return nil
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

	// Delete active player sessions
	if s.sessions != nil {
		if err := s.sessions.DeleteByPlayerID(ctx, playerID); err != nil {
			s.logger.Warn(ctx, "player.delete.sessions", slog.String("player_id", playerID), slog.String("reason", err.Error()))
		}
	}

	// Delete active player API tokens
	if s.apiTokens != nil {
		if err := s.apiTokens.DeleteByPlayerID(ctx, playerID); err != nil {
			s.logger.Warn(ctx, "player.delete.api_tokens", slog.String("player_id", playerID), slog.String("reason", err.Error()))
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
