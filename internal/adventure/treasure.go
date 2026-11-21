package adventure

import (
	"fmt"
	"slices"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/random"
)

// Legacy item IDs with special adventure interactions.
const (
	ItemLuckyPendant   = "item-191" // ラッキーペンダント (1/3 chance +1 treasure box)
	ItemTreasurePocket = "item-215" // トレジャーポケット (allows opening additional treasure boxes; _data.cgi:1913)
)

// Acting-player treasure bonus jobs (vs_monster.cgi:93–98).
const (
	merchantJob       = "job-07"
	gamblerJob        = "job-81"
	treasureHunterJob = "job-78"
)

var defaultBoxNames = []string{
	"普通の宝箱",
	"大きい宝箱",
	"小さい宝箱",
	"黒い宝箱",
	"青い宝箱",
	"古い宝箱",
	"丸い宝箱",
}

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

	// Legacy party2/lib/_npc_action.cgi:499 fallback:
	//   my $count = shift || $#partys;
	// In Perl, if $count passed from vs_monster.cgi evaluated to 0 (falsy), it falls back to $#partys (total - 1).
	if count == 0 {
		count = total - 1
	}

	// Chapel Blessing 4 ("宝箱がほしい"): 20% chance of +1 box (party2/lib/_npc_action.cgi:501, rand(5) < 1)
	if input.HasTreasureBlessing && rng(5) == 0 {
		count++
	}

	// Dead party members penalty (party2/lib/_npc_action.cgi:503-505):
	//   for my $name (@partys) { --$count if $ms{$name}{hp} <= 0; }
	count -= dead

	// Legacy box generation loop: for my $i (0 .. $count) (party2/lib/_npc_action.cgi:519)
	// In Perl, 0 .. $count is inclusive:
	// - If count < 0: produces 0 boxes (empty range)
	// - If count >= 0: produces count + 1 boxes
	if count < 0 {
		return 0
	}
	return count + 1
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

	pools := StageTreasurePools{Weapons: s.Stage.TreasureWeapons, Armors: s.Stage.TreasureArmors, Items: s.Stage.TreasureItems}
	s.TreasureBoxes = GenerateTreasureBoxesWithPools(boxCount, pools, s.now(), s.Rng)
	if s.Stage.UnsealsOnTreasure {
		s.UnsealDemonKing = true
	}
}

// IsTreasureBlessing reports Chapel Blessing 4 ("宝箱がほしい") as the chapel stores it.
func IsTreasureBlessing(blessing string) bool { return blessing == "DROP" }

// StageTreasurePools defines candidate item pools across equipment and consumables for Floor 11.
type StageTreasurePools struct {
	Weapons []string
	Armors  []string
	Items   []string
}

// GetDayOfWeekOrb returns the canonical weekday orb item ID matching legacy Party2 (_npc_action.cgi:541-543).
// Weekday is evaluated in JST (UTC+9) matching authentic server time.
// Sunday (0): random among item-060..item-065
// Monday (1) .. Saturday (6): item-060 .. item-065
func GetDayOfWeekOrb(t time.Time, rng func(n int) int) string {
	if rng == nil {
		rng = random.Intn
	}
	jst := time.FixedZone("JST", 9*60*60)
	wday := t.In(jst).Weekday()
	if wday == time.Sunday {
		return fmt.Sprintf("item-%03d", 60+rng(6))
	}
	return fmt.Sprintf("item-%03d", 60+int(wday)-1)
}

// TreasureBox represents an individual treasure chest discovered on Floor 11.
type TreasureBox struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	OpenedBy    string `json:"opened_by,omitempty"`
	ItemID      string `json:"item_id,omitempty"`
	ItemName    string `json:"item_name,omitempty"`
	DeliveredTo string `json:"delivered_to,omitempty"` // "inventory" or "depot"
	Fixed       bool   `json:"fixed,omitempty"`        // EX fixed-item chest (over_lv); never uses the ordinary allowance
}

// GenerateTreasureBoxes creates the list of treasure chests for Floor 11 (legacy convenience wrapper).
func GenerateTreasureBoxes(count int, dropPool []string, rng func(n int) int) []TreasureBox {
	return GenerateTreasureBoxesWithPools(count, StageTreasurePools{Items: dropPool}, time.Now().UTC(), rng)
}

// GenerateTreasureBoxesWithPools creates treasure chests with the legacy add_treasure draw:
// each chest first adds the weekday orb to the tool pool (so orbs accumulate),
// then picks weapon/armor/tool as 1/4, 1/4, 1/2, falling back to tools when the
// chosen pool is empty. An empty pool entry ("") yields an empty chest.
func GenerateTreasureBoxesWithPools(count int, pools StageTreasurePools, now time.Time, rng func(n int) int) []TreasureBox {
	if count <= 0 {
		return nil
	}
	if rng == nil {
		rng = random.Intn
	}

	items := slices.Clone(pools.Items)
	boxes := make([]TreasureBox, count)
	for i := 0; i < count; i++ {
		name := boxName(defaultBoxNames[rng(len(defaultBoxNames))], i)
		items = append(items, GetDayOfWeekOrb(now, rng))

		pool := items
		switch v := rng(4) + 1; {
		case v == 1 && len(pools.Weapons) > 0:
			pool = pools.Weapons
		case v == 2 && len(pools.Armors) > 0:
			pool = pools.Armors
		}

		boxes[i] = TreasureBox{
			ID:     fmt.Sprintf("box-%d", i+1),
			Name:   name,
			ItemID: pool[rng(len(pool))],
		}
	}
	return boxes
}

func boxName(base string, i int) string {
	suffix := rune('A' + (i % 26))
	if i >= 26 {
		suffix = rune('a' + ((i - 26) % 26))
	}
	return fmt.Sprintf("%s%c", base, suffix)
}
