package job

import (
	"errors"
	"testing"
)

func TestCharacterJobChangesAndRecordsHistory(t *testing.T) {
	state, err := NewCharacterJob("character-1", "starter")
	if err != nil {
		t.Fatal(err)
	}
	target, err := NewDefinition("vanguard", "Vanguard", 6, 1, 3, 5, 2, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := state.ChangeTo(target, 1, "unspecified"); err != nil {
		t.Fatal(err)
	}
	if state.CurrentJobID != "vanguard" || len(state.History) != 1 ||
		state.History[0] != (Change{FromJobID: "starter", ToJobID: "vanguard"}) {
		t.Fatalf("state = %#v", state)
	}
}

func TestCharacterJobRejectsUnmetRequirements(t *testing.T) {
	state, _ := NewCharacterJob("character-1", "starter")
	target, _ := NewDefinition("advanced", "Advanced", 3, 3, 3, 3, 3, 10, "female")
	if err := state.ChangeTo(target, 9, "male"); !errors.Is(err, ErrJobUnavailable) {
		t.Fatalf("ChangeTo() error = %v", err)
	}
}
