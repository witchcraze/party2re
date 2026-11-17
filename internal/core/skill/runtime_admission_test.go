package skill

import (
	"encoding/json"
	"errors"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

type constantDraws float64

func (r constantDraws) Float64() float64 { return float64(r) }
func (r constantDraws) Intn(n int) int   { return int(float64(r) * float64(n)) }
func (r constantDraws) IntN(n int) int   { return r.Intn(n) }

// assertAtomicAfterUnknown is the contract for slots after an unknown
// predecessor: admission checks registration only, and execution either
// completes or rejects with the caller's state and context restored.
func assertAtomicAfterUnknown(t *testing.T, s *corebattle.LifetimeState, f *corebattle.CallbackContext, d BattleDefinitions, ids ...string) error {
	t.Helper()
	ops, err := d.InvocationAction(s, f, ids, noArithmeticDraws{t})
	if err != nil {
		t.Fatal("unknown predecessor rejected at admission", ids, err)
	}
	snapshot := func() string {
		raw, err := json.Marshal([]any{s, f.TargetContext, f.Arguments, f.ArithmeticDivisor, f.Damage, f.Status, f.Defeat, f.Assets})
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	before := snapshot()
	err = s.ApplyInvocations(f, d.Invocations, ops, constantDraws(.5))
	if err != nil {
		if !errors.Is(err, corebattle.ErrUnsupportedEffect) && !errors.Is(err, corebattle.ErrInvalidParticipant) {
			t.Fatal(ids, err)
		}
		if after := snapshot(); after != before {
			t.Fatal("rejected execution was not restored", ids, after)
		}
	}
	return err
}
