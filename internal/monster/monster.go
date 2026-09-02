package monster

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/id"
)

var (
	ErrMonsterNotFound        = errors.New("monster not found")
	ErrBoxFull                = errors.New("monster box capacity reached")
	ErrHomeFull               = errors.New("home pet capacity reached")
	ErrDuplicatePetNameAtHome = errors.New("a pet with the same name already exists in home")
	ErrInvalidName            = errors.New("monster name contains invalid characters or whitespace")
	ErrNameTooLong            = errors.New("monster name exceeds maximum length of 8 characters")
	ErrCannotSendToSelf       = errors.New("cannot send monster to yourself")
	ErrRecipientNotFound      = errors.New("recipient character not found")
	ErrRecipientBoxFull       = errors.New("recipient monster box is full")
	ErrCharacterNotFound      = errors.New("character not found")
	ErrInvalidLocation        = errors.New("invalid monster location")
	ErrForbidden              = errors.New("forbidden: monster belongs to another character")
)

const (
	LocationBox            = "box"
	LocationHome           = "home"
	BaseBoxCapacity        = 50
	OverBoxCapacityPerTier = 50
	MaxHomePets            = 8
	MaxMonsterNameLength   = 8
)

// MonsterInstance represents a captured/tamed monster stored in Monster Grandpa or living as a home pet.
type MonsterInstance struct {
	ID          string    `json:"id"`
	CharacterID string    `json:"character_id"`
	MonsterID   string    `json:"monster_id"`
	CustomName  string    `json:"custom_name"`
	Location    string    `json:"location"` // "box" or "home"
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// MonsterBoxSummary holds counts and monsters for UI/API consumption.
type MonsterBoxSummary struct {
	BoxCount     int               `json:"box_count"`
	BoxCapacity  int               `json:"box_capacity"`
	HomeCount    int               `json:"home_count"`
	HomeCapacity int               `json:"home_capacity"`
	Monsters     []MonsterInstance `json:"monsters"`
}

// Dialogue represents NPC @モンジィ dialogue.
type Dialogue struct {
	NPCName  string   `json:"npc_name"`
	Title    string   `json:"title"`
	Greeting string   `json:"greeting"`
	Phrases  []string `json:"phrases"`
}

// CharacterRepository defines character data access for Monster service.
type CharacterRepository interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
	FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error)
}

// MonsterRepository defines storage operations for monster instances.
type MonsterRepository interface {
	ListByCharacterID(ctx context.Context, characterID string) ([]MonsterInstance, error)
	ListByCharacterIDAndLocation(ctx context.Context, characterID, location string) ([]MonsterInstance, error)
	FindByID(ctx context.Context, id string) (MonsterInstance, error)
	FindByIDForUpdate(ctx context.Context, id string) (MonsterInstance, error)
	Save(ctx context.Context, monster MonsterInstance) error
	Delete(ctx context.Context, id string) error
	CountByLocation(ctx context.Context, characterID, location string) (int, error)
}

// TransactionProvider executes functions within atomic database transactions.
type TransactionProvider interface {
	RunInTx(ctx context.Context, fn func(txCtx context.Context) error) error
}

type Service struct {
	characters CharacterRepository
	monsters   MonsterRepository
	txProvider TransactionProvider
}

type Option func(*Service)

func WithTransactionProvider(tp TransactionProvider) Option {
	return func(s *Service) {
		s.txProvider = tp
	}
}

func NewService(characters CharacterRepository, monsters MonsterRepository, opts ...Option) *Service {
	s := &Service{
		characters: characters,
		monsters:   monsters,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// BoxCapacityForCharacter computes maximum monster box capacity based on OverMonster limit break.
func BoxCapacityForCharacter(char corecharacter.Character) int {
	tier := char.OverMonster
	if tier < 0 {
		tier = 0
	}
	if tier > 5 {
		tier = 5
	}
	return BaseBoxCapacity + (tier * OverBoxCapacityPerTier)
}

// ValidateMonsterName ensures the custom pet name conforms to legacy restrictions.
func ValidateMonsterName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrInvalidName
	}
	if utf8.RuneCountInString(name) > MaxMonsterNameLength {
		return ErrNameTooLong
	}
	for _, r := range name {
		if unicode.IsSpace(r) || r == '\u3000' || strings.ContainsRune(",;\"'&<>@", r) {
			return ErrInvalidName
		}
	}
	return nil
}

