package adventure

import (
	"errors"
	"maps"
	"slices"
	"strconv"
	"strings"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/progression"
	"github.com/witchcraze/party2re/internal/core/skill"
)

type RunOperationKind string

const (
	RunOperationBattle     RunOperationKind = "battle"
	RunOperationRoom       RunOperationKind = "room"
	RunOperationComment    RunOperationKind = "comment"
	RunOperationWhisper    RunOperationKind = "whisper"
	RunOperationScreenshot RunOperationKind = "screenshot"
)

// RunOperationChoice describes eligibility in the supplied input and port
// context. A gated choice may be shown but cannot be submitted as executable.
type RunOperationChoice struct {
	Action, Name string
	Kind         RunOperationKind
	MPCost       int
	Gated        bool
}

// NormalOperationContext supplies operation-local battle ports. The requester
// is read separately from the temporary battle actor for stock and appearance.
type NormalOperationContext struct {
	Definitions skill.BattleDefinitions
	Callback    corebattle.CallbackContext
	Requester   *corecharacter.Character
	// Arithmetic receives the submitted text and the discovered names that
	// job72 selectors look up (_skill.cgi:35–104,260–309).
	Arithmetic *corebattle.ArithmeticContext
}

type runOperationCandidate struct {
	RunOperationChoice
	binding  corebattle.CallbackBinding
	key      string
	heldTool string
	custom   bool
}

type normalOperations struct {
	run      *NormalRun
	state    corebattle.LifetimeState
	actor    *corebattle.LifetimeActor
	ctx      NormalOperationContext
	order    []string
	byKey    map[string]runOperationCandidate
	treasure bool
}

var separatorKey = skill.OperationNameKey("br")

// DiscoverNormalOperations uses saved encounter membership and the same
// argument-dependent admission as parsing. Authorization belongs to the caller.
func (r *NormalRun) DiscoverNormalOperations(cmd RunCommand, ctx NormalOperationContext) ([]RunOperationChoice, error) {
	ops, err := r.normalOperations(cmd, ctx)
	if err != nil {
		return nil, err
	}
	choices := make([]RunOperationChoice, 0, len(ops.order))
	checked := make(map[string]RunOperationChoice, len(ops.byKey))
	for _, key := range ops.order {
		if key == separatorKey {
			continue
		}
		choice, exists := checked[key]
		if !exists {
			candidate := ops.byKey[key]
			choice = candidate.RunOperationChoice
			_, err := ops.bind(candidate, cmd.TargetID, cmd.Input, false)
			choice.Gated = err != nil
			checked[key] = choice
		}
		choices = append(choices, choice)
	}
	return choices, nil
}

