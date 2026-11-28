package adventure

import (
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
	"github.com/witchcraze/party2re/internal/runhost"
)

// Legacy item IDs with special adventure interactions.
const (
	ItemLuckyPendant = "item-191" // ラッキーペンダント (1/3 chance +1 treasure box)
)

// Acting-player treasure bonus jobs (vs_monster.cgi:93–98).
const (
	merchantJob       = "job-07"
	gamblerJob        = "job-81"
	treasureHunterJob = "job-78"
)

// TreasureCalculationInput specifies the party state when creating Floor 11 (Treasure Room).
type TreasureCalculationInput struct {
	TripleTreasure      bool // StageTraits.TripleTreasure
	AliveMembers        int
	TotalMembers        int // legacy @partys, fallen members included
	HasMerchant         bool
	HasGambler          bool
	HasTreasureHunter   bool
	HasLuckyPendant     bool
	HasTreasureBlessing bool
	Rng                 func(n int) int
}

// CalculateTreasureCount calculates the number of treasure boxes spawned on Floor 11
// in 1:1 parity with legacy vs_monster.cgi ($boss_round+1) and _npc_action.cgi (add_treasure).
func CalculateTreasureCount(input TreasureCalculationInput) int {
	if input.AliveMembers <= 0 {
		return 0
	}
	rng := input.Rng
	if rng == nil {
		rng = random.Intn
	}
	total, dead := input.TotalMembers, input.TotalMembers-input.AliveMembers

	// Legacy vs_monster.cgi:92:
	//   my $count = $stage eq '17' || $stage eq '20' || $stage eq '21' ? ($#partys+1) * 3 : $#partys;
	// where ($#partys + 1) is total party members, and $#partys is (total - 1).
	var count int
	if input.TripleTreasure {
		count = total * 3
	} else {
		count = total - 1
	}

	// Merchant (job 7 / job-07): +1 box (vs_monster.cgi:93: ++$count if $m{job} eq '7')
	if input.HasMerchant {
		count++
	}

	// Gambler / Pirate (job 81 / job-81): + rand(3) - rand(2)
	// (party2/lib/vs_monster.cgi:94-97)
	// If Lucky Pendant (item-191) is held and count drops below $#partys (total - 1), restored to (total - 1).
	if input.HasGambler {
		count += rng(3) - rng(2)
		minCount := total - 1
		if input.HasLuckyPendant && count < minCount {
			count = minCount
		}
	}

	// Treasure Hunter (job 78 / job-78): +1 to +2 boxes (1 + rand(2)) (vs_monster.cgi:98)
	if input.HasTreasureHunter {
		count += 1 + rng(2)
	}

	// Lucky Pendant (item-191): 1/3 chance of +1 box (rand(3) < 1) (vs_monster.cgi:100)
	if input.HasLuckyPendant && rng(3) == 0 {
		count++
	}

	return runhost.TreasureCount(count, total, dead, input.HasTreasureBlessing, rng)
}

// ParticipantHasLuckyPendant reports whether a combat participant has equipped Lucky Pendant (item-191).
// In legacy Party2 ($m{ite} eq '191'), this checks the currently equipped accessory slot, not unequipped inventory.
func ParticipantHasLuckyPendant(p corebattle.Participant) bool {
	return p.Gear != nil && p.Gear.AccessoryID == ItemLuckyPendant
}
