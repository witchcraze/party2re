package skill

import (
	"fmt"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

type callingZeroDraw struct{}

func (callingZeroDraw) Float64() float64 { return 0 }
func (callingZeroDraw) Intn(int) int     { return 0 }
func (callingZeroDraw) IntN(int) int     { return 0 }

// Independent source vectors: _skill.cgi at ff97fc5, complete calling tables
// 4033–5286,5355–5445,6311–6465,6908–7036. Stats are the level39, zero-draw
// results; source helpers independently floor each stat after the .9 factor.
var callingManifest = []struct {
	job       int
	stats     [5]int
	last      string
	cost      int
	ext       string
	extCost   int
	positions string
}{
	{59, [5]int{63, 108, 45, 27, 108}, "merge", 100, "ride", 15, "4033–4140"},
	{60, [5]int{144, 63, 162, 90, 63}, "merge", 100, "", 0, "4149–4239"},
	{61, [5]int{108, 108, 108, 54, 108}, "revive-party", 42, "", 0, "4248–4333"},
	{62, [5]int{81, 72, 108, 9, 162}, "rain", 37, "drain-barrage", 53, "4342–4440"},
	{63, [5]int{90, 90, 72, 45, 72}, "merge", 100, "duplicate", 0, "4449–4585"},
	{64, [5]int{90, 90, 45, 90, 90}, "merge", 100, "grave", 1, "4594–4718"},
	{65, [5]int{90, 54, 90, 54, 162}, "barrage", 40, "", 0, "4727–4811"},
	{66, [5]int{108, 99, 144, 81, 135}, "fallen-power", 50, "revive", 0, "4820–4931"},
	{67, [5]int{81, 108, 81, 45, 81}, "double-tail", 40, "", 0, "4940–5027"},
	{68, [5]int{72, 18, 270, 144, 54}, "party-heal", 50, "", 0, "5036–5128"},
	{69, [5]int{72, 18, 270, 144, 54}, "share-mp", 20, "small", 15, "5137–5286"},
	{71, [5]int{135, 63, 90, 45, 162}, "merge", 100, "", 0, "5355–5445"},
	{80, [5]int{63, 206, 126, 108, 97}, "distribution", 20, "", 0, "6311–6465"},
	{87, [5]int{162, 9, 162, 27, 315}, "venom", 58, "", 0, "6908–7036"},
}

func TestCallingManifestMetadataAndIntrinsicResults(t *testing.T) {
	count := 0
	for _, row := range callingManifest {
		_, c, d := rosterRegistryFixture()
		d.RegisterCallingJobs(&c)
		prefix := fmt.Sprintf("job-%d-", row.job)
		want := []corebattle.CallbackEntry{
			{ID: prefix + "summon", Name: "summon", RequiredSP: 10, MPCost: 30},
			{ID: prefix + "dismiss", Name: "dismiss", RequiredSP: 11},
			{ID: prefix + "attack", Name: "attack", RequiredSP: 12},
			{ID: prefix + "special", Name: "special", RequiredSP: 30, MPCost: 30},
			{ID: prefix + "defend", Name: "defend", RequiredSP: 100, MPCost: 10},
			{ID: prefix + row.last, Name: row.last, RequiredSP: 150, MPCost: row.cost},
		}
		if row.ext != "" {
			want = append(want, corebattle.CallbackEntry{ID: prefix + row.ext, Name: row.ext, RequiredSP: 150, MPCost: row.extCost, Extended: true})
		}
		if !reflect.DeepEqual(d.Invocations.JobTables[row.job].Variants, [][]corebattle.CallbackEntry{want}) {
			t.Fatal(row.job, row.positions, "metadata mismatch")
		}
		for _, e := range want {
			if !d.RosterSkills[e.ID] {
				continue // Separate body owners retain their complete table positions.
			}
			count++
			t.Run(e.ID, func(t *testing.T) {
				s, c, d := callingManifestFixture()
				d.RegisterCallingJobs(&c)
				frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "remaining"}}
				if e.Name != "summon" {
					addCallingManifestHelper(&s, &c, row.job)
					frame.TargetContext = c.TargetContext
				}
				ops, err := d.InvocationAction(&s, &frame, []string{e.ID}, noArithmeticDraws{t})
				if err != nil {
					t.Fatal(row.positions, err)
				}
				if err := s.ApplyInvocations(&frame, d.Invocations, ops, callingZeroDraw{}); err != nil {
					t.Fatal(err)
				}
				wantMP := 100
				if e.Name == "merge" {
					wantMP = 110
				}
				if frame.ActorID != "self" || s.Actors[0].MP != wantMP {
					t.Fatal("actor/cost context changed", s)
				}
				checkCallingIntrinsic(t, &s, &c, e.Name, row.job, row.stats)
				// Present summon and fallen-helper guards require no draws or ports.
				s, c, d = callingManifestFixture()
				d.RegisterCallingJobs(&c)
				addCallingManifestHelper(&s, &c, row.job)
				s.Actors[3].HP = 0
				frame.TargetContext = c.TargetContext
				before := append([]corebattle.LifetimeActor(nil), s.Actors...)
				if e.Name == "duplicate" {
					if err := d.Invocations.Entries[e.ID].Execute(&s, &frame, callingZeroDraw{}); err != nil || len(s.Actors) != 4 || s.Actors[0].HP != 100 {
						t.Fatal(s, err)
					}
				} else if err := d.Invocations.Entries[e.ID].Execute(&s, &frame, noArithmeticDraws{t}); err != nil {
					t.Fatal(err)
				}
				if e.Name != "duplicate" && !reflect.DeepEqual(s.Actors, before) {
					t.Fatal("fallen guard mutated state")
				}
			})
		}
	}
	if count != 78 {
		t.Fatal("manifest positions", count)
	}
}