// GetSummary retrieves current box and home monster statistics.
func (s *Service) GetSummary(ctx context.Context, characterID, locationFilter string) (MonsterBoxSummary, error) {
	char, err := s.characters.FindByID(ctx, characterID)
	if err != nil {
		if errors.Is(err, corecharacter.ErrNotFound) {
			return MonsterBoxSummary{}, ErrCharacterNotFound
		}
		return MonsterBoxSummary{}, err
	}

	boxCount, err := s.monsters.CountByLocation(ctx, characterID, LocationBox)
	if err != nil {
		return MonsterBoxSummary{}, err
	}

	homeCount, err := s.monsters.CountByLocation(ctx, characterID, LocationHome)
	if err != nil {
		return MonsterBoxSummary{}, err
	}

	var instances []MonsterInstance
	if locationFilter != "" {
		if locationFilter != LocationBox && locationFilter != LocationHome {
			return MonsterBoxSummary{}, ErrInvalidLocation
		}
		instances, err = s.monsters.ListByCharacterIDAndLocation(ctx, characterID, locationFilter)
	} else {
		instances, err = s.monsters.ListByCharacterID(ctx, characterID)
	}
	if err != nil {
		return MonsterBoxSummary{}, err
	}

	return MonsterBoxSummary{
		BoxCount:     boxCount,
		BoxCapacity:  BoxCapacityForCharacter(char),
		HomeCount:    homeCount,
		HomeCapacity: MaxHomePets,
		Monsters:     instances,
	}, nil
}

// TameMonster captures/befriends a monster into the character's box.
func (s *Service) TameMonster(ctx context.Context, characterID, monsterID, customName string) (MonsterInstance, error) {
	monsterID = strings.TrimSpace(monsterID)
	if monsterID == "" {
		return MonsterInstance{}, errors.New("monster_id cannot be empty")
	}

	customName = strings.TrimSpace(customName)
	if customName == "" {
		customName = monsterID
	}
	if err := ValidateMonsterName(customName); err != nil {
		return MonsterInstance{}, err
	}

	var created MonsterInstance
	err := s.runInTx(ctx, func(txCtx context.Context) error {
		char, err := s.characters.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			if errors.Is(err, corecharacter.ErrNotFound) {
				return ErrCharacterNotFound
			}
			return err
		}

		boxCount, err := s.monsters.CountByLocation(txCtx, characterID, LocationBox)
		if err != nil {
			return err
		}

		if boxCount >= BoxCapacityForCharacter(char) {
			return ErrBoxFull
		}

		now := time.Now().UTC()
		created = MonsterInstance{
			ID:          id.New(),
			CharacterID: characterID,
			MonsterID:   monsterID,
			CustomName:  customName,
			Location:    LocationBox,
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		return s.monsters.Save(txCtx, created)
	})

	if err != nil {
		return MonsterInstance{}, err
	}
	return created, nil
}

// BringToHome transfers a monster from Monster Grandpa's box into the player's home as a pet.
func (s *Service) BringToHome(ctx context.Context, characterID, instanceID string) (MonsterInstance, error) {
	var updated MonsterInstance
	err := s.runInTx(ctx, func(txCtx context.Context) error {
		_, err := s.characters.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			if errors.Is(err, corecharacter.ErrNotFound) {
				return ErrCharacterNotFound
			}
			return err
		}

		inst, err := s.monsters.FindByIDForUpdate(txCtx, instanceID)
		if err != nil {
			return err
		}
		if inst.CharacterID != characterID {
			return ErrForbidden
		}
		if inst.Location == LocationHome {
			return nil // already at home
		}

		homeMonsters, err := s.monsters.ListByCharacterIDAndLocation(txCtx, characterID, LocationHome)
		if err != nil {
			return err
		}
		if len(homeMonsters) >= MaxHomePets {
			return ErrHomeFull
		}

		for _, m := range homeMonsters {
			if m.CustomName == inst.CustomName {
				return ErrDuplicatePetNameAtHome
			}
		}

		inst.Location = LocationHome
		inst.UpdatedAt = time.Now().UTC()
		if err := s.monsters.Save(txCtx, inst); err != nil {
			return err
		}
		updated = inst
		return nil
	})

	if err != nil {
		return MonsterInstance{}, err
	}
	return updated, nil
}

// DepositToBox deposits a home pet back to Monster Grandpa storage box.
func (s *Service) DepositToBox(ctx context.Context, characterID, instanceID string) (MonsterInstance, error) {
	var updated MonsterInstance
	err := s.runInTx(ctx, func(txCtx context.Context) error {
		char, err := s.characters.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			if errors.Is(err, corecharacter.ErrNotFound) {
				return ErrCharacterNotFound
			}
			return err
		}

		inst, err := s.monsters.FindByIDForUpdate(txCtx, instanceID)
		if err != nil {
			return err
		}
		if inst.CharacterID != characterID {
			return ErrForbidden
		}
		if inst.Location == LocationBox {
			return nil // already in box
		}

		boxCount, err := s.monsters.CountByLocation(txCtx, characterID, LocationBox)
		if err != nil {
			return err
		}
		if boxCount >= BoxCapacityForCharacter(char) {
			return ErrBoxFull
		}

		inst.Location = LocationBox
		inst.UpdatedAt = time.Now().UTC()
		if err := s.monsters.Save(txCtx, inst); err != nil {
			return err
		}
		updated = inst
		return nil
	})

	if err != nil {
		return MonsterInstance{}, err
	}
	return updated, nil
}

