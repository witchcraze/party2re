package adventure

import (
	"math"
	"testing"

	"github.com/witchcraze/party2re/internal/runhost"
)

func TestTryAggregateEarnings(t *testing.T) {
	e, err := runhost.AddEarnings(runhost.Earnings{EXP: 1, Gold: 3}, runhost.Earnings{EXP: 1, Gold: 2, Crystals: 1})
	if err != nil {
		t.Fatal(err)
	}
	e, err = scaleTryEarnings(e)
	if err != nil || e != (runhost.Earnings{EXP: 3, Gold: 7, Crystals: 1}) {
		t.Fatal(e, err)
	}
	if _, err := scaleTryEarnings(runhost.Earnings{Gold: math.MaxInt}); err == nil {
		t.Fatal("overflow accepted")
	}
	if _, err := runhost.AddEarnings(runhost.Earnings{}, runhost.Earnings{Gold: -1}); err == nil {
		t.Fatal("negative earnings accepted")
	}
}
