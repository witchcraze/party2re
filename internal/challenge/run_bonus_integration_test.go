package challenge_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/witchcraze/party2re/internal/challenge"
)

func TestRunProtocolRandomBonusReceiptIntegration(t *testing.T) {
	f := newProtocolFixture(t, 1)
	ctx := context.Background()
	actor := f.actors[0].ID
	if _, err := f.db.ExecContext(ctx, `UPDATE inventory_items SET definition_id='item-106' WHERE character_id=? AND definition_id='item-105'`, actor); err != nil {
		t.Fatal(err)
	}
	run := f.start(t)
	for i := 0; i < 3; i++ {
		run = f.advance(t, run)
	}
	loss := &commitLossTransitions{TransitionRepository: f.repo, lose: true}
	f.svc = f.service(t, f.store, loss)
	result, err := f.svc.ExitSession(ctx, actor, run.ID, run.Revision)
	if err != nil {
		t.Fatal(err)
	}
	bonus := result.Settled[0].ExitBonusGold
	got, err := f.chars.FindByID(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	if bonus < 0 || bonus >= 231 || got.Money != f.actors[0].Money+15+bonus || got.Experience != f.actors[0].Experience+60 {
		t.Fatalf("bonus/earnings: %+v %+v", result, got)
	}
	for i := 0; i < 3; i++ {
		retry, err := f.svc.ExitSession(ctx, actor, run.ID, run.Revision)
		if err != nil || !reflect.DeepEqual(result, retry) {
			t.Fatalf("random reroll: %+v %v", retry, err)
		}
	}
	if _, _, err := f.svc.AdvanceRound(ctx, actor, run.ID, run.Revision); !errors.Is(err, challenge.ErrRevisionConflict) {
		t.Fatalf("input conflict: %v", err)
	}
}

func TestRunProtocolConsumedBonusEquipmentIntegration(t *testing.T) {
	f := newProtocolFixture(t, 1)
	ctx := context.Background()
	f.engine.consumeBonus.Store(true)
	run := f.start(t)
	for i := 0; i < 3; i++ {
		run = f.advance(t, run)
	}
	result, err := f.svc.ExitSession(ctx, run.CharacterID, run.ID, run.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if result.Settled[0].ExitBonusExp != 0 {
		t.Fatal("consumed accessory granted bonus")
	}
	got, err := f.chars.FindByID(ctx, run.CharacterID)
	if err != nil || got.Experience != f.actors[0].Experience+60 {
		t.Fatalf("consumed bonus: %+v %v", got, err)
	}
	var count int
	if err := f.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM equipment_slots WHERE character_id=?`, run.CharacterID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("consumed equipment persisted: %d %v", count, err)
	}
}