func (r *NormalRun) normalOperations(cmd RunCommand, ctx NormalOperationContext) (*normalOperations, error) {
	actorID := cmd.ActorID
	if r == nil || actorID == "" || r.TimedOut {
		return nil, ErrRunConflict
	}
	ops := &normalOperations{run: r, actor: r.actor(actorID), ctx: ctx, byKey: make(map[string]runOperationCandidate)}
	if !slices.Contains(r.Roster, actorID) {
		if ops.actor != nil {
			return nil, ErrRunForbidden
		}
		ops.addSpecial("whisper", "ささやき", RunOperationWhisper)
		ops.addSpecial("exit", "にげる", RunOperationRoom)
		ops.addSpecial("screenshot", "すくしょ", RunOperationScreenshot)
		return ops, nil
	}
	a := ops.actor
	if a == nil || a.NPC {
		return nil, ErrRunForbidden
	}

	state, frame, inspection, err := r.ProjectCombat(actorID)
	if err != nil {
		return nil, err
	}
	ops.state = state
	ops.actor = findActor(ops.state.Actors, actorID)
	a = ops.actor
	ops.ctx.Callback.TargetContext = frame
	if len(ops.ctx.Callback.Inspection) == 0 {
		ops.ctx.Callback.Inspection = inspection
	} else {
		merged := maps.Clone(inspection)
		for id, content := range ops.ctx.Callback.Inspection {
			merged[id] = content
		}
		ops.ctx.Callback.Inspection = merged
	}

	cursed := slices.Contains(a.Abilities, "cursed")
	ops.treasure = slices.ContainsFunc(r.Boxes, TreasureBox.isUnopened)
	enemies := len(frame.EnemyIDs) > 0
	combat := r.Floor > 0 && enemies
	jobs := a.Jobs
	if combat {
		if !cursed {
			ops.addJob(jobs.JobID, jobs.SP, false)
			ops.addJob(jobs.OldJobID, jobs.OldSP, true)
			if jobs.SP != 0 || jobs.OldSP != 0 {
				ops.addBattle("br", "br", "br", "br")
			}
		}
		ops.addBattle("attack", "attack", "こうげき", "")
		ops.addBattle("defend", "defend", "ぼうぎょ", "job-0-entry-4")
		ops.addBattle("tension", "tension", "てんしょん", "job-0-entry-3")
		ops.addGear()
	}
	if err := ops.addHeldTool(); err != nil {
		return nil, err
	}
	if len(frame.PartyIDs) > 1 {
		ops.addBattle("front", "front", "まえ", "front")
		ops.addBattle("back", "back", "うしろ", "back")
	}
	ops.addSpecial("whisper", "ささやき", RunOperationWhisper)
	ops.addSpecial("exit", "にげる", RunOperationRoom)
	ops.addSpecial("screenshot", "すくしょ", RunOperationScreenshot)
	if r.Floor == 0 {
		ops.addSpecial("invite", "さそう", RunOperationRoom)
		if r.LeaderID == actorID {
			ops.addSpecial("kick", "きっく", RunOperationRoom)
		}
	}
	if !cursed {
		ops.addBetweenEncounter()
		if enemies && a.Gear != nil && a.Gear.WeaponID != "" {
			for i, entry := range corebattle.WeaponSealEntries() {
				if a.WeaponSeal == i+7 {
					ops.addListed(entry, jobs.OldSP)
				}
			}
		}
	}
	if !r.livingEnemies() && (r.Floor != 0 || r.LeaderID == actorID) && (r.Floor < TreasureRoomFloor || r.HasEX() && r.Floor <= EXBossFloor) {
		ops.addSpecial("advance", "すすむ", RunOperationRoom)
	}
	if isTreasureFloor(r.Floor) {
		ops.addSpecial("chest", "しらべる", RunOperationRoom)
	}
	if enemies && !cursed {
		ops.addCustom()
	}
	if p := ctx.Requester; p != nil && !enemies {
		if exp, err := progression.ExperienceForNextLevelWithMax(p.Level, progression.MaxLevelForCharacter(p)); err == nil && p.Experience >= exp {
			ops.addBattle(stockLevelAction, stockLevelAction, "すとっく", stockLevelAction)
		}
	}
	if c := ctx.Arithmetic; c != nil {
		// Names are source keys; a repeated key resolves to its last candidate.
		c.SubmittedText, c.Discovered = cmd.Input+" ", make([]corebattle.ArithmeticEntry, 0, len(ops.order))
		for _, key := range ops.order {
			c.Discovered = append(c.Discovered, corebattle.ArithmeticEntry{ID: ops.byKey[key].binding.ID, Name: key})
		}
	}
	return ops, nil
}

func (o *normalOperations) add(c runOperationCandidate) {
	o.order = append(o.order, c.key)
	o.byKey[c.key] = c
}

func (o *normalOperations) addSpecial(action, source string, kind RunOperationKind) {
	o.add(runOperationCandidate{RunOperationChoice: RunOperationChoice{Action: action, Name: action, Kind: kind}, key: skill.OperationNameKey(source)})
}

func (o *normalOperations) addBattle(action, name, source, entry string) {
	c := runOperationCandidate{RunOperationChoice: RunOperationChoice{Action: action, Name: name, Kind: RunOperationBattle}, key: skill.OperationNameKey(source)}
	if entry != "" {
		c.binding.Invocation = corebattle.CallbackInvocation{Kind: "entry", EntryID: entry}
	}
	o.add(c)
}

func (o *normalOperations) addJob(id string, sp int, previous bool) {
	if sp == 0 {
		return
	}
	entries, err := o.ctx.Definitions.PlayerCallbackTable(&o.state, &o.ctx.Callback, id)
	if err != nil {
		// An unavailable table must be visible even while its metadata cannot
		// be constructed. It is neither an executable choice nor a random pool.
		o.addBattle(id, id, id, id)
		return
	}
	for _, entry := range entries {
		if entry.ID == "job-73-gear-current" && previous || entry.ID == "job-73-gear-previous" && !previous {
			continue
		}
		if entry.Extended && !o.extendedJob(id) {
			continue
		}
		o.addBinding(entry.CallbackBinding, sp)
	}
}

func (o *normalOperations) extendedJob(id string) bool {
	a := o.actor
	if id == "job-73" {
		return false
	}
	if a.HasExtendedSkillAmulet() {
		return true
	}
	switch id {
	case "job-06", "job-15", "job-16", "job-19", "job-29", "job-30", "job-31", "job-51":
		return slices.Contains(a.ItemDefinitionIDs, "item-249")
	case "job-01", "job-21", "job-47", "job-52":
		return slices.Contains(a.ItemDefinitionIDs, "item-181")
	case "job-26":
		return slices.Contains(a.ItemDefinitionIDs, "item-248")
	}
	return false
}

