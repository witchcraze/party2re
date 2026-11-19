package dungeon_test

import (
	"context"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/dungeon"
)

// mapDef returns a listed native map.
func mapDef(t *testing.T, id string) dungeon.Dungeon {
	t.Helper()
	for _, d := range dungeon.DefaultDungeonCatalog() {
		if d.ID == id {
			return d
		}
	}
	t.Fatalf("%s missing from default catalog", id)
	return dungeon.Dungeon{}
}

// scriptedRNG replays fixed draws and counts every draw.
type scriptedRNG struct {
	ints       []int
	floats     []float64
	intCalls   int
	floatCalls int
}

func (r *scriptedRNG) Intn(n int) int {
	r.intCalls++
	if len(r.ints) == 0 || n <= 0 {
		return 0
	}
	v := r.ints[0]
	r.ints = r.ints[1:]
	return v % n
}

func (r *scriptedRNG) Float64() float64 {
	r.floatCalls++
	if len(r.floats) == 0 {
		return 0.99 // passage roll 1.98: no spawn
	}
	v := r.floats[0]
	r.floats = r.floats[1:]
	return v
}

func (r *scriptedRNG) calls() int { return r.intCalls + r.floatCalls }

type nativeFixture struct {
	svc   *dungeon.Service
	store dungeon.ActiveExpeditionStore
	repo  *mockDungeonRepo
	chars *mockCharRepo
	rng   *scriptedRNG
}

func newNativeFixture(t *testing.T, hero corecharacter.Character) *nativeFixture {
	t.Helper()
	return newNativeFixtureOn(t, dungeon.NewMemoryExpeditionRepository(), hero)
}

func newNativeFixtureOn(t *testing.T, store dungeon.ActiveExpeditionStore, hero corecharacter.Character, opts ...dungeon.Option) *nativeFixture {
	t.Helper()
	f := &nativeFixture{
		store: store,
		repo:  newMockDungeonRepo(),
		chars: &mockCharRepo{chars: map[string]corecharacter.Character{hero.ID: hero}},
		rng:   &scriptedRNG{},
	}
	opts = append([]dungeon.Option{dungeon.WithActiveExpeditionStore(f.store), dungeon.WithRNG(f.rng),
		dungeon.WithInventoryProvider(&mockInventoryRepo{}), dungeon.WithAdmission(approvedRoster{})}, opts...)
	svc, err := dungeon.NewService(f.repo, f.chars, corebattle.Engine{}, opts...)
	if err != nil {
		t.Fatal(err)
	}
	f.svc = svc
	return f
}

func (f *nativeFixture) reload(t *testing.T, id string) dungeon.ActiveExpedition {
	t.Helper()
	exp, err := f.svc.GetActiveExpedition(context.Background(), id)
	if err != nil || exp == nil {
		t.Fatalf("reload: %v %v", exp, err)
	}
	return *exp
}

func (f *nativeFixture) placeNative(t *testing.T, mapID string, id string, variant, x, y, round int) {
	t.Helper()
	f.rng.ints = []int{variant - 1}
	if _, err := f.svc.StartExpedition(context.Background(), id, mapID); err != nil {
		t.Fatal(err)
	}
	exp := f.reload(t, id)
	if exp.MapVariant != variant {
		t.Fatalf("variant %d, want %d", exp.MapVariant, variant)
	}
	exp.PosX, exp.PosY, exp.Round = x, y, round
	if err := f.store.SaveActiveExpedition(context.Background(), exp); err != nil {
		t.Fatal(err)
	}
	f.rng.ints, f.rng.intCalls, f.rng.floatCalls = nil, 0, 0
}

func (f *nativeFixture) moveTo(t *testing.T, id string, x, y int) {
	t.Helper()
	exp := f.reload(t, id)
	exp.PosX, exp.PosY = x, y
	if err := f.store.SaveActiveExpedition(context.Background(), exp); err != nil {
		t.Fatal(err)
	}
	f.rng.intCalls, f.rng.floatCalls = 0, 0
}

// setVitality overwrites one member's saved HP/MP.
func (f *nativeFixture) setVitality(t *testing.T, lead, member string, hp, mp int) {
	t.Helper()
	exp := f.reload(t, lead)
	for i := range exp.Members {
		if exp.Members[i].CharacterID == member {
			exp.Members[i].CurrentHP, exp.Members[i].CurrentMP = hp, mp
			if member == lead {
				exp.CurrentHP = hp
			}
		}
	}
	if err := f.store.SaveActiveExpedition(context.Background(), exp); err != nil {
		t.Fatal(err)
	}
}

func memberOf(t *testing.T, exp dungeon.ActiveExpedition, id string) dungeon.ExpeditionMember {
	t.Helper()
	for _, m := range exp.Members {
		if m.CharacterID == id {
			return m
		}
	}
	t.Fatalf("member %s missing", id)
	return dungeon.ExpeditionMember{}
}

func cellOf(fl dungeon.Floor, sym byte) [][2]int {
	var out [][2]int
	for y, row := range fl.Grid {
		for x := range len(row) {
			if row[x] == sym {
				out = append(out, [2]int{x, y})
			}
		}
	}
	return out
}

// approach returns an open neighbour of p and the direction that enters p.
func approach(fl dungeon.Floor, p [2]int) ([2]int, dungeon.Direction) {
	for _, c := range []struct {
		dx, dy int
		dir    dungeon.Direction
	}{{-1, 0, dungeon.DirectionEast}, {1, 0, dungeon.DirectionWest}, {0, -1, dungeon.DirectionSouth}, {0, 1, dungeon.DirectionNorth}} {
		x, y := p[0]+c.dx, p[1]+c.dy
		if x >= 0 && y >= 0 && x < fl.Width && y < fl.Height && fl.Grid[y][x] != '1' {
			return [2]int{x, y}, c.dir
		}
	}
	return [2]int{-1, -1}, ""
}

// reachable floods from start; the door is passable only when open.
func reachable(fl dungeon.Floor, open bool) map[[2]int]bool {
	seen := map[[2]int]bool{{fl.StartX, fl.StartY}: true}
	queue := [][2]int{{fl.StartX, fl.StartY}}
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			n := [2]int{p[0] + d[0], p[1] + d[1]}
			if n[0] < 0 || n[1] < 0 || n[0] >= fl.Width || n[1] >= fl.Height || seen[n] {
				continue
			}
			if c := fl.Grid[n[1]][n[0]]; c == '1' || (c == 'D' && !open) {
				continue
			}
			seen[n] = true
			queue = append(queue, n)
		}
	}
	return seen
}

// startNativeParty starts lead and ally on the given variant, then moves them.
func (f *nativeFixture) startNativeParty(t *testing.T, mapID string, variant, x, y int) {
	t.Helper()
	f.rng.ints = []int{variant - 1}
	if _, err := f.svc.StartPartyExpedition(context.Background(), "lead", []string{"lead", "ally"}, mapID, "p"); err != nil {
		t.Fatal(err)
	}
	f.moveTo(t, "lead", x, y)
	f.rng.ints, f.rng.intCalls, f.rng.floatCalls = nil, 0, 0
}
