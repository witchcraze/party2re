package collection

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

const (
	// DefaultTotalMonsters is the canonical completion threshold from legacy Party2 (_add_monster_book.cgi: my $complete = 180;).
	DefaultTotalMonsters = 180
	// DefaultTotalItems is the canonical completion threshold from legacy Party2 (collection.cgi: my $default_ites = 141;).
	DefaultTotalItems = 141
	// DefaultTotalWeapons is the canonical completion threshold from legacy Party2 (collection.cgi: $#weas = 71).
	DefaultTotalWeapons = 71
	// DefaultTotalArmors is the canonical completion threshold from legacy Party2 (collection.cgi: $#arms = 55).
	DefaultTotalArmors = 55
)

var (
	ErrInvalidCharacterID = errors.New("character ID cannot be empty")
	ErrInvalidMonsterID   = errors.New("monster ID cannot be empty")
	ErrInvalidItemID      = errors.New("item ID cannot be empty")
)

type MonsterBookEntry struct {
	CharacterID      string    `json:"character_id"`
	MonsterID        string    `json:"monster_id"`
	MonsterName      string    `json:"monster_name"`
	Habitat          string    `json:"habitat"`
	DefeatedCount    int       `json:"defeated_count"`
	FirstDefeatedAt  time.Time `json:"first_defeated_at"`
	LastDefeatedAt   time.Time `json:"last_defeated_at"`
	Icon             string    `json:"icon"`
	Strong           int       `json:"strong"`
	HP               int       `json:"hp"`
	MP               int       `json:"mp"`
	Attack           int       `json:"attack"`
	Defense          int       `json:"defense"`
	Agility          int       `json:"agility"`
	ExperienceReward int       `json:"exp_reward"`
	GoldReward       int       `json:"gold_reward"`
}

// DefeatedMonsterRecord contains combat and reward snapshot data for recording monster defeat.
type DefeatedMonsterRecord struct {
	MonsterID        string
	MonsterName      string
	Habitat          string
	Icon             string
	Strong           int
	HP               int
	MP               int
	Attack           int
	Defense          int
	Agility          int
	ExperienceReward int
	GoldReward       int
}

type ItemCollectionEntry struct {
	CharacterID  string    `json:"character_id"`
	ItemID       string    `json:"item_id"`
	ItemName     string    `json:"item_name"`
	Category     string    `json:"category"`
	DiscoveredAt time.Time `json:"discovered_at"`
}

type CompletionProgress struct {
	DiscoveredCount      int     `json:"discovered_count"`
	TotalCatalogCount    int     `json:"total_catalog_count"`
	CompletionPercentage float64 `json:"completion_percentage"`
	IsCompleted          bool    `json:"is_completed"`
}

type Repository interface {
	RecordMonsterDefeat(ctx context.Context, characterID string, record DefeatedMonsterRecord) error
	GetMonsterBook(ctx context.Context, characterID string) ([]MonsterBookEntry, error)
	GetMonsterBookCount(ctx context.Context, characterID string) (int, error)

	RecordItemDiscovered(ctx context.Context, characterID, itemID, itemName, category string) error
	GetItemCollection(ctx context.Context, characterID, category string) ([]ItemCollectionEntry, error)
	GetItemCollectionCount(ctx context.Context, characterID, category string) (int, error)

	// MarkCompleted records 100% completion milestone for a collection kind (e.g. "monster_book", "item").
	// Returns true if this was the initial completion (rows affected == 1).
	MarkCompleted(ctx context.Context, characterID, kind string) (bool, error)
	IsCompleted(ctx context.Context, characterID, kind string) (bool, error)
}

type NewsPublisher interface {
	PublishNews(ctx context.Context, category, title, content, author string, publishedAt time.Time) error
}

type NewsPublisherFunc func(ctx context.Context, category, title, content, author string, publishedAt time.Time) error

func (f NewsPublisherFunc) PublishNews(ctx context.Context, category, title, content, author string, publishedAt time.Time) error {
	return f(ctx, category, title, content, author, publishedAt)
}

type CharacterRepository interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
}