func (o *normalOperations) addListed(entry corebattle.CallbackEntry, sp int) {
	o.addBinding(corebattle.CallbackBinding{
		CallbackEntry: entry,
		Invocation:    corebattle.CallbackInvocation{Kind: "entry", EntryID: entry.ID},
	}, sp)
}

func (o *normalOperations) addBinding(binding corebattle.CallbackBinding, sp int) {
	entry := binding.CallbackEntry
	if entry.RequiredSP > sp {
		return
	}
	burn := o.ctx.Requester != nil && o.actor.LateBreathSongBurn(o.run.Battle.Fields.Current(), o.ctx.Requester.Gender)
	key := skill.BindingSourceKey(binding, burn)
	if key == "" {
		key = "unavailable:" + entry.ID
	}
	cost := entry.MPCost
	if slices.Contains(o.actor.ItemDefinitionIDs, "item-249") {
		if skill.IsMagicianOperationKey(key) {
			cost /= 2
		} else {
			cost *= 2
		}
	}
	if cost > o.actor.MP {
		return
	}
	c := runOperationCandidate{
		RunOperationChoice: RunOperationChoice{Action: entry.ID, Name: entry.Name, Kind: RunOperationBattle, MPCost: cost},
		binding:            binding,
		key:                key,
	}
	if skill.IsCallOperationKey(key) && slices.Contains(o.actor.ItemDefinitionIDs, "item-165") {
		c.MPCost = 0
	}
	o.add(c)
}

func (o *normalOperations) addGear() {
	a := o.actor
	for _, id := range a.ItemDefinitionIDs {
		if gear, ok := skill.OperationGear(id); ok {
			o.addListed(corebattle.CallbackEntry{ID: gear.ID, Name: gear.ID, MPCost: gear.MP}, a.Jobs.OldSP)
		}
	}
}

func (o *normalOperations) addHeldTool() error {
	var held *string
	if assets := o.ctx.Callback.Assets; assets != nil {
		held = assets.HeldToolID
	}
	if held == nil {
		held = o.actor.HeldToolID
	}
	if held == nil || *held == "" {
		return nil
	}
	catalog, err := item.DefaultCatalog()
	if err != nil {
		return err
	}
	def, err := catalog.FindByID(*held)
	if errors.Is(err, item.ErrDefinitionNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if !def.UsageCategory.IsUsableInCombatCommand() {
		return nil
	}
	n, err := strconv.Atoi(strings.TrimPrefix(*held, "item-"))
	if err != nil {
		return err
	}
	key := skill.OperationNameKey("どうぐ")
	o.addBattle("tool", "tool", "どうぐ", "tool-"+strconv.Itoa(n))
	candidate := o.byKey[key]
	candidate.heldTool = *held
	o.byKey[key] = candidate
	return nil
}

func (o *normalOperations) addBetweenEncounter() {
	if len(o.ctx.Callback.EnemyIDs) != 0 || o.treasure {
		return
	}
	a := o.actor
	j := a.Jobs
	if o.run.Floor > 0 && (j.JobID == "job-78" && j.SP >= 55 || j.OldJobID == "job-78" && j.OldSP >= 55) {
		o.addListed(corebattle.CallbackEntry{ID: "between-encounter-survival", Name: "survival", MPCost: 10}, j.OldSP)
	}
	if form, ok := a.BetweenEncounterForm(o.run.Floor, 0); ok {
		o.addListed(corebattle.CallbackEntry{ID: form.ID, Name: form.Name, MPCost: form.MPCost}, j.OldSP)
	}
	if !slices.Contains(a.ItemDefinitionIDs, "item-165") {
		return
	}
	var entries []skill.PlayerCallback
	for _, id := range []string{j.JobID, j.OldJobID} {
		n, _ := strconv.Atoi(strings.TrimPrefix(id, "job-"))
		if len(corebattle.RosterEntries(n)) == 0 {
			continue
		}
		table, err := o.ctx.Definitions.PlayerCallbackTable(&o.state, &o.ctx.Callback, id)
		if err != nil || len(table) < 2 {
			o.addBattle(id, id, id, id)
			continue
		}
		entries = append(entries, table[:2]...)
	}
	for _, entry := range entries {
		o.addBinding(entry.CallbackBinding, j.OldSP)
	}
}

func (o *normalOperations) addCustom() {
	a := o.actor
	if a.Custom == nil || a.Custom.Name == "" || a.CMP < a.Custom.Cost || a.Custom.Gems == [3]string{} {
		return
	}
	name := a.Custom.Name
	key := skill.OperationNameKey(name)
	if _, exists := o.byKey[key]; exists {
		return
	}
	o.add(runOperationCandidate{RunOperationChoice: RunOperationChoice{Action: "custom", Name: name, Kind: RunOperationBattle}, key: key, custom: true})
}
