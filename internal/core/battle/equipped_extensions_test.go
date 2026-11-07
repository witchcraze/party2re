package battle

import (
	"errors"
	"reflect"
	"testing"
)

func extensionFixture(items ...string) (LifetimeState, EquippedExtensionContext, *int) {
	s, base := damageFixture()
	level := 40
	base.RequesterLevel = &level
	base.GuaranteedHit = true
	for i := range s.Actors {
		s.Actors[i].MP = 100
		s.Actors[i].MaxMP = 100
		if s.Actors[i].HP > 0 {
			s.Actors[i].HP = 900
		}
	}
	s.actor("self").ItemDefinitionIDs = items
	return s, EquippedExtensionContext{DamageContext: base, Death: &DefeatContext{Rewards: &DefeatRewards{}}}, &level
}

func runExtension(t *testing.T, s *LifetimeState, c *EquippedExtensionContext, id string, draws ...float64) {
	t.Helper()
	rng := &targetRNG{t: t, draws: draws}
	if err := s.ApplyEquippedExtension(c, id, rng); err != nil {
		t.Fatal(id, err)
	}
	rng.done()
}

// Costs and source order are the generic identities' only table facts.
func TestEquippedExtensionManifest(t *testing.T) {
	want := map[string]int{"extension-165": 30, "extension-201": 30, "extension-163": 12, "extension-212": 5, "extension-247": 5, "extension-248": 2, "extension-146": 37, "extension-200": 10}
	entries := EquippedExtensionEntries()
	if len(entries) != len(want) || entries[0].ID != "extension-165" || entries[7].ID != "extension-200" {
		t.Fatal(entries)
	}
	for _, entry := range entries {
		if want[entry.ID] != entry.MPCost || entry.DefinitionID != "item-"+entry.ID[len("extension-"):] {
			t.Fatal(entry)
		}
	}
}

