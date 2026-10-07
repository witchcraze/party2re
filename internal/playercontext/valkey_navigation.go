package playercontext

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"

	"github.com/valkey-io/valkey-go"
)

const navigationKeyPrefix = "party2:playercontext:navigation:"

// ValkeyNavigationRepository uses GET and atomic SET EX 604800 only.
type ValkeyNavigationRepository struct{ client valkey.Client }

func NewValkeyNavigationRepository(client valkey.Client) *ValkeyNavigationRepository {
	return &ValkeyNavigationRepository{client: client}
}

func (r *ValkeyNavigationRepository) Load(ctx context.Context, actorID string) (Selection, error) {
	if r.client == nil {
		return Selection{}, ErrNavigationNotConfigured
	}
	if !navigationID.MatchString(actorID) {
		return Selection{}, ErrInvalidSelection
	}
	raw, err := r.client.Do(ctx, r.client.B().Get().Key(navigationKeyPrefix+actorID).Build()).ToString()
	if errors.Is(err, valkey.Nil) {
		return Selection{}, nil
	}
	if err != nil {
		return Selection{}, err
	}
	if len(raw) > 2048 {
		return Selection{}, ErrInvalidSelection
	}
	var selection Selection
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&selection); err != nil {
		return Selection{}, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return Selection{}, ErrInvalidSelection
	}
	if !validSelection(selection) {
		return Selection{}, ErrInvalidSelection
	}
	return selection, nil
}

func (r *ValkeyNavigationRepository) Save(ctx context.Context, actorID string, selection Selection) error {
	if r.client == nil {
		return ErrNavigationNotConfigured
	}
	if !navigationID.MatchString(actorID) || !validSelection(selection) {
		return ErrInvalidSelection
	}
	raw, err := json.Marshal(selection)
	if err != nil {
		return err
	}
	return r.client.Do(ctx, r.client.B().Set().Key(navigationKeyPrefix+actorID).Value(string(raw)).ExSeconds(604800).Build()).Error()
}