// Rename updates a monster's custom nickname.
func (s *Service) Rename(ctx context.Context, characterID, instanceID, newName string) (MonsterInstance, error) {
	if err := ValidateMonsterName(newName); err != nil {
		return MonsterInstance{}, err
	}

	var updated MonsterInstance
	err := s.runInTx(ctx, func(txCtx context.Context) error {
		_, err := s.characters.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			if errors.Is(err, corecharacter.ErrNotFound) {
				return ErrCharacterNotFound
			}
			return err
		}

		inst, err := s.monsters.FindByIDForUpdate(txCtx, instanceID)
		if err != nil {
			return err
		}
		if inst.CharacterID != characterID {
			return ErrForbidden
		}

		if inst.Location == LocationHome {
			homeMonsters, err := s.monsters.ListByCharacterIDAndLocation(txCtx, characterID, LocationHome)
			if err != nil {
				return err
			}
			for _, m := range homeMonsters {
				if m.ID != inst.ID && m.CustomName == newName {
					return ErrDuplicatePetNameAtHome
				}
			}
		}

		inst.CustomName = newName
		inst.UpdatedAt = time.Now().UTC()
		if err := s.monsters.Save(txCtx, inst); err != nil {
			return err
		}
		updated = inst
		return nil
	})

	if err != nil {
		return MonsterInstance{}, err
	}
	return updated, nil
}

// SendMonster gifts a monster to another character's storage box.
func (s *Service) SendMonster(ctx context.Context, senderCharID, recipientCharID, instanceID string) error {
	senderCharID = strings.TrimSpace(senderCharID)
	recipientCharID = strings.TrimSpace(recipientCharID)
	if senderCharID == "" || recipientCharID == "" {
		return errors.New("sender and recipient IDs cannot be empty")
	}
	if senderCharID == recipientCharID {
		return ErrCannotSendToSelf
	}

	return s.runInTx(ctx, func(txCtx context.Context) error {
		// Acquire character locks in deterministic order
		firstID, secondID := id.Sort2(senderCharID, recipientCharID)

		firstChar, err := s.characters.FindByIDForUpdate(txCtx, firstID)
		if err != nil {
			if errors.Is(err, corecharacter.ErrNotFound) {
				if firstID == recipientCharID {
					return ErrRecipientNotFound
				}
				return ErrCharacterNotFound
			}
			return err
		}

		secondChar, err := s.characters.FindByIDForUpdate(txCtx, secondID)
		if err != nil {
			if errors.Is(err, corecharacter.ErrNotFound) {
				if secondID == recipientCharID {
					return ErrRecipientNotFound
				}
				return ErrCharacterNotFound
			}
			return err
		}

		var recipientChar corecharacter.Character
		if firstChar.ID == recipientCharID {
			recipientChar = firstChar
		} else {
			recipientChar = secondChar
		}

		inst, err := s.monsters.FindByIDForUpdate(txCtx, instanceID)
		if err != nil {
			return err
		}
		if inst.CharacterID != senderCharID {
			return ErrForbidden
		}

		recipientBoxCount, err := s.monsters.CountByLocation(txCtx, recipientCharID, LocationBox)
		if err != nil {
			return err
		}
		if recipientBoxCount >= BoxCapacityForCharacter(recipientChar) {
			return ErrRecipientBoxFull
		}

		inst.CharacterID = recipientCharID
		inst.Location = LocationBox
		inst.UpdatedAt = time.Now().UTC()
		return s.monsters.Save(txCtx, inst)
	})
}

// ReleaseMonster releases a monster back into the wild.
func (s *Service) ReleaseMonster(ctx context.Context, characterID, instanceID string) error {
	return s.runInTx(ctx, func(txCtx context.Context) error {
		_, err := s.characters.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			if errors.Is(err, corecharacter.ErrNotFound) {
				return ErrCharacterNotFound
			}
			return err
		}

		inst, err := s.monsters.FindByIDForUpdate(txCtx, instanceID)
		if err != nil {
			return err
		}
		if inst.CharacterID != characterID {
			return ErrForbidden
		}

		return s.monsters.Delete(txCtx, instanceID)
	})
}

// GetDialogue returns NPC @モンジィ dialogue.
func (s *Service) GetDialogue() Dialogue {
	return Dialogue{
		NPCName:  "@モンジィ",
		Title:    "モンスターじいさん",
		Greeting: "わしが有名な@モンジィじゃ。モンスターのことなら何でも聞いてくれい",
		Phrases: []string{
			"わしが有名な@モンジィじゃ。モンスターのことなら何でも聞いてくれい",
			"何度かモンスターを倒していると、なついてくるモンスターがいるのじゃ",
			"人間を好むモンスターもいるということじゃ",
			"純粋な強さにモンスターはひきつけられるのじゃ",
			fmt.Sprintf("自分の家には%d匹までペットを連れて行くことができるぞい", MaxHomePets),
			"モンスターは最大50匹（限界突破で最大300匹）まで預かっておけるぞい。それ以上は、残念じゃが＠わかれるしかないのぉ…",
			"モンスター預かり所がまんぱんの状態だと、モンスターは仲間にならんから注意じゃ",
			"自分が相手より強い方が仲間になりやすいぞい",
			"ふがふがふがふがふがふがふが",
		},
	}
}

func (s *Service) runInTx(ctx context.Context, fn func(txCtx context.Context) error) error {
	if s.txProvider != nil {
		return s.txProvider.RunInTx(ctx, fn)
	}
	return fn(ctx)
}
