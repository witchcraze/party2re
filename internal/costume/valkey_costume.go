package costume

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/valkey-io/valkey-go"
)

//go:embed lua/release_with_claim.lua
var releaseWithClaimLua string

var releaseWithClaimScript = valkey.NewLuaScript(releaseWithClaimLua)

const (
	valkeyCostumeKeyPrefix = "party2:daily:costume:"
)

// ValkeyCostumeRepository stores active daily rented costumes in Valkey Master with midnight TTL.
type ValkeyCostumeRepository struct {
	client valkey.Client
}

// NewValkeyCostumeRepository creates a new Valkey-backed costume repository.
func NewValkeyCostumeRepository(client valkey.Client) *ValkeyCostumeRepository {
	return &ValkeyCostumeRepository{client: client}
}

// GetActiveCostume retrieves the active costume for a character from Valkey.
func (r *ValkeyCostumeRepository) GetActiveCostume(ctx context.Context, characterID string) (*ActiveCostume, error) {
	if r.client == nil {
		return nil, nil
	}

	key := valkeyCostumeKeyPrefix + characterID
	cmd := r.client.B().Get().Key(key).Build()
	resp := r.client.Do(ctx, cmd)
	if err := resp.Error(); err != nil {
		if errors.Is(err, valkey.Nil) {
			return nil, nil
		}
		return nil, err
	}

	raw, err := resp.AsBytes()
	if err != nil {
		return nil, err
	}

	var c ActiveCostume
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, err
	}
	if !c.IsActive(time.Now().UTC()) {
		return nil, nil
	}
	return &c, nil
}

// SaveActiveCostume stores active costume state in Valkey with a TTL.
func (r *ValkeyCostumeRepository) SaveActiveCostume(ctx context.Context, costume ActiveCostume, ttl time.Duration) error {
	if r.client == nil {
		return nil
	}

	raw, err := json.Marshal(costume)
	if err != nil {
		return err
	}

	key := valkeyCostumeKeyPrefix + costume.CharacterID
	seconds := int64(ttl.Seconds())
	if seconds <= 0 {
		seconds = 86400
	}

	cmd := r.client.B().Set().Key(key).Value(string(raw)).ExSeconds(seconds).Build()
	return r.client.Do(ctx, cmd).Error()
}

// ClearActiveCostume deletes active costume rental state from Valkey.
func (r *ValkeyCostumeRepository) ClearActiveCostume(ctx context.Context, characterID string) error {
	if r.client == nil {
		return nil
	}

	key := valkeyCostumeKeyPrefix + characterID
	cmd := r.client.B().Del().Key(key).Build()
	return r.client.Do(ctx, cmd).Error()
}

// ClearActiveCostumeWithClaim atomically deletes active costume rental state only if claimCategory has claimToken.
func (r *ValkeyCostumeRepository) ClearActiveCostumeWithClaim(ctx context.Context, characterID, claimCategory, claimToken string) (bool, error) {
	if r.client == nil {
		return false, nil
	}
	claimToken = strings.TrimSpace(claimToken)
	if claimToken == "" {
		return false, errors.New("claim token cannot be empty")
	}
	characterID = strings.TrimSpace(characterID)
	if characterID == "" {
		return false, errors.New("character id cannot be empty")
	}
	claimCategory = strings.TrimSpace(claimCategory)
	if claimCategory == "" {
		return false, errors.New("claim category cannot be empty")
	}

	claimK := "party2:timer:" + claimCategory + ":" + characterID
	costumeK := valkeyCostumeKeyPrefix + characterID

	res := releaseWithClaimScript.Exec(ctx, r.client, []string{claimK, costumeK}, []string{claimToken})
	if res.Error() != nil {
		if valkey.IsValkeyNil(res.Error()) {
			return false, nil
		}
		return false, res.Error()
	}
	n, err := res.AsInt64()
	if err != nil {
		return false, err
	}
	return n == 1, nil
}