// LegendInductor defines permanent Hall of Fame induction contract (legend.cgi).
type LegendInductor interface {
	RecordLegend(ctx context.Context, category, characterID string) error
}

type LegendInductorFunc func(ctx context.Context, category, characterID string) error

func (f LegendInductorFunc) RecordLegend(ctx context.Context, category, characterID string) error {
	return f(ctx, category, characterID)
}

type Option func(*Service)

func WithNewsPublisher(pub NewsPublisher) Option {
	return func(s *Service) {
		s.newsPub = pub
	}
}

func WithCharacterRepository(repo CharacterRepository) Option {
	return func(s *Service) {
		s.charRepo = repo
	}
}

func WithLegendInductor(inductor LegendInductor) Option {
	return func(s *Service) {
		s.legend = inductor
	}
}

func WithTotalWeapons(n int) Option {
	return func(s *Service) {
		s.totalWeapons = n
	}
}

func WithTotalArmors(n int) Option {
	return func(s *Service) {
		s.totalArmors = n
	}
}

type Service struct {
	repo          Repository
	totalMonsters int
	totalItems    int
	totalWeapons  int
	totalArmors   int
	newsPub       NewsPublisher
	charRepo      CharacterRepository
	legend        LegendInductor
}

func NewService(repo Repository, totalMonsters, totalItems int, opts ...Option) (*Service, error) {
	if repo == nil {
		return nil, errors.New("repository is required")
	}
	if totalMonsters <= 0 {
		totalMonsters = DefaultTotalMonsters
	}
	if totalItems <= 0 {
		totalItems = DefaultTotalItems
	}
	s := &Service{
		repo:          repo,
		totalMonsters: totalMonsters,
		totalItems:    totalItems,
		totalWeapons:  DefaultTotalWeapons,
		totalArmors:   DefaultTotalArmors,
	}
	for _, opt := range opts {
		opt(s)
	}
	if s.totalWeapons <= 0 {
		s.totalWeapons = DefaultTotalWeapons
	}
	if s.totalArmors <= 0 {
		s.totalArmors = DefaultTotalArmors
	}
	return s, nil
}

func (s *Service) SetLegendInductor(inductor LegendInductor) {
	s.legend = inductor
}

func (s *Service) RecordMonsterDefeat(ctx context.Context, characterID string, record DefeatedMonsterRecord) error {
	if characterID == "" {
		return ErrInvalidCharacterID
	}
	if record.MonsterID == "" {
		return ErrInvalidMonsterID
	}
	if record.Strong == 0 && (record.HP > 0 || record.Attack > 0 || record.Defense > 0 || record.Agility > 0) {
		record.Strong = CalculateStrong(record.HP, record.MP, record.Attack, record.Defense, record.Agility)
	}
	if record.Icon == "" {
		record.Icon = deriveMonsterIcon(record.MonsterID)
	}
	if err := s.repo.RecordMonsterDefeat(ctx, characterID, record); err != nil {
		return err
	}
	s.checkMonsterBookCompletion(ctx, characterID)
	return nil
}

// CalculateStrong calculates combat strength metric matching legacy Perl CGI (party2/lib/_battle.cgi:1380-1383):
// int(mhp + mmp + at + df * 0.5 + ag)
func CalculateStrong(hp, mp, at, df, ag int) int {
	return int(float64(hp) + float64(mp) + float64(at) + float64(df)*0.5 + float64(ag))
}

func deriveMonsterIcon(monsterID string) string {
	if strings.HasPrefix(monsterID, "monster-") {
		return fmt.Sprintf("mon/%s.gif", strings.TrimPrefix(monsterID, "monster-"))
	}
	if strings.HasPrefix(monsterID, "mon_") {
		return fmt.Sprintf("mon/%s.gif", strings.TrimPrefix(monsterID, "mon_"))
	}
	return monsterID
}

