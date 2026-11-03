package adventure

import (
	"math"
	"testing"
)

func TestTryAggregateEarnings(t *testing.T) {
	e, err := addRunEarnings(RunEarnings{EXP: 1, Gold: 3}, RunEarnings{EXP: 1, Gold: 2, Crystals: 1})
	if err != nil {
		t.Fatal(err)
	}
	e, err = scaleTryEarnings(e)
	if err != nil || e != (RunEarnings{EXP: 3, Gold: 7, Crystals: 1}) {
		t.Fatal(e, err)
	}
	if _, err := scaleTryEarnings(RunEarnings{Gold: math.MaxInt}); err == nil {
		t.Fatal("overflow accepted")
	}
	if _, err := addRunEarnings(RunEarnings{}, RunEarnings{Gold: -1}); err == nil {
		t.Fatal("negative earnings accepted")
	}
}
