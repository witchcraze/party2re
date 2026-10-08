package playercontext_test

import (
	"testing"

	"github.com/witchcraze/party2re/internal/playercontext"
)

func TestDefaultCatalog_Integrity(t *testing.T) {
	actions := playercontext.AllActions()

	if len(actions) == 0 {
		t.Fatal("action catalog must not be empty")
	}

	seenIDs := make(map[string]bool)
	seenOperationIDs := make(map[string]bool)

	for _, act := range actions {
		if act.ID == "" {
			t.Errorf("action has empty ID: %+v", act)
		}
		if seenIDs[act.ID] {
			t.Errorf("duplicate action ID: %s", act.ID)
		}
		seenIDs[act.ID] = true

		if act.Label == "" {
			t.Errorf("action %s has empty Label", act.ID)
		}
		if act.Category == "" {
			t.Errorf("action %s has empty Category", act.ID)
		}
		if act.OperationID == "" {
			t.Errorf("action %s has empty OperationID", act.ID)
		}
		seenOperationIDs[act.OperationID] = true

		if act.RequiredGates == 0 && act.ID != "rescue_request" && act.ActivityKind == "" {
			t.Errorf("action %s has zero RequiredGates", act.ID)
		}
		if act.ID == "rescue_request" && act.RequiredGates != 0 {
			t.Error("emergency rescue must remain exempt from condition gates")
		}

		// Verify GetAction works for every item in catalog
		got, ok := playercontext.GetAction(act.ID)
		if !ok {
			t.Errorf("GetAction(%q) returned false, want true", act.ID)
		}
		if got.ID != act.ID {
			t.Errorf("GetAction(%q) ID = %q, want %q", act.ID, got.ID, act.ID)
		}
	}

	// Verify unknown action lookup
	_, ok := playercontext.GetAction("non_existent_action_xyz")
	if ok {
		t.Error("GetAction returned true for non-existent action ID")
	}
}

func TestGateFlags_Has(t *testing.T) {
	gates := playercontext.GateDeadCheck | playercontext.GateFatigueCheck | playercontext.GateSleepCheck

	if !gates.Has(playercontext.GateDeadCheck) {
		t.Error("expected GateDeadCheck to be present")
	}
	if !gates.Has(playercontext.GateFatigueCheck) {
		t.Error("expected GateFatigueCheck to be present")
	}
	if !gates.Has(playercontext.GateSleepCheck) {
		t.Error("expected GateSleepCheck to be present")
	}
	if gates.Has(playercontext.GateCurrencyCheck) {
		t.Error("did not expect GateCurrencyCheck to be present")
	}
	if gates.Has(playercontext.GateLocationCheck) {
		t.Error("did not expect GateLocationCheck to be present")
	}
	if gates.Has(playercontext.GateCooldownCheck) {
		t.Error("did not expect GateCooldownCheck to be present")
	}
}
