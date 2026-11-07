package replay

import (
	"context"
	"errors"
	"strings"
)

// ErrArchiveSourceUnavailable distinguishes missing/undecodable frozen scenes from an empty archive.
var ErrArchiveSourceUnavailable = errors.New("replay archive source unavailable")

// ArchiveRepository persists scene sources, frozen copy intent and per-recipient latest slots.
// Staging methods MUST join the caller's ambient transaction so rollback leaves no source or copy.
type ArchiveRepository interface {
	// AppendScene stores the encoded scene unless the run already holds commandID or its accumulated
	// encoded bytes exceed limit; it reports whether this call appended.
	AppendScene(ctx context.Context, runID, commandID, actorID, encoded string, limit int64) (bool, error)
	// StageCopy freezes the run's current scene prefix for recipientID; "" means an empty source.
	StageCopy(ctx context.Context, runID, commandID, recipientID string) (string, error)
	// PublishPending materializes the recipient's pending copies; the latest slot only moves forward.
	PublishPending(ctx context.Context, recipientID string) error
	FindLatest(ctx context.Context, recipientID string) (StoredArchive, error)
}

// StoredArchive is the recipient's published slot; CopyID is empty when none was published.
type StoredArchive struct {
	CopyID  string
	Scenes  []string
	Pending bool
}

// LatestArchive is the owner's personal latest replay with decoded scenes, oldest first.
type LatestArchive struct {
	CopyID  string
	Scenes  []Scene
	Pending bool
}

type ArchiveService struct {
	repo ArchiveRepository
	auth recipientAuth
}

func NewArchiveService(repo ArchiveRepository, players PlayerAuthenticator, characters CharacterRepository) (*ArchiveService, error) {
	if repo == nil || players == nil || characters == nil {
		return nil, errors.New("replay archive repository, authentication and characters are required")
	}
	return &ArchiveService{repo: repo, auth: recipientAuth{players: players, characters: characters}}, nil
}

// StageScene projects one eligible operation inside the caller's command transaction.
func (s *ArchiveService) StageScene(ctx context.Context, runID, commandID, actorID string, c SceneCapture) (bool, error) {
	if blank(runID, commandID, actorID) {
		return false, ErrInvalidScene
	}
	scene, err := Project(c)
	if err != nil {
		return false, err
	}
	encoded, err := EncodeScene(scene)
	if err != nil {
		return false, err
	}
	return s.repo.AppendScene(ctx, runID, commandID, actorID, encoded, MaxSourceBytes)
}

// StageExitCopy freezes the exiting participant's prefix; call it before that exit's own scene.
func (s *ArchiveService) StageExitCopy(ctx context.Context, runID, commandID, recipientID string) (string, error) {
	if blank(runID, commandID, recipientID) {
		return "", ErrInvalidReplay
	}
	return s.repo.StageCopy(ctx, runID, commandID, recipientID)
}

// Publish is the trusted post-commit materialization for an already-authorized command outcome.
func (s *ArchiveService) Publish(ctx context.Context, recipientID string) error {
	if blank(recipientID) {
		return ErrInvalidReplay
	}
	return s.repo.PublishPending(ctx, recipientID)
}

// Recover lets the archive owner retry pending publication; it never reruns gameplay.
func (s *ArchiveService) Recover(ctx context.Context, sessionID, characterID string) error {
	if err := s.auth.history(ctx, sessionID, characterID); err != nil {
		return err
	}
	return s.repo.PublishPending(ctx, characterID)
}

// GetLatest reads the owner's latest slot without publishing pending work.
func (s *ArchiveService) GetLatest(ctx context.Context, sessionID, characterID string) (LatestArchive, error) {
	if err := s.auth.history(ctx, sessionID, characterID); err != nil {
		return LatestArchive{}, err
	}
	stored, err := s.repo.FindLatest(ctx, characterID)
	if err != nil {
		return LatestArchive{}, err
	}
	out := LatestArchive{CopyID: stored.CopyID, Pending: stored.Pending}
	for _, raw := range stored.Scenes {
		scene, err := DecodeJSON[Scene](raw)
		if err != nil || scene.Version != SceneVersion {
			return LatestArchive{}, ErrArchiveSourceUnavailable
		}
		out.Scenes = append(out.Scenes, scene)
	}
	return out, nil
}

func blank(values ...string) bool {
	for _, v := range values {
		if strings.TrimSpace(v) == "" {
			return true
		}
	}
	return false
}
