package home

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	mrand "math/rand"
	"strings"
	"time"
	"unicode/utf8"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

type CharacterReader interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
}

type LetterListResult struct {
	Letters []Letter `json:"letters"`
	Total   int      `json:"total"`
	Limit   int      `json:"limit"`
	Offset  int      `json:"offset"`
}

type Service struct {
	repo       Repository
	charReader CharacterReader
	rng        *mrand.Rand
	nowFunc    func() time.Time
}

type ServiceOption func(*Service)

func WithNowFunc(fn func() time.Time) ServiceOption {
	return func(s *Service) {
		s.nowFunc = fn
	}
}

func WithRNG(rng *mrand.Rand) ServiceOption {
	return func(s *Service) {
		s.rng = rng
	}
}

func NewService(repo Repository, charReader CharacterReader, opts ...ServiceOption) (*Service, error) {
	if repo == nil {
		return nil, errors.New("repository is required")
	}
	if charReader == nil {
		return nil, errors.New("character reader is required")
	}
	s := &Service{
		repo:       repo,
		charReader: charReader,
		rng:        mrand.New(mrand.NewSource(time.Now().UnixNano())),
		nowFunc:    func() time.Time { return time.Now().UTC() },
	}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// GetHomeView retrieves the aggregated home view for a character, incrementing visitor count if visited by another character.
func (s *Service) GetHomeView(ctx context.Context, homeCharacterID, visitorCharacterID string) (HomeView, error) {
	homeChar, err := s.charReader.FindByID(ctx, homeCharacterID)
	if err != nil {
		if errors.Is(err, corecharacter.ErrNotFound) {
			return HomeView{}, ErrCharacterNotFound
		}
		return HomeView{}, err
	}

	isOwner := (homeCharacterID == visitorCharacterID)
	now := s.nowFunc().UTC()

	if !isOwner && visitorCharacterID != "" {
		_ = s.repo.IncrementVisitorCount(ctx, homeCharacterID, now)
	}

	h, err := s.repo.GetHome(ctx, homeCharacterID)
	if err != nil {
		return HomeView{}, err
	}

	unreadCount, _ := s.repo.GetUnreadLetterCount(ctx, homeCharacterID)
	phrases, _ := s.repo.ListCompanionPhrases(ctx, homeCharacterID)
	notices, _ := s.repo.ListDeliveryNotices(ctx, homeCharacterID, true)

	return HomeView{
		Owner:                homeChar,
		Home:                 h,
		UnreadLetterCount:    unreadCount,
		CompanionPhraseCount: len(phrases),
		RecentDeliveryCount:  len(notices),
		IsOwner:              isOwner,
	}, nil
}

// UpdateHome updates private home custom settings (theme, motto, companion_name).
func (s *Service) UpdateHome(ctx context.Context, characterID, theme, motto, companionName string) (CharacterHome, error) {
	_, err := s.charReader.FindByID(ctx, characterID)
	if err != nil {
		if errors.Is(err, corecharacter.ErrNotFound) {
			return CharacterHome{}, ErrCharacterNotFound
		}
		return CharacterHome{}, err
	}

	cleanTheme := strings.TrimSpace(theme)
	if cleanTheme == "" {
		cleanTheme = DefaultTheme
	}
	if len(cleanTheme) > MaxColorLength {
		cleanTheme = cleanTheme[:MaxColorLength]
	}

	cleanMotto := strings.TrimSpace(motto)
	if utf8.RuneCountInString(cleanMotto) > MaxMottoLength {
		cleanMotto = string([]rune(cleanMotto)[:MaxMottoLength])
	}

	cleanCompanion := strings.TrimSpace(companionName)
	if cleanCompanion == "" {
		cleanCompanion = DefaultCompanionName
	}
	if utf8.RuneCountInString(cleanCompanion) > MaxCompanionNameLength {
		cleanCompanion = string([]rune(cleanCompanion)[:MaxCompanionNameLength])
	}

	h, err := s.repo.GetHome(ctx, characterID)
	if err != nil {
		return CharacterHome{}, err
	}

	h.Theme = cleanTheme
	h.Motto = cleanMotto
	h.CompanionName = cleanCompanion
	h.UpdatedAt = s.nowFunc().UTC()

	if err := s.repo.SaveHome(ctx, h); err != nil {
		return CharacterHome{}, err
	}

	return h, nil
}

// SendLetter sends a player-to-player letter to a recipient character.
func (s *Service) SendLetter(ctx context.Context, senderID, recipientID, content, color string) (Letter, error) {
	if err := ValidateLetter(senderID, recipientID, content); err != nil {
		return Letter{}, err
	}

	sender, err := s.charReader.FindByID(ctx, senderID)
	if err != nil {
		if errors.Is(err, corecharacter.ErrNotFound) {
			return Letter{}, ErrCharacterNotFound
		}
		return Letter{}, err
	}

	recipient, err := s.charReader.FindByID(ctx, recipientID)
	if err != nil {
		if errors.Is(err, corecharacter.ErrNotFound) {
			return Letter{}, ErrCharacterNotFound
		}
		return Letter{}, err
	}

	cleanColor := strings.TrimSpace(color)
	if cleanColor == "" {
		cleanColor = "#000000"
	}
	if len(cleanColor) > MaxColorLength {
		cleanColor = cleanColor[:MaxColorLength]
	}

	now := s.nowFunc().UTC()
	letter := Letter{
		ID:                   generateID(),
		SenderCharacterID:    sender.ID,
		SenderName:           sender.Name,
		RecipientCharacterID: recipient.ID,
		RecipientName:        recipient.Name,
		Content:              strings.TrimSpace(content),
		Color:                cleanColor,
		IsRead:               false,
		ReadAt:               nil,
		CreatedAt:            now,
	}

	if err := s.repo.CreateLetter(ctx, letter); err != nil {
		return Letter{}, err
	}

	return letter, nil
}

// ReadLetter marks a letter as read for the recipient.
func (s *Service) ReadLetter(ctx context.Context, letterID, recipientID string) error {
	recipientID = strings.TrimSpace(recipientID)
	if recipientID == "" {
		return ErrInvalidRecipient
	}

	now := s.nowFunc().UTC()
	return s.repo.MarkLetterAsRead(ctx, letterID, recipientID, now)
}

// ListInbox retrieves received letters for a recipient character.
func (s *Service) ListInbox(ctx context.Context, recipientID string, limit, offset int) (LetterListResult, error) {
	recipientID = strings.TrimSpace(recipientID)
	if recipientID == "" {
		return LetterListResult{}, ErrInvalidRecipient
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	letters, total, err := s.repo.ListInboxLetters(ctx, recipientID, limit, offset)
	if err != nil {
		return LetterListResult{}, err
	}

	return LetterListResult{
		Letters: letters,
		Total:   total,
		Limit:   limit,
		Offset:  offset,
	}, nil
}

// ListOutbox retrieves sent letters for a sender character.
func (s *Service) ListOutbox(ctx context.Context, senderID string, limit, offset int) (LetterListResult, error) {
	senderID = strings.TrimSpace(senderID)
	if senderID == "" {
		return LetterListResult{}, ErrInvalidSender
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	letters, total, err := s.repo.ListOutboxLetters(ctx, senderID, limit, offset)
	if err != nil {
		return LetterListResult{}, err
	}

	return LetterListResult{
		Letters: letters,
		Total:   total,
		Limit:   limit,
		Offset:  offset,
	}, nil
}

// GetUnreadLetterCount returns the number of unread received letters.
func (s *Service) GetUnreadLetterCount(ctx context.Context, recipientID string) (int, error) {
	recipientID = strings.TrimSpace(recipientID)
	if recipientID == "" {
		return 0, ErrInvalidRecipient
	}
	return s.repo.GetUnreadLetterCount(ctx, recipientID)
}

// DeleteLetter deletes a letter belonging to either recipient or sender.
func (s *Service) DeleteLetter(ctx context.Context, letterID, characterID string) error {
	characterID = strings.TrimSpace(characterID)
	if characterID == "" {
		return ErrInvalidSender
	}
	return s.repo.DeleteLetter(ctx, letterID, characterID)
}

// TeachCompanionPhrase teaches a new greeting phrase to the home companion.
func (s *Service) TeachCompanionPhrase(ctx context.Context, characterID, phrase string) (CompanionPhrase, error) {
	if err := ValidatePhrase(phrase); err != nil {
		return CompanionPhrase{}, err
	}

	phrases, err := s.repo.ListCompanionPhrases(ctx, characterID)
	if err != nil {
		return CompanionPhrase{}, err
	}
	if len(phrases) >= MaxPhrasesPerCompanion {
		return CompanionPhrase{}, ErrMaxPhrasesReached
	}

	now := s.nowFunc().UTC()
	cp := CompanionPhrase{
		ID:          generateID(),
		CharacterID: characterID,
		Phrase:      strings.TrimSpace(phrase),
		CreatedAt:   now,
	}

	if err := s.repo.AddCompanionPhrase(ctx, cp); err != nil {
		return CompanionPhrase{}, err
	}

	return cp, nil
}

// ForgetCompanionPhrase removes a taught phrase.
func (s *Service) ForgetCompanionPhrase(ctx context.Context, phraseID, characterID string) error {
	return s.repo.DeleteCompanionPhrase(ctx, phraseID, characterID)
}

// ListCompanionPhrases returns all taught phrases for a character's companion.
func (s *Service) ListCompanionPhrases(ctx context.Context, characterID string) ([]CompanionPhrase, error) {
	return s.repo.ListCompanionPhrases(ctx, characterID)
}

// TalkToCompanion returns a greeting phrase spoken by the companion.
func (s *Service) TalkToCompanion(ctx context.Context, characterID string) (string, error) {
	phrases, err := s.repo.ListCompanionPhrases(ctx, characterID)
	if err != nil {
		return "", err
	}

	if len(phrases) == 0 {
		return "クエッ？（何か言いたそうにこちらを見つめている）", nil
	}

	idx := s.rng.Intn(len(phrases))
	return phrases[idx].Phrase, nil
}

// AddDeliveryNotice records an incoming delivery notice for a character.
func (s *Service) AddDeliveryNotice(ctx context.Context, characterID, noticeType, message string) error {
	cleanType := strings.TrimSpace(noticeType)
	if cleanType == "" {
		cleanType = "item_transfer"
	}
	notice := DeliveryNotice{
		ID:          generateID(),
		CharacterID: characterID,
		NoticeType:  cleanType,
		Message:     strings.TrimSpace(message),
		IsCleared:   false,
		CreatedAt:   s.nowFunc().UTC(),
	}
	return s.repo.AddDeliveryNotice(ctx, notice)
}

// ListDeliveryNotices returns delivery notices for a character.
func (s *Service) ListDeliveryNotices(ctx context.Context, characterID string, unclearedOnly bool) ([]DeliveryNotice, error) {
	return s.repo.ListDeliveryNotices(ctx, characterID, unclearedOnly)
}

// ClearDeliveryNotices marks all delivery notices as cleared.
func (s *Service) ClearDeliveryNotices(ctx context.Context, characterID string) error {
	return s.repo.ClearDeliveryNotices(ctx, characterID)
}
