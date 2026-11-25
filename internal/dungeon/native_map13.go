package dungeon

import (
	"encoding/json"
	"fmt"
	"math/bits"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

// nativeMap13ID is the channel puzzle (legacy map/13). It stays unlisted and
// start-rejected until its destination events land (#1774).
const nativeMap13ID = "native-map-13"

// nativeMap13Floors transcribe map/13/1.cgi, _1.cgi and _EX.cgi. Destination
// event cells are passages until their event units land.
var nativeMap13Floors = [3][]string{
	{"111111111", "111101111", "11R000W11", "110000011", "1000C0001", "110000011", "11B0A0Y11", "1111S1111", "111111111"},
	{"1111h1111", "111feg111", "111cdb111", "1111V1111", "1111Q1111", "1111Z1111", "111101111", "111111111", "111111111"},
	{"1111h1111", "111feg111", "111cdb111", "1111V1111", "1111Q1111", "111101111", "111101111", "111111111", "111111111"},
}

const (
	map13Normal = 2
	map13EX     = 3
)

// map13Channels are the channel bits in @color's column order (B, Y, R, W),
// so @color index i is the mask i+1 (1.cgi:21–38).
var map13Channels = [4]struct {
	cell byte
	bit  uint8
	name string
	// color names the legacy star color; the projection has no color.
	color string
}{{'B', 8, "blue", "青"}, {'Y', 4, "yellow", "黄"}, {'R', 2, "red", "赤"}, {'W', 1, "white", "白"}}

func map13Bit(cell byte) uint8 {
	for _, c := range map13Channels {
		if c.cell == cell {
			return c.bit
		}
	}
	return 0
}

// map13State is the main floor's puzzle. The selected route is the run's
// map variant, and nothing changes after the transfer because no path returns.
type map13State struct {
	Channels  uint8 `json:"channels,omitempty"`
	Target    uint8 `json:"target,omitempty"`
	Activated bool  `json:"activated,omitempty"`
	// Disclosed is the target the altar last showed; Rerolled marks a reroll since.
	Disclosed uint8 `json:"disclosed,omitempty"`
	Rerolled  bool  `json:"rerolled,omitempty"`
}

func decodeMap13(raw json.RawMessage) (map13State, error) {
	var st map13State
	if len(raw) == 0 {
		return st, nil
	}
	if err := json.Unmarshal(raw, &st); err != nil {
		return st, fmt.Errorf("dungeon: map13 state: %w", err)
	}
	return st, nil
}

func (st map13State) encode() json.RawMessage {
	raw, err := json.Marshal(st)
	if err != nil {
		panic(err) // plain fields always marshal
	}
	return raw
}

// drawMap13Target is int(rand(15)) over the nonempty combinations.
func drawMap13Target(rng RandomSource) uint8 { return uint8(rng.Intn(15)) + 1 }

// map13Channel toggles its channel with no draw and no once flag (1.cgi:59–62).
type map13Channel struct{}

func (map13Channel) apply(n *nativeStep) error {
	st, err := decodeMap13(n.exp.MapState)
	if err != nil {
		return err
	}
	st.Channels ^= map13Bit(n.cell)
	n.step.MapState = st.encode()
	return nil
}

// mark shows an active channel by its color (1.cgi:41–44).
func (map13Channel) mark(exp *ActiveExpedition, cell byte) (string, error) {
	st, err := decodeMap13(exp.MapState)
	if err != nil || st.Channels&map13Bit(cell) == 0 {
		return "☆", err
	}
	for _, c := range map13Channels {
		if c.cell == cell {
			return c.color, nil
		}
	}
	return "☆", nil
}

// map13Reroll always redraws the target (1.cgi:63). Before activation the
// altar's first draw supersedes it, because the last digit run wins (1.cgi:48–49).
type map13Reroll struct{}

func (map13Reroll) apply(n *nativeStep) error {
	st, err := decodeMap13(n.exp.MapState)
	if err != nil {
		return err
	}
	st.Target, st.Rerolled = drawMap13Target(n.rng), st.Disclosed != 0
	n.step.MapState = st.encode()
	return nil
}

// map13Altar activates on its first visit, then compares in the same step: a
// mismatch discloses the target, a match transfers at the same coordinate
// (1.cgi:47–58). Its removal and return branches are unreachable because no
// path leads back.
type map13Altar struct{}

func (map13Altar) apply(n *nativeStep) error {
	st, err := decodeMap13(n.exp.MapState)
	if err != nil {
		return err
	}
	if !st.Activated {
		st.Activated, st.Target = true, drawMap13Target(n.rng)
	}
	if st.Channels != st.Target {
		st.Disclosed, st.Rerolled = st.Target, false
	} else {
		n.step.MapVariant = map13Route(bits.OnesCount8(st.Channels), n.partySize(), n.rng)
		n.step.RoundLimit = n.dungeon.limitOf(n.dungeon.Variants[n.step.MapVariant-1])
	}
	n.step.MapState = st.encode()
	return nil
}

// map13Route is _is_EX for k active channels and an n-member roster, fallen
// members included: EX when n ≥ 5 or k = 4, else with k ≥ rand(5) for n ≥ 2
// (1.cgi:68–83). n ≥ 5 needs allied NPC members, which native rosters lack.
func map13Route(k, n int, rng RandomSource) int {
	if n >= 5 || k == 4 || n >= 2 && float64(k) >= 5*rng.Float64() {
		return map13EX
	}
	return map13Normal
}

// map13Disclosure projects the disclosed target on the main floor only.
func map13Disclosure(exp *ActiveExpedition) (*ChannelTarget, error) {
	if exp.MapVariant != 1 {
		return nil, nil
	}
	st, err := decodeMap13(exp.MapState)
	if err != nil || st.Disclosed == 0 {
		return nil, err
	}
	out := &ChannelTarget{Channels: []string{}, RerolledSince: st.Rerolled}
	for _, c := range map13Channels {
		if st.Disclosed&c.bit != 0 {
			out.Channels = append(out.Channels, c.name)
		}
	}
	return out, nil
}

// nm13Pools are _data.cgi (main floor and _1) and _data_EX.cgi. Their boss
// rosters belong to the destination event units.
var nm13Pools = [2]EncounterPool{
	{
		Monsters: []DungeonMonster{
			{ID: "nm13-magic-doll", Name: "マジックドール", HP: 260, Attack: 220, Defense: 140, Agility: 250, ExpReward: 111, GoldReward: 88,
				combat: enemyCombat(corebattle.LifetimeActor{
					Participant: corebattle.Participant{MP: 200, ItemDefinitionIDs: []string{"item-143"}},
					Jobs:        corebattle.BattleJobs{JobID: "job-15", SP: 999, OldJobID: "job-06", OldSP: 999},
				}),
			},
			{ID: "nm13-dark-doll", Name: "ダークドール", HP: 300, Attack: 250, Defense: 220, Agility: 300, ExpReward: 150, GoldReward: 96,
				combat: enemyCombat(corebattle.LifetimeActor{
					Participant: corebattle.Participant{MP: 250, CMP: 300, ItemDefinitionIDs: []string{"item-143"}},
					Jobs:        corebattle.BattleJobs{JobID: "job-75", SP: 999, OldJobID: "job-15", OldSP: 999},
					NPCLoadout: &corebattle.NPCLoadout{Source: "ff97fc5:map/13/_data.cgi:98", CustomName: "custom-gem_magic_atk_sky-gem_inflict_confusion",
						Gems: [3]string{"gem_magic_atk_sky", "gem_inflict_confusion"}, GemCosts: [3]int{26, 6}},
				}),
			},
			{ID: "nm13-empusa", Name: "エンプーサ", HP: 230, Attack: 270, Defense: 100, Agility: 320, ExpReward: 103, GoldReward: 77,
				combat: enemyCombat(corebattle.LifetimeActor{
					Participant: corebattle.Participant{MP: 200},
					Jobs:        corebattle.BattleJobs{JobID: "job-08", SP: 24, OldJobID: "job-79", OldSP: 999},
				}),
			},
			{ID: "nm13-gargoyle", Name: "ガーゴイル", HP: 400, Attack: 300, Defense: 70, Agility: 410, ExpReward: 194, GoldReward: 4,
				combat: enemyCombat(corebattle.LifetimeActor{
					Participant: corebattle.Participant{MP: 300},
					Jobs:        corebattle.BattleJobs{JobID: "job-56", SP: 999, OldJobID: "job-36", OldSP: 999},
				}),
			},
			{ID: "nm13-cursed-book", Name: "呪いの書", HP: 280, Attack: 230, Defense: 270, Agility: 100, ExpReward: 121, GoldReward: 500,
				combat: enemyCombat(corebattle.LifetimeActor{
					Participant: corebattle.Participant{MP: 400, CMP: 200, ItemDefinitionIDs: []string{"item-143"}},
					Jobs:        corebattle.BattleJobs{JobID: "job-29", SP: 999},
					Temporary:   corebattle.TemporaryReflectAttack,
					NPCLoadout: &corebattle.NPCLoadout{Source: "ff97fc5:map/13/_data.cgi:152", CustomName: "custom-gem_inflict_seal_atk-gem_inflict_seal_magic-gem_inflict_seal_act",
						Gems: [3]string{"gem_inflict_seal_atk", "gem_inflict_seal_magic", "gem_inflict_seal_act"}, GemCosts: [3]int{6, 6, 6}},
				}),
			},
			nm13Envoy,
			{ID: "nm13-skull-crow", Name: "スカルクロウ", HP: 240, Attack: 180, Defense: 280, Agility: 60, ExpReward: 99, GoldReward: 68,
				combat: enemyCombat(corebattle.LifetimeActor{
					Participant: corebattle.Participant{MP: 370, CMP: 10},
					Jobs:        corebattle.BattleJobs{JobID: "job-40", SP: 999, OldJobID: "job-39", OldSP: 999},
					Temporary:   corebattle.TemporaryNullBreath,
					NPCLoadout: &corebattle.NPCLoadout{Source: "ff97fc5:map/13/_data.cgi:192", CustomName: "custom-gem_kamikaze_sky",
						Gems: [3]string{"gem_kamikaze_sky"}, GemCosts: [3]int{9}},
				}),
			},
			{ID: "nm13-crocter", Name: "クロクター", HP: 450, Attack: 200, Defense: 350, Agility: 190, ExpReward: 147, GoldReward: 200,
				combat: enemyCombat(corebattle.LifetimeActor{
					Participant: corebattle.Participant{MP: 680, ItemDefinitionIDs: []string{"item-158"}},
					Jobs:        corebattle.BattleJobs{JobID: "job-29", SP: 999, OldJobID: "job-30", OldSP: 999},
					Temporary:   corebattle.TemporaryNullMagic,
				}),
			},
		},
		Appears: []int{0, 0, 0, 0, 1, 1, 2, 2, 2, 3, 3, 3, 4, 4, 5, 6, 6, 6, 6, 7, 7},
	},
	{Monsters: []DungeonMonster{nm13Envoy}},
}

var nm13Envoy = DungeonMonster{ID: "nm13-phantom-envoy", Name: "幻魔のつかい", HP: 550, Attack: 350, Defense: 380, Agility: 200, ExpReward: 444, GoldReward: 4,
	combat: enemyCombat(corebattle.LifetimeActor{
		Participant: corebattle.Participant{MP: 500, ItemDefinitionIDs: []string{"item-144"}},
		Jobs:        corebattle.BattleJobs{JobID: "job-404", SP: 999, OldJobID: "job-53", OldSP: 120},
		Temporary:   corebattle.TemporaryNullAttack,
	}),
}

// nativeMap13 has round limit 80 on every floor and the leader job level 80
// of @ex_dungeon. The main floor overrides base B, so it has no boss or
// treasure; its passages spawn from _data.cgi.
func nativeMap13() Dungeon {
	main := Floor{Pools: nm13Pools[:1], Marks: map[byte]string{'A': "★", 'C': "灰"},
		events: map[byte]mapEvent{'A': {action: map13Altar{}}, 'C': {action: map13Reroll{}}}}
	for _, c := range map13Channels {
		main.events[c.cell] = mapEvent{action: map13Channel{}}
	}
	floors := [3]Floor{main, {FloorNumber: 2, Pools: nm13Pools[:1]}, {FloorNumber: 2, Pools: nm13Pools[1:]}}
	variants := make([]Floor, len(floors))
	for i, f := range floors {
		variants[i] = gridFloor(f, nativeMap13Floors[i])
	}
	return Dungeon{
		ID:             nativeMap13ID,
		Name:           "幻魔の広間",
		Description:    "四色の魔法陣が光る広間。祭壇の示す色に魔法陣をそろえると、光が先へ導く。",
		RoundLimit:     80,
		LeaderJobLevel: 80,
		Variants:       variants,
		Initial:        1,
		disclosure:     map13Disclosure,
	}
}