func (s *Service) checkMonsterBookCompletion(ctx context.Context, characterID string) {
	if s.totalMonsters <= 0 {
		return
	}
	count, err := s.repo.GetMonsterBookCount(ctx, characterID)
	if err != nil || count < s.totalMonsters {
		return
	}
	newlyCompleted, err := s.repo.MarkCompleted(ctx, characterID, "monster_book")
	if err != nil || !newlyCompleted {
		return
	}
	if s.newsPub != nil {
		charName := s.resolveCharacterName(ctx, characterID)
		msg := fmt.Sprintf("%sがモンスターブックをコンプリートしました！", charName)
		_ = s.newsPub.PublishNews(ctx, "collection", msg, msg, "System", time.Now().UTC())
	}
	if s.legend != nil {
		_ = s.legend.RecordLegend(ctx, "comp_mon", characterID)
	}
}

func (s *Service) GetMonsterBook(ctx context.Context, characterID string) ([]MonsterBookEntry, CompletionProgress, error) {
	if characterID == "" {
		return nil, CompletionProgress{}, ErrInvalidCharacterID
	}
	entries, err := s.repo.GetMonsterBook(ctx, characterID)
	if err != nil {
		return nil, CompletionProgress{}, err
	}
	discovered := len(entries)
	percentage := 0.0
	if s.totalMonsters > 0 {
		percentage = (float64(discovered) / float64(s.totalMonsters)) * 100.0
		if percentage > 100.0 {
			percentage = 100.0
		}
	}

	progress := CompletionProgress{
		DiscoveredCount:      discovered,
		TotalCatalogCount:    s.totalMonsters,
		CompletionPercentage: percentage,
		IsCompleted:          s.totalMonsters > 0 && discovered >= s.totalMonsters,
	}
	return entries, progress, nil
}

func normalizeCategory(cat string) string {
	switch strings.ToLower(strings.TrimSpace(cat)) {
	case "weapon", "main-hand":
		return "weapon"
	case "armor", "shield", "off-hand", "body", "accessory":
		return "armor"
	default:
		return "item"
	}
}

func (s *Service) RecordItemDiscovered(ctx context.Context, characterID, itemID, itemName, category string) error {
	if characterID == "" {
		return ErrInvalidCharacterID
	}
	if itemID == "" {
		return ErrInvalidItemID
	}
	normCat := normalizeCategory(category)
	if err := s.repo.RecordItemDiscovered(ctx, characterID, itemID, itemName, normCat); err != nil {
		return err
	}
	switch normCat {
	case "weapon":
		s.checkWeaponCollectionCompletion(ctx, characterID)
	case "armor":
		s.checkArmorCollectionCompletion(ctx, characterID)
	default:
		s.checkItemCollectionCompletion(ctx, characterID)
	}
	return nil
}

func (s *Service) checkWeaponCollectionCompletion(ctx context.Context, characterID string) {
	if s.totalWeapons <= 0 {
		return
	}
	_, progress, err := s.GetWeaponCollection(ctx, characterID)
	if err != nil || !progress.IsCompleted {
		return
	}
	newlyCompleted, err := s.repo.MarkCompleted(ctx, characterID, "weapon")
	if err != nil || !newlyCompleted {
		return
	}
	if s.newsPub != nil {
		charName := s.resolveCharacterName(ctx, characterID)
		msg := fmt.Sprintf("%sが武器図鑑をコンプリートしました！", charName)
		_ = s.newsPub.PublishNews(ctx, "collection", msg, msg, "System", time.Now().UTC())
	}
	if s.legend != nil {
		_ = s.legend.RecordLegend(ctx, "comp_wea", characterID)
	}
}

func (s *Service) checkArmorCollectionCompletion(ctx context.Context, characterID string) {
	if s.totalArmors <= 0 {
		return
	}
	_, progress, err := s.GetArmorCollection(ctx, characterID)
	if err != nil || !progress.IsCompleted {
		return
	}
	newlyCompleted, err := s.repo.MarkCompleted(ctx, characterID, "armor")
	if err != nil || !newlyCompleted {
		return
	}
	if s.newsPub != nil {
		charName := s.resolveCharacterName(ctx, characterID)
		msg := fmt.Sprintf("%sが防具図鑑をコンプリートしました！", charName)
		_ = s.newsPub.PublishNews(ctx, "collection", msg, msg, "System", time.Now().UTC())
	}
	if s.legend != nil {
		_ = s.legend.RecordLegend(ctx, "comp_arm", characterID)
	}
}