// Discovery: _skill.cgi:34,124–134,260–284. No SP or curse gate; item249 doubles.
func TestEquippedExtensionDiscoveryGates(t *testing.T) {
	rng := &targetRNG{t: t}
	s, c, _ := extensionFixture("item-212")
	s.actor("self").Abilities = []string{"cursed"}
	s.actor("self").Jobs = BattleJobs{}
	if entry, err := s.ValidateEquippedExtension(&c, "extension-212", rng); err != nil || entry.MPCost != 5 {
		t.Fatal(entry, err)
	}
	for name, change := range map[string]func(*LifetimeState, *EquippedExtensionContext){
		"not equipped": func(s *LifetimeState, _ *EquippedExtensionContext) { s.actor("self").ItemDefinitionIDs = nil },
		"no MP":        func(s *LifetimeState, _ *EquippedExtensionContext) { s.actor("self").MP = 4 },
		"NPC":          func(s *LifetimeState, _ *EquippedExtensionContext) { s.actor("self").NPC = true },
		"NPC actor":    func(_ *LifetimeState, c *EquippedExtensionContext) { c.ActorNPC = true },
	} {
		s, c, _ := extensionFixture("item-212")
		change(&s, &c)
		if _, err := s.ValidateEquippedExtension(&c, "extension-212", rng); !errors.Is(err, ErrUnsupportedEffect) {
			t.Fatal(name, err)
		}
	}
	s, c, _ = extensionFixture("item-212", "item-249")
	s.actor("self").MP = 9
	if _, err := s.ValidateEquippedExtension(&c, "extension-212", rng); !errors.Is(err, ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	s.actor("self").MP = 10
	if entry, err := s.ValidateEquippedExtension(&c, "extension-212", rng); err != nil || entry.MPCost != 10 {
		t.Fatal(entry, err)
	}
	if _, err := s.ValidateEquippedExtension(&c, "extension-244", rng); !errors.Is(err, ErrUnsupportedEffect) {
		t.Fatal(err)
	}
}

// Admission is pure on both outcomes, including unsupported later descendants.
func TestEquippedExtensionAdmissionLeavesStateUnchanged(t *testing.T) {
	for _, entry := range EquippedExtensionEntries() {
		for _, accepted := range []bool{false, true} {
			s, c, _ := extensionFixture(entry.DefinitionID)
			s.actor("self").Temporary = TemporaryReflectAttack
			s.actor("self").Status = StatusPhysicalSeal
			s.actor("self").Jobs.JobID = "job-26"
			c.Arguments = []string{"enemy"}
			if !accepted {
				c.Death = nil
				s.actor("enemy").HP = 1
			}
			before := reloadLifetime(t, s)
			_, err := s.ValidateEquippedExtension(&c, entry.ID, &targetRNG{t: t})
			if accepted && err != nil {
				t.Fatal(entry.ID, err)
			}
			if !reflect.DeepEqual(s, before) || !reflect.DeepEqual(c.Arguments, []string{"enemy"}) {
				t.Fatal(entry.ID, s)
			}
		}
	}
}

// _skill.cgi:322–342: old calling job first without fallback, exact living helper.
func TestExtension165Partner(t *testing.T) {
	helper := func(s *LifetimeState, job int, hp int) {
		s.Actors = append(s.Actors, LifetimeActor{Participant: Participant{ID: "h" + callingIdentity(job), TeamID: "party", HP: hp, MaxHP: 100, Attack: 10, Defense: 20, Agility: 30, Gear: &BattleGearStats{}}, RosterIdentity: callingIdentity(job), Tension: 1})
	}
	run := func(jobs BattleJobs, setup func(*LifetimeState), draws ...float64) (LifetimeState, int) {
		s, c, _ := extensionFixture("item-165")
		s.actor("self").Jobs = jobs
		s.actor("self").Attack = 100
		setup(&s)
		c.PartyIDs = nil
		for _, a := range s.Actors {
			if a.TeamID == s.actor("self").TeamID {
				c.PartyIDs = append(c.PartyIDs, a.ID)
			}
		}
		c.Arguments = []string{"enemy"}
		before := s.actor("enemy").HP
		runExtension(t, &s, &c, "extension-165", draws...)
		return s, before - s.actor("enemy").HP
	}
	noHelper := func(*LifetimeState) {}
	for name, jobs := range map[string]BattleJobs{"none": {JobID: "job-1"}, "current missing": {JobID: "job-59"}} {
		if _, hit := run(jobs, noHelper); hit != 0 {
			t.Fatal(name, hit)
		}
	}
	// A present previous helper wins; an absent one never falls back to current.
	if _, hit := run(BattleJobs{JobID: "job-60", OldJobID: "job-59"}, func(s *LifetimeState) { helper(s, 60, 50) }); hit != 0 {
		t.Fatal(hit)
	}
	if _, hit := run(BattleJobs{JobID: "job-59"}, func(s *LifetimeState) { helper(s, 59, 0) }); hit != 0 {
		t.Fatal("fallen helper", hit)
	}
	// Appearance-only matches belong to FindCallingHelper, not this exact lookup.
	if _, hit := run(BattleJobs{JobID: "job-59"}, func(s *LifetimeState) {
		s.actor("ally").Icon = nativeFormIcon("job-39", "m")
	}); hit != 0 {
		t.Fatal("appearance", hit)
	}
	// (100+stat)×.85×.5−(10×.3), floor of the lowest variance .9.
	for draw, want := range map[float64]int{0: 39, .34: 43, .67: 47} {
		_, hit := run(BattleJobs{JobID: "job-59"}, func(s *LifetimeState) {
			helper(s, 59, 50)
			s.actor("enemy").Defense = 10
		}, draw, 0)
		if hit != want {
			t.Fatal(draw, hit, want)
		}
	}
	_, hit := run(BattleJobs{JobID: "job-1", OldJobID: "job-59"}, func(s *LifetimeState) { helper(s, 59, 50) }, 0, 0)
	if hit == 0 {
		t.Fatal("previous job helper")
	}
}

// _skill.cgi:343–358 and the #1367 decision on the package cursor.
func TestExtension201GuardAndCursor(t *testing.T) {
	setup := func(agility int) (LifetimeState, EquippedExtensionContext) {
		s, c, _ := extensionFixture("item-201")
		s.actor("self").Agility, s.actor("self").BaseStats.Agility = agility, 100
		for i := range s.Actors {
			s.Actors[i].Gear = &BattleGearStats{WeaponWeight: 20, ArmorWeight: 10}
		}
		c.Arguments = []string{"enemy"}
		return s, c
	}
	// Equal to base is no effect, without draws or any cursor fact.
	s, c := setup(100)
	c.Cursor = TargetCursor{Source: "unknown", ActorID: "missing"}
	before := reloadLifetime(t, s)
	runExtension(t, &s, &c, "extension-201")
	if !reflect.DeepEqual(s, before) {
		t.Fatal(s)
	}
	// base+1 passes; an absent cursor is not self and the actor still resets.
	s, c = setup(101)
	ally := s.actor("ally").Agility
	before = reloadLifetime(t, s)
	runExtension(t, &s, &c, "extension-201", 0)
	if s.actor("self").Agility != 100 || s.actor("ally").Agility != ally || s.actor("enemy").HP >= before.actor("enemy").HP {
		t.Fatal(s)
	}
	for _, cursor := range []string{"self", "ally", "fallen"} {
		s, c = setup(150)
		c.Cursor = TargetCursor{Source: "item150", ActorID: cursor}
		runExtension(t, &s, &c, "extension-201", 0)
		// Self's write falls below base, so the later reset guard keeps it.
		if s.actor(cursor).Agility != 70 {
			t.Fatal(cursor, s.actor(cursor))
		}
	}
	for _, cursor := range []TargetCursor{{Source: "submitted", ActorID: "enemy"}, {Source: "item150", ActorID: "missing"}, {ActorID: "enemy"}, {Source: "item150"}} {
		s, c = setup(150)
		c.Cursor = cursor
		before = reloadLifetime(t, s)
		if _, err := s.ValidateEquippedExtension(&c, "extension-201", &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(s, before) {
			t.Fatal(cursor, err)
		}
	}
	// Purification's real item159 reset moves the cursor before the post-damage
	// write, through the shared owner rather than a stubbed hook.
	s, c = setup(150)
	c.Cursor = TargetCursor{Source: "item150", ActorID: "ally"}
	ally = s.actor("ally").Agility
	s.actor("self").Extended = ExtendedPurification
	s.actor("enemy").Gear.AccessoryID = "item-159"
	runExtension(t, &s, &c, "extension-201", .5, .5, .5, 0)
	if s.actor("enemy").Agility != 70 || s.actor("ally").Agility != ally || c.Cursor.ActorID != "enemy" {
		t.Fatal(c.Cursor, s)
	}
	if c.Reset != nil || c.Defeat != nil {
		t.Fatal("bound hooks must not outlive the operation")
	}
}

// _skill.cgi:359–374: reflection only, any recipient, count includes the fallen.
func TestExtension163MirrorShare(t *testing.T) {
	s, c, _ := extensionFixture("item-163")
	before := reloadLifetime(t, s)
	runExtension(t, &s, &c, "extension-163")
	if !reflect.DeepEqual(s, before) {
		t.Fatal("non-reflection", s)
	}
	for name, row := range map[string]struct {
		argument, recipient string
		count               float64
	}{
		"default self": {"", "self", 2},
		"unknown self": {"missing", "self", 2},
		"ally":         {"ally", "ally", 3},
		"enemy":        {"enemy", "enemy", 2},
	} {
		s, c, _ := extensionFixture("item-163")
		s.actor("self").Temporary = TemporaryReflectMagic
		s.actor("fallen").Temporary = TemporaryReflectMagic
		s.actor("self").HP, s.actor("ally").HP = 100, 100
		c.Arguments = []string{row.argument}
		// One variance per ordered member; the fallen member redirects to the actor.
		runExtension(t, &s, &c, "extension-163", 0, 0, 0)
		if s.actor(row.recipient).Temporary != TemporaryReflectMagic {
			t.Fatal(name, s.actor(row.recipient))
		}
		if got, want := s.actor("ally").HP, 100+int(row.count*50*.9); got != want {
			t.Fatal(name, got, want)
		}
	}
}

// _skill.cgi:375–385,401–412,449–460: exact counters and guards.
func TestExtendedStateExtensions(t *testing.T) {
	s, c, _ := extensionFixture("item-212")
	s.actor("self").Extended = ExtendedWarGod
	runExtension(t, &s, &c, "extension-212")
	if a := s.actor("self"); a.Extended != ExtendedInterception || a.ExtendedTurns != 7 {
		t.Fatal(a)
	}
	for draw, turns := range map[float64]int{0: 3, .9999: 7} {
		s, c, _ := extensionFixture("item-247")
		runExtension(t, &s, &c, "extension-247", draw)
		if a := s.actor("self"); a.Extended != ExtendedIncense || a.ExtendedTurns != turns {
			t.Fatal(draw, a)
		}
	}
	s, c, _ = extensionFixture("item-247")
	s.actor("self").HP = 0
	runExtension(t, &s, &c, "extension-247")
	if s.actor("self").Extended != "" {
		t.Fatal(s.actor("self"))
	}
	for draw, turns := range map[float64]int{0: 2, .9999: 4} {
		s, c, _ := extensionFixture("item-200")
		runExtension(t, &s, &c, "extension-200", draw)
		if a := s.actor("self"); a.Extended != ExtendedWarGod || a.ExtendedTurns != turns {
			t.Fatal(draw, a)
		}
	}
	s, c, _ = extensionFixture("item-200")
	s.actor("self").Extended, s.actor("self").ExtendedTurns = ExtendedWarGod, 1
	runExtension(t, &s, &c, "extension-200")
	if s.actor("self").ExtendedTurns != 1 {
		t.Fatal("same-state replaced", s.actor("self"))
	}
}

// _skill.cgi:435–448: magic seal gate, base 50..99, then regeneration on the first target.
func TestExtension146Regeneration(t *testing.T) {
	s, c, _ := extensionFixture("item-146")
	s.actor("self").Status = StatusMagicSeal
	c.Arguments = []string{"ally"}
	before := reloadLifetime(t, s)
	runExtension(t, &s, &c, "extension-146", .5)
	if !reflect.DeepEqual(s, before) {
		t.Fatal("sealed", s)
	}
	for draw, want := range map[float64]int{0: 187, .9999: 273} {
		s, c, _ := extensionFixture("item-146")
		s.actor("self").Status = StatusMagicSeal
		s.actor("ally").HP = 100
		c.Arguments = []string{"ally"}
		// Base 50 or 99, item146's ×1.8 in the shared helper, then variance .975.
		runExtension(t, &s, &c, "extension-146", 0, draw, .25)
		if got := s.actor("ally").HP; got != want || s.actor("ally").Temporary != TemporaryRegeneration || s.actor("self").Status != "" {
			t.Fatal(draw, got, want, s.actor("ally"))
		}
	}
	// A fallen submission falls back to the actor for both target and state.
	s, c, _ = extensionFixture("item-146")
	c.Arguments = []string{"fallen"}
	runExtension(t, &s, &c, "extension-146", 0, 0)
	if s.actor("self").Temporary != TemporaryRegeneration || s.actor("fallen").Temporary != "" {
		t.Fatal(s)
	}
}

// _skill.cgi:413–434; _data.cgi:2013: requester level, current job26 only, boundary.
func TestProjectileVariants(t *testing.T) {
	for _, row := range []struct {
		name         string
		tool         bool
		job, old     string
		draw, chance float64
		damage       int
		poisoned     bool
	}{
		// Level40 and AG100, direct, lowest variance .9, enemy immune to neither.
		{name: "extension other", job: "job-1", damage: 72},
		{name: "extension old only", job: "job-1", old: "job-26", damage: 72},
		{name: "extension job26 boundary miss", job: "job-26", damage: 139, chance: .5},
		{name: "extension job26 hit", job: "job-26", damage: 139, chance: .49, poisoned: true},
		{name: "extension high draw", job: "job-1", draw: .9999, damage: 89},
		{name: "tool other", tool: true, job: "job-1", damage: 144},
		{name: "tool job26 boundary miss", tool: true, job: "job-26", damage: 279, chance: .7},
		{name: "tool job26 hit", tool: true, job: "job-26", damage: 279, chance: .69, poisoned: true},
		{name: "tool high draw", tool: true, job: "job-1", draw: .9999, damage: 215},
	} {
		t.Run(row.name, func(t *testing.T) {
			s, c, _ := extensionFixture("item-248")
			a := s.actor("self")
			a.Jobs = BattleJobs{JobID: row.job, OldJobID: row.old}
			a.Agility = 100
			s.actor("enemy").HP = 900
			draws := []float64{row.draw, 0}
			if row.job == "job-26" {
				draws = append(draws, row.chance)
			}
			rng := &targetRNG{t: t, draws: draws}
			if row.tool {
				tool := OrdinaryToolContext{DamageContext: c.DamageContext, Death: c.Death, Arguments: []string{"enemy"}}
				if err := s.ApplyOrdinaryTool(&tool, "tool-248", rng); err != nil {
					t.Fatal(err)
				}
				if len(tool.Results) != 1 || !tool.Results[0].Consume {
					t.Fatal(tool.Results)
				}
			} else {
				c.Arguments = []string{"enemy"}
				if err := s.ApplyEquippedExtension(&c, "extension-248", rng); err != nil {
					t.Fatal(err)
				}
			}
			rng.done()
			if got := 900 - s.actor("enemy").HP; got != row.damage {
				t.Fatal(got, row.damage)
			}
			if got := s.actor("enemy").Status == StatusDeadlyPoison; got != row.poisoned {
				t.Fatal(s.actor("enemy").Status)
			}
		})
	}
}

// Requester level binds, not the actor; self default differs from raw shift.
func TestProjectileBindings(t *testing.T) {
	s, c, level := extensionFixture("item-248")
	*level = 10
	s.actor("self").Jobs.JobID = "job-1"
	s.actor("enemy").HP = 900
	c.Arguments = []string{"enemy"}
	runExtension(t, &s, &c, "extension-248", 0, 0)
	if got := 900 - s.actor("enemy").HP; got != 18 {
		t.Fatal(got)
	}
	// Actors have no level fact: an NPC actor's tool reads the requester's.
	s, c, level = extensionFixture()
	*level = 10
	s.actor("self").NPC = true
	s.actor("self").Jobs.JobID = "job-1"
	s.actor("enemy").HP = 900
	c.ActorNPC = true
	npcTool := OrdinaryToolContext{DamageContext: c.DamageContext, Death: c.Death, Arguments: []string{"enemy"}}
	if err := s.ApplyOrdinaryTool(&npcTool, "tool-248", &targetRNG{t: t, draws: []float64{0, 0}}); err != nil {
		t.Fatal(err)
	}
	if got := 900 - s.actor("enemy").HP; got != 36 {
		t.Fatal(got)
	}
	// Tool's omitted selector is self, so a confused actor keeps self: no member draw.
	s, c, _ = extensionFixture("item-248")
	s.actor("self").Status = StatusConfusion
	s.actor("self").Jobs.JobID = "job-1"
	tool := OrdinaryToolContext{DamageContext: c.DamageContext, Death: c.Death}
	rng := &targetRNG{t: t, draws: []float64{0, 0}}
	if err := s.ApplyOrdinaryTool(&tool, "tool-248", rng); err != nil {
		t.Fatal(err)
	}
	rng.done()
	// The extension's raw empty selector instead draws a member before keeping it.
	s, c, _ = extensionFixture("item-248")
	s.actor("self").Status = StatusConfusion
	s.actor("self").Jobs.JobID = "job-1"
	runExtension(t, &s, &c, "extension-248", 0, 0, 0)
	// Missing requester level and missing targets reject before any effect.
	s, c, _ = extensionFixture("item-248")
	c.RequesterLevel = nil
	if _, err := s.ValidateEquippedExtension(&c, "extension-248", &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	s, c, _ = extensionFixture()
	c.EnemyIDs = nil
	tool = OrdinaryToolContext{DamageContext: c.DamageContext, Death: c.Death, Arguments: []string{"enemy"}}
	for i := range s.Actors {
		if s.Actors[i].TeamID != s.actor("self").TeamID {
			tool.MemberOnlyIDs = append(tool.MemberOnlyIDs, s.Actors[i].ID)
		}
	}
	if err := s.ValidateOrdinaryTool(&tool, "tool-248", &targetRNG{t: t}); !errors.Is(err, ErrNoBattleTarget) {
		t.Fatal(err)
	}
}

// A lethal hit's descendants need the supplied owner, on the status tail's path too.
func TestProjectileRejectsMissingDefeatOwner(t *testing.T) {
	s, c, _ := extensionFixture("item-248")
	s.actor("self").Jobs.JobID = "job-26"
	s.actor("enemy").HP = 1
	c.Death = nil
	c.Arguments = []string{"enemy"}
	before := reloadLifetime(t, s)
	if err := s.ApplyEquippedExtension(&c, "extension-248", &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(s, before) {
		t.Fatal(err, s)
	}
	// A nonlethal hit reaches no owner, so its absence must not reject the entry.
	s.actor("enemy").HP = 900
	runExtension(t, &s, &c, "extension-248", 0, 0, .9)
	if s.actor("enemy").HP >= 900 {
		t.Fatal(s.actor("enemy"))
	}
}

// Results survive reload; the caller persists them instead of replaying callbacks.
func TestEquippedExtensionResultsSurviveReload(t *testing.T) {
	s, c, _ := extensionFixture("item-247", "item-146")
	runExtension(t, &s, &c, "extension-247", 0)
	c.Arguments = []string{"ally"}
	runExtension(t, &s, &c, "extension-146", 0, 0)
	reloaded := reloadLifetime(t, s)
	if a := reloaded.actor("self"); a.Extended != ExtendedIncense || a.ExtendedTurns != 3 {
		t.Fatal(a)
	}
	if reloaded.actor("ally").Temporary != TemporaryRegeneration {
		t.Fatal(reloaded.actor("ally"))
	}
}

// _skill.cgi:10186–10191: each of the three checks charges MP3 and zeroes symbols,
// so the numeric hit becomes magic×.8 while the status tail has nothing to apply.
func TestProjectileUnderCategoryConversionSeal(t *testing.T) {
	s, c, _ := extensionFixture("item-248")
	a := s.actor("self")
	a.Jobs.JobID, a.Agility, a.Abilities = "job-26", 100, []string{"seal_kotowari"}
	s.actor("enemy").HP = 900
	c.Arguments = []string{"enemy"}
	runExtension(t, &s, &c, "extension-248", 0, 0)
	if e := s.actor("enemy"); e.HP != 900-111 || e.Status != "" || s.actor("self").MP != 91 {
		t.Fatal(e, s.actor("self"))
	}
	if c.symbolicConversion {
		t.Fatal("conversion certification must not outlive the body")
	}
}