func callingManifestFixture() (corebattle.LifetimeState, corebattle.RosterContext, BattleDefinitions) {
	s, c, d := rosterRegistryFixture()
	level := 39
	c.RequesterLevel = &level
	s.Actors[0].Attack, s.Actors[0].Defense, s.Actors[0].Agility = 100, 80, 60
	s.Actors[0].BaseStats = corebattle.BattleBaseStats{Attack: 100, Defense: 80, Agility: 60}
	s.Actors[0].MP, s.Actors[0].MaxMP = 100, 100
	s.Actors[1].HP, s.Actors[1].MaxHP = 10000, 10000
	s.Actors = append(s.Actors, corebattle.LifetimeActor{Participant: corebattle.Participant{ID: "fallen", TeamID: "party", MaxHP: 200, MP: 5, Gear: &corebattle.BattleGearStats{}}})
	c.PartyIDs = append(c.PartyIDs, "fallen")
	return s, c, d
}

func addCallingManifestHelper(s *corebattle.LifetimeState, c *corebattle.RosterContext, job int) {
	s.Actors = append(s.Actors, corebattle.LifetimeActor{Participant: corebattle.Participant{
		ID: "helper", TeamID: "party", HP: 100, MaxHP: 100, MP: 10, MaxMP: 10, Attack: 100, Defense: 100, Agility: 100,
		BaseStats: corebattle.BattleBaseStats{Attack: 100, Defense: 100, Agility: 100}, Gear: &corebattle.BattleGearStats{},
	}, RosterIdentity: fmt.Sprintf("calling-%d", job), Tension: 1, Hit: 95})
	c.PartyIDs = append(c.PartyIDs, "helper")
}

func checkCallingIntrinsic(t *testing.T, s *corebattle.LifetimeState, c *corebattle.RosterContext, name string, job int, stats [5]int) {
	t.Helper()
	wantDamage := 0
	h := &s.Actors[3]
	switch name {
	case "summon":
		if got := [5]int{h.HP, h.MP, h.Attack, h.Defense, h.Agility}; got != stats || h.ExperienceReward != 19 || h.GoldReward != 9 {
			t.Fatal(got, stats, h)
		}
	case "dismiss":
		if h.HP != 0 || !h.RosterDetached || h.TeamID != "npc" {
			t.Fatal(h)
		}
	case "attack":
		wantDamage = 67
	case "special":
		wantDamage = map[int]int{59: 67, 60: 76, 65: 67, 68: 135, 69: 135}[job]
	case "defend":
		if job == 87 {
			if h.SummonForm != 1 || h.HP != 270 || h.Temporary != "" {
				t.Fatal(h)
			}
		} else if h.Temporary != corebattle.TemporaryDefense {
			t.Fatal(h)
		}
	case "merge":
		if h.HP != 0 || s.Actors[0].HP != 150 || s.Actors[0].Attack != 200 || !h.RosterDetached {
			t.Fatal(s)
		}
	case "revive-party":
		if s.Actors[2].HP != 84 || s.Actors[2].Attack != 300 || s.Actors[2].Agility != 300 {
			t.Fatal(s)
		}
	case "rain":
		wantDamage = 153
	case "barrage":
		wantDamage = 270
	case "fallen-power":
		wantDamage = 468
	case "double-tail":
		wantDamage = 135
		if !c.AdditionalEffect {
			t.Fatal("missing additional-effect flag")
		}
	case "party-heal":
		if s.Actors[0].HP != 100 || s.Actors[2].HP != 0 {
			t.Fatal(s)
		}
	case "share-mp":
		wantDamage = 243
		if h.MP != 5 || s.Actors[2].MP != 0 {
			t.Fatal(s)
		}
	case "distribution":
		if s.Actors[0].Attack != 200 || h.Attack != 150 || s.Actors[0].HP != 100 {
			t.Fatal(s)
		}
	case "venom":
		wantDamage = 108
		if h.SummonForm != 1 || h.HP != 270 || s.Actors[1].Status != corebattle.StatusPoison {
			t.Fatal(s)
		}
	case "ride":
		wantDamage = 86
	case "duplicate":
		if len(s.Actors) != 5 || s.Actors[4].HP != 90 || s.Actors[4].MP != 9 || s.Actors[0].HP != 100 {
			t.Fatal(s)
		}
	case "small":
		if len(s.Actors) != 7 || s.Actors[4].HP != 9 || s.Actors[6].Attack != 9 {
			t.Fatal(s)
		}
	default:
		t.Fatal("missing intrinsic oracle", name)
	}
	if 10000-s.Actors[1].HP != wantDamage {
		t.Fatal(name, "damage", 10000-s.Actors[1].HP, "want", wantDamage)
	}
}