func (s *Service) checkItemCollectionCompletion(ctx context.Context, characterID string) {
	if s.totalItems <= 0 {
		return
	}
	count, err := s.repo.GetItemCollectionCount(ctx, characterID, "item")
	if err != nil || count < s.totalItems {
		return
	}
	newlyCompleted, err := s.repo.MarkCompleted(ctx, characterID, "item")
	if err != nil || !newlyCompleted {
		return
	}
	if s.newsPub != nil {
		charName := s.resolveCharacterName(ctx, characterID)
		msg := fmt.Sprintf("%sがアイテム図鑑をコンプリートしました！", charName)
		_ = s.newsPub.PublishNews(ctx, "collection", msg, msg, "System", time.Now().UTC())
	}
	if s.legend != nil {
		_ = s.legend.RecordLegend(ctx, "comp_ite", characterID)
	}
}

func (s *Service) GetItemCollection(ctx context.Context, characterID, category string) ([]ItemCollectionEntry, CompletionProgress, error) {
	if characterID == "" {
		return nil, CompletionProgress{}, ErrInvalidCharacterID
	}
	if strings.TrimSpace(category) == "" {
		category = "item"
	}
	entries, err := s.repo.GetItemCollection(ctx, characterID, category)
	if err != nil {
		return nil, CompletionProgress{}, err
	}
	totalDiscovered, err := s.repo.GetItemCollectionCount(ctx, characterID, category)
	if err != nil {
		return nil, CompletionProgress{}, err
	}
	percentage := 0.0
	if s.totalItems > 0 {
		percentage = (float64(totalDiscovered) / float64(s.totalItems)) * 100.0
		if percentage > 100.0 {
			percentage = 100.0
		}
	}

	progress := CompletionProgress{
		DiscoveredCount:      totalDiscovered,
		TotalCatalogCount:    s.totalItems,
		CompletionPercentage: percentage,
		IsCompleted:          s.totalItems > 0 && totalDiscovered >= s.totalItems,
	}
	return entries, progress, nil
}

func (s *Service) GetWeaponCollection(ctx context.Context, characterID string) ([]ItemCollectionEntry, CompletionProgress, error) {
	if characterID == "" {
		return nil, CompletionProgress{}, ErrInvalidCharacterID
	}
	entries, err := s.repo.GetItemCollection(ctx, characterID, "weapon")
	if err != nil {
		return nil, CompletionProgress{}, err
	}
	discovered := len(entries)
	percentage := 0.0
	if s.totalWeapons > 0 {
		percentage = (float64(discovered) / float64(s.totalWeapons)) * 100.0
		if percentage > 100.0 {
			percentage = 100.0
		}
	}

	progress := CompletionProgress{
		DiscoveredCount:      discovered,
		TotalCatalogCount:    s.totalWeapons,
		CompletionPercentage: percentage,
		IsCompleted:          s.totalWeapons > 0 && discovered >= s.totalWeapons,
	}
	return entries, progress, nil
}

func (s *Service) GetArmorCollection(ctx context.Context, characterID string) ([]ItemCollectionEntry, CompletionProgress, error) {
	if characterID == "" {
		return nil, CompletionProgress{}, ErrInvalidCharacterID
	}
	entries, err := s.repo.GetItemCollection(ctx, characterID, "armor")
	if err != nil {
		return nil, CompletionProgress{}, err
	}
	discovered := len(entries)
	percentage := 0.0
	if s.totalArmors > 0 {
		percentage = (float64(discovered) / float64(s.totalArmors)) * 100.0
		if percentage > 100.0 {
			percentage = 100.0
		}
	}

	progress := CompletionProgress{
		DiscoveredCount:      discovered,
		TotalCatalogCount:    s.totalArmors,
		CompletionPercentage: percentage,
		IsCompleted:          s.totalArmors > 0 && discovered >= s.totalArmors,
	}
	return entries, progress, nil
}

func (s *Service) resolveCharacterName(ctx context.Context, characterID string) string {
	if s.charRepo != nil {
		if c, err := s.charRepo.FindByID(ctx, characterID); err == nil && c.Name != "" {
			return c.Name
		}
	}
	return characterID
}
