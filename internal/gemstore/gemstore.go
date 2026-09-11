package gemstore

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/economy"
)

type inMemoryGemBoxRepo struct {
	mu    sync.RWMutex
	boxes map[string]GemBox
}

func newInMemoryGemBoxRepo() *inMemoryGemBoxRepo {
	return &inMemoryGemBoxRepo{boxes: make(map[string]GemBox)}
}

func (r *inMemoryGemBoxRepo) FindByCharacterID(ctx context.Context, characterID string) (GemBox, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	b, ok := r.boxes[characterID]
	if !ok {
		return GemBox{}, ErrGemBoxNotFound
	}
	return b, nil
}

func (r *inMemoryGemBoxRepo) FindByCharacterIDForUpdate(ctx context.Context, characterID string) (GemBox, error) {
	return r.FindByCharacterID(ctx, characterID)
}

func (r *inMemoryGemBoxRepo) Save(ctx context.Context, box GemBox) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.boxes[box.CharacterID] = box
	return nil
}

type Option func(*Service)

// WithItemDefinitionProvider configures the item definition catalog.
func WithItemDefinitionProvider(provider ItemDefinitionProvider) Option {
	return func(s *Service) {
		s.items = provider
	}
}

// WithTransactionProvider configures the transaction provider.
func WithTransactionProvider(provider TransactionProvider) Option {
	return func(s *Service) {
		s.txProvider = provider
	}
}

// WithRandomSource configures an explicit random source (useful for deterministic tests).
func WithRandomSource(r RandomSource) Option {
	return func(s *Service) {
		s.randomSource = r
	}
}

// WithGemBoxRepository configures the dedicated gem box repository.
func WithGemBoxRepository(repo GemBoxRepository) Option {
	return func(s *Service) {
		s.gemBoxes = repo
	}
}

// WithDepotRepository configures the depot repository for crafting materials.
func WithDepotRepository(repo DepotRepository) Option {
	return func(s *Service) {
		s.depots = repo
	}
}

// Service implements all gem store operations and domain invariants.
type Service struct {
	catalog      *Catalog
	characters   CharacterRepository
	inventories  InventoryRepository
	depots       DepotRepository
	gemBoxes     GemBoxRepository
	items        ItemDefinitionProvider
	txProvider   TransactionProvider
	randomSource RandomSource
	economy      *economy.Service
}

// NewService creates a new GemStore service instance.
func NewService(
	catalog *Catalog,
	characters CharacterRepository,
	inventories InventoryRepository,
	opts ...Option,
) (*Service, error) {
	if catalog == nil || characters == nil || inventories == nil {
		return nil, ErrNilDependency
	}

	s := &Service{
		catalog:      catalog,
		characters:   characters,
		inventories:  inventories,
		gemBoxes:     newInMemoryGemBoxRepo(),
		randomSource: DefaultRandomSource(),
	}

	for _, opt := range opts {
		opt(s)
	}

	var ecoOpts []economy.Option
	if s.txProvider != nil {
		ecoOpts = append(ecoOpts, economy.WithTransactionProvider(s.txProvider))
	}
	eco, err := economy.NewService(characters, inventories, ecoOpts...)
	if err != nil {
		return nil, err
	}
	s.economy = eco

	return s, nil
}

// GetGemBox retrieves the character's gem box with capacity dynamically derived from job_lv.
func (s *Service) GetGemBox(ctx context.Context, characterID string) (GemBox, error) {
	characterID = strings.TrimSpace(characterID)
	if characterID == "" {
		return GemBox{}, ErrInvalidCharacterID
	}

	char, err := s.characters.FindByID(ctx, characterID)
	if err != nil {
		return GemBox{}, err
	}

	expectedCap := CalculateGemBoxCapacity(char.JobLevel)
	box, err := s.gemBoxes.FindByCharacterID(ctx, characterID)
	if errors.Is(err, ErrGemBoxNotFound) {
		return GemBox{
			CharacterID: characterID,
			Capacity:    expectedCap,
			Items:       []coreitem.Instance{},
		}, nil
	}
	if err != nil {
		return GemBox{}, err
	}
	box.Capacity = expectedCap
	return box, nil
}

func (s *Service) getOrCreateGemBoxForUpdate(ctx context.Context, char corecharacter.Character) (GemBox, error) {
	expectedCap := CalculateGemBoxCapacity(char.JobLevel)
	box, err := s.gemBoxes.FindByCharacterIDForUpdate(ctx, char.ID)
	if errors.Is(err, ErrGemBoxNotFound) {
		return GemBox{
			CharacterID: char.ID,
			Capacity:    expectedCap,
			Items:       []coreitem.Instance{},
		}, nil
	}
	if err != nil {
		return GemBox{}, err
	}
	box.Capacity = expectedCap
	return box, nil
}

// SortGemBox sorts the gems in the character's gem box by catalog index ascending (party2: seiton).
func (s *Service) SortGemBox(ctx context.Context, characterID string) (GemBox, error) {
	characterID = strings.TrimSpace(characterID)
	if characterID == "" {
		return GemBox{}, ErrInvalidCharacterID
	}

	var sortedBox GemBox
	run := func(txCtx context.Context) error {
		char, err := s.characters.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		box, err := s.getOrCreateGemBoxForUpdate(txCtx, char)
		if err != nil {
			return err
		}

		sort.SliceStable(box.Items, func(i, j int) bool {
			rankI := s.catalog.GemRank(box.Items[i].DefinitionID)
			rankJ := s.catalog.GemRank(box.Items[j].DefinitionID)
			if rankI != rankJ {
				return rankI < rankJ
			}
			return box.Items[i].DefinitionID < box.Items[j].DefinitionID
		})

		if err := s.gemBoxes.Save(txCtx, box); err != nil {
			return err
		}
		sortedBox = box
		return nil
	}

	if s.txProvider != nil {
		if err := s.txProvider.RunInTx(ctx, run); err != nil {
			return GemBox{}, err
		}
	} else {
		if err := run(ctx); err != nil {
			return GemBox{}, err
		}
	}

	return sortedBox, nil
}

// GetCatalog returns the gem catalog available for purchase at the specified job level (transfer count).
func (s *Service) GetCatalog(jobLevel int) []Gem {
	return s.catalog.GetGemsForJobLevel(jobLevel)
}

// GetRecipes returns all known gem synthesis recipes.
func (s *Service) GetRecipes() []Recipe {
	return s.catalog.AllRecipes()
}

// GetDialogue returns shopkeeper @ジェマ dialogue lines.
func (s *Service) GetDialogue() []string {
	return []string{
		"ここは宝石店です。色んな宝石を売っていますよ",
		"どんな宝石がお好きですか？",
		"アイテムや宝石を使って、新しい宝石に加工することもできますよ",
		"宝石ごとに不思議な力があるんです",
		"あなたにはキラリと光る素敵な宝石がお似合いですね",
	}
}
