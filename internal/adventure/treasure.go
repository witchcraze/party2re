package adventure

import (
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
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

// TreasureCalculationInput specifies the party state when arriving at Floor 11 (Treasure Room).
type TreasureCalculationInput struct {
	TripleTreasure      bool // StageTraits.TripleTreasure
	AliveMembers        int
	DeadMembers         int // Number of dead party members (legacy add_treasure: --$count if hp <= 0)
	TotalMembers        int // Total party members; if 0, defaults to AliveMembers + DeadMembers
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

	total := input.TotalMembers
	if total <= 0 {
		total = input.AliveMembers + input.DeadMembers
	}
	if total < input.AliveMembers {
		total = input.AliveMembers
	}
	dead := input.DeadMembers
	if dead <= 0 && total > input.AliveMembers {
		dead = total - input.AliveMembers
	}
	if dead < 0 {
		dead = 0
	}

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

func (s *CrawlSession) spawnTreasureBoxes() {
	aliveCount := 0
	deadCount := 0

	for _, c := range s.Characters {
		isAlive := true
		for _, p := range s.Participants {
			if p.ID == c.ID && p.HP <= 0 {
				isAlive = false
				break
			}
		}
		if isAlive {
			aliveCount++
		} else {
			deadCount++
		}
	}

	// Identify the acting player (操作主体: session leader, matching legacy %m in vs_monster.cgi)
	var leader *corecharacter.Character
	for i := range s.Characters {
		if s.Characters[i].ID == s.LeaderID {
			leader = &s.Characters[i]
			break
		}
	}
	if leader == nil && len(s.Characters) > 0 {
		leader = &s.Characters[0]
	}

	var hasMerchant, hasGambler, hasTreasureHunter, hasLuckyPendant bool
	if leader != nil {
		hasMerchant, hasGambler, hasTreasureHunter = leader.JobID == merchantJob, leader.JobID == gamblerJob, leader.JobID == treasureHunterJob
		for _, p := range s.Participants {
			if p.ID == leader.ID {
				hasLuckyPendant = ParticipantHasLuckyPendant(p)
				break
			}
		}
	}

	boxCount := CalculateTreasureCount(TreasureCalculationInput{
		TripleTreasure:      s.Stage.TripleTreasure,
		AliveMembers:        aliveCount,
		DeadMembers:         deadCount,
		TotalMembers:        len(s.Characters),
		HasMerchant:         hasMerchant,
		HasGambler:          hasGambler,
		HasTreasureHunter:   hasTreasureHunter,
		HasLuckyPendant:     hasLuckyPendant,
		HasTreasureBlessing: s.HasTreasureBlessing,
		Rng:                 s.Rng,
	})

	pools := runhost.TreasurePools{Weapons: s.Stage.TreasureWeapons, Armors: s.Stage.TreasureArmors, Items: s.Stage.TreasureItems}
	s.TreasureBoxes = runhost.GenerateTreasureBoxes(boxCount, pools, s.now(), s.Rng)
	if s.Stage.UnsealsOnTreasure {
		s.UnsealDemonKing = true
	}
}
