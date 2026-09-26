package customskill

import (
	"context"
	"errors"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/gemstore"
)

var (
	ErrCharacterNotFound   = errors.New("character not found")
	ErrInvalidSkillName    = errors.New("custom skill name is invalid")
	ErrInvalidSkillComment = errors.New("custom skill comment is invalid")
	ErrTooManyGemSlots     = errors.New("custom skill gem slots exceed 3")
	ErrGemNotOwned         = errors.New("custom skill gem is not owned")
	ErrGemNotFound         = errors.New("custom skill gem not found")
	ErrGemDependencies     = errors.New("custom skill gem dependencies are not configured")
)

type GemDefinition struct {
	ID       string
	Name     string
	SlotCost int
	MPCost   int
}

type GemProvider interface {
	FindGemByID(id string) (GemDefinition, bool)
}

type GemBoxProvider interface {
	FindByCharacterIDForUpdate(ctx context.Context, characterID string) (gemstore.GemBox, error)
	Save(ctx context.Context, box gemstore.GemBox) error
}

type TransactionProvider interface {
	RunInTx(ctx context.Context, fn func(context.Context) error) error
}

type CustomSkill struct {
	CharacterID string    `json:"character_id"`
	Name        string    `json:"name"`
	Comment     string    `json:"comment"`
	CMP         int       `json:"cmp"`
	Gems        [3]string `json:"gems"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type CharacterProvider interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
}

type Repository interface {
	SaveCustomSkill(ctx context.Context, skill CustomSkill) error
	FindCustomSkill(ctx context.Context, characterID string) (*CustomSkill, error)
}

type Service struct {
	repo       Repository
	charRepo   CharacterProvider
	gems       GemProvider
	gemBox     GemBoxProvider
	txProvider TransactionProvider
}

func NewService(repo Repository, charRepo CharacterProvider) (*Service, error) {
	if repo == nil {
		return nil, errors.New("custom skill repository is required")
	}
	if charRepo == nil {
		return nil, errors.New("character provider is required")
	}
	return &Service{repo: repo, charRepo: charRepo}, nil
}

func (s *Service) ConfigureGemSynthesis(gems GemProvider, gemBox GemBoxProvider, txProvider TransactionProvider) {
	s.gems = gems
	s.gemBox = gemBox
	s.txProvider = txProvider
}
