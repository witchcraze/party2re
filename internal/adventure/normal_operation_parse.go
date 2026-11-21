package adventure

import (
	"regexp"
	"slices"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/random"
	"github.com/witchcraze/party2re/internal/core/skill"
)

// ParsedRunOperation carries dispatch and the frozen player entries. The
// consumer binds any remaining Validate and Execute functions when building the step.
type ParsedRunOperation struct {
	Kind   RunOperationKind
	First  ParsedRunAction
	Second *ParsedRunAction
}

type ParsedRunAction struct {
	Action, TargetID, Input string
	corebattle.PlayerAction
	Arguments  []string
	Invocation corebattle.CallbackInvocation
	Custom     bool
}

const legacyOperationPattern = "＠(.+?)(?:(?:[ 　])?(?:&gt;|>)(.+?)[ 　]|[ 　])"

var (
	legacyOperation       = regexp.MustCompile(legacyOperationPattern)
	legacyDoubleOperation = regexp.MustCompile(legacyOperationPattern + legacyOperationPattern)
)

// ParseNormalOperation binds a typed selection or legacy free text against the
// same ordered choices. It preserves omitted and wrong-side targets for the
// callback helpers. Neither parsing nor admission executes a callback.
func (r *NormalRun) ParseNormalOperation(cmd RunCommand, ctx NormalOperationContext) (ParsedRunOperation, error) {
	ops, err := r.normalOperations(cmd.ActorID, ctx)
	if err != nil {
		return ParsedRunOperation{}, err
	}
	return ops.parse(cmd)
}

func (ops *normalOperations) parse(cmd RunCommand) (ParsedRunOperation, error) {
	if cmd.Action != "" && cmd.Action != "comment" {
		var namedCustom *runOperationCandidate
		for _, key := range ops.order {
			c := ops.byKey[key]
			if c.Action == cmd.Action && c.Action != "br" {
				return ops.parseFirst(c, cmd.TargetID, cmd.Input)
			}
			if c.custom && c.Name == cmd.Action {
				namedCustom = &c
			}
		}
		if namedCustom != nil {
			return ops.parseFirst(*namedCustom, cmd.TargetID, cmd.Input)
		}
		return ParsedRunOperation{}, ErrRunUnavailable
	}

	input := cmd.Input + " "
	matches := legacyOperation.FindStringSubmatch(input)
	if ops.actor != nil && ops.actor.Extended == corebattle.ExtendedAdditional {
		if double := legacyDoubleOperation.FindStringSubmatch(input); double != nil {
			matches = double
		}
	}
	comment := ParsedRunOperation{Kind: RunOperationComment, First: ParsedRunAction{Action: "comment", Input: cmd.Input}}
	if len(matches) == 0 {
		return comment, nil
	}
	c, ok := ops.byKey[skill.OperationNameKey(matches[1])]
	if !ok || ops.banned(c) {
		return comment, nil
	}
	first, err := ops.bind(c, matches[2], "", true)
	if err != nil {
		return ParsedRunOperation{}, err
	}
	result := ParsedRunOperation{Kind: c.Kind, First: first}
	if c.Kind == RunOperationBattle && len(matches) == 5 {
		secondKey := skill.OperationNameKey(matches[3])
		second, listed := ops.byKey[secondKey]
		if listed && second.Kind == RunOperationBattle {
			action, err := ops.parseEntry(second, matches[4], "", true)
			if err != nil {
				return ParsedRunOperation{}, err
			}
			if action.Whitelisted && first.Whitelisted {
				if !ops.registered(second) {
					return ParsedRunOperation{}, ErrRunUnavailable
				}
				frame := ops.ctx.Callback
				frame.Arguments = slices.Clone(action.Arguments)
				frame.Command = action.EntryID
				if err := ops.state.CertifyInvocations(&frame, ops.ctx.Definitions.Invocations, []corebattle.CallbackInvocation{action.Invocation}, random.Default()); err != nil {
					return ParsedRunOperation{}, err
				}
			}
			action.Cost = second.binding.MPCost
			result.Second = &action
		} else if skill.IsSecondOperationKey(secondKey) {
			if first.Whitelisted {
				return ParsedRunOperation{}, ErrRunUnavailable
			}
			// The extended counter still sees a whitelisted name without an
			// eligible second callback (_battle.cgi:1084–1089).
			result.Second = &ParsedRunAction{PlayerAction: corebattle.PlayerAction{Whitelisted: true}}
		}
	}
	return result, nil
}

func (o *normalOperations) parseFirst(c runOperationCandidate, target, input string) (ParsedRunOperation, error) {
	first, err := o.bind(c, target, input, false)
	if err != nil {
		return ParsedRunOperation{}, err
	}
	return ParsedRunOperation{Kind: c.Kind, First: first}, nil
}

func (o *normalOperations) bind(c runOperationCandidate, target, input string, legacy bool) (ParsedRunAction, error) {
	action, err := o.parseEntry(c, target, input, legacy)
	if err != nil {
		return ParsedRunAction{}, err
	}
	if o.actor != nil && o.actor.HP <= 0 && !action.FallenSelf && c.Action != "exit" && c.Kind != RunOperationWhisper && c.Kind != RunOperationScreenshot {
		return ParsedRunAction{}, ErrUnconsciousCharacter
	}
	if c.Kind != RunOperationBattle {
		switch c.Action {
		case "kick":
			if action.TargetID != "" && (action.TargetID == o.actor.ID || !slices.Contains(o.run.Roster, action.TargetID)) {
				return ParsedRunAction{}, ErrRunForbidden
			}
		case "advance":
			if o.run.Floor == EXBossFloor && len(o.run.Stage.exTreasureItems()) == 0 || o.run.Floor == TreasureRoomFloor && (o.run.Stage.Ordinary == nil || len(o.run.Stage.Ordinary.EXRoster) == 0) {
				return ParsedRunAction{}, ErrRunUnavailable
			}
		case "chest":
			if !isTreasureFloor(o.run.Floor) || action.TargetID == "" {
				return ParsedRunAction{}, ErrNoUnopenedTreasure
			}
		}
		return action, nil
	}
	if o.banned(c) {
		return ParsedRunAction{}, ErrRunUnavailable
	}
	if action.EntryID == stockLevelAction {
		return action, nil
	}
	frame := o.ctx.Callback
	frame.Arguments = slices.Clone(action.Arguments)
	frame.Command, frame.Custom = action.EntryID, c.custom
	if frame.Assets != nil && action.EntryID == "job-50-entry-7" {
		assets := *frame.Assets
		assets.StoredItem, assets.StoredTarget = action.TargetID, action.Input
		frame.Assets = &assets
	}
	state := &o.state
	rng := random.Default()
	switch {
	case c.custom:
		_, _, err = o.ctx.Definitions.PlayerCustomAction(state, &frame, nil, rng)
	case c.Action == "attack":
		if frame.Damage == nil {
			return ParsedRunAction{}, ErrRunUnavailable
		}
		damage := *frame.Damage
		damage.TargetContext = frame.TargetContext
		err = state.ValidateNormalAttack(&damage, action.TargetID, rng)
	default:
		if !o.registered(c) {
			return ParsedRunAction{}, ErrRunUnavailable
		}
		err = state.ValidateInvocations(&frame, o.ctx.Definitions.Invocations, []corebattle.CallbackInvocation{action.Invocation}, rng)
	}
	if err != nil {
		return ParsedRunAction{}, err
	}
	switch action.EntryID {
	case "front", "back", "between-encounter-survival", "br", "job-66-revive":
		bindOrdinaryPlayerAction(&action, frame, o.ctx.Definitions.Invocations)
	}
	return action, nil
}

func (o *normalOperations) parseEntry(c runOperationCandidate, target, input string, legacy bool) (ParsedRunAction, error) {
	if takesSelector(c) {
		if legacy {
			input, target = target, ""
		} else if target != "" {
			return ParsedRunAction{}, ErrRunConflict
		}
	}
	entry := c.binding.Invocation.EntryID
	if entry == "" {
		entry = c.Action
	}
	if c.custom {
		entry = "custom"
	}
	scoped := c.Kind == RunOperationBattle || c.Action == "kick"
	if entry == "job-50-entry-7" || entry == "job-86-entry-0" || entry == "job-86-entry-3" || takesSelector(c) {
		scoped = false
	}
	if c.Action == "chest" {
		target = o.chestTargetID(target, legacy)
	} else if scoped && target != "" {
		resolved := o.participantID(target, legacy)
		if resolved == "" && entry == "job-78-entry-1" {
			resolved = o.catalogItemID(target, legacy)
		}
		if resolved == "" {
			return ParsedRunAction{}, ErrRunConflict
		}
		target = resolved
	}
	if input != "" && (entry == "job-86-entry-5" || entry == "job-50-entry-7" && !toolSelector(target)) {
		input = o.participantID(input, legacy)
		if input == "" {
			return ParsedRunAction{}, ErrRunConflict
		}
	}
	var args []string
	bookSelector := entry == "job-86-entry-0" || entry == "job-86-entry-3"
	if bookSelector && input != "" && target != "" {
		return ParsedRunAction{}, ErrRunConflict
	}
	if (takesSelector(c) || bookSelector) && input != "" {
		args = []string{input}
	} else if target != "" || input != "" {
		args = []string{target}
		if input != "" {
			args = append(args, input)
		}
	}
	return ParsedRunAction{
		Action: c.Action, TargetID: target, Input: input, Arguments: args, Invocation: c.binding.Invocation, Custom: c.custom,
		PlayerAction: corebattle.PlayerAction{
			EntryID: entry, Cost: c.MPCost, FallenSelf: entry == "job-66-revive",
			Whitelisted: !c.custom && skill.IsSecondOperationKey(c.key),
			Bow:         !c.custom && skill.IsBowOperationKey(c.key),
			MirrorShare: entry == "extension-163",
		},
	}, nil
}

func (o *normalOperations) participantID(target string, legacy bool) string {
	if findActor(o.state.Actors, target) != nil {
		return target
	}
	if legacy {
		for _, a := range o.state.Actors {
			if a.Name == target || (a.NPC && "@"+a.Name == target) {
				return a.ID
			}
		}
	}
	return ""
}

func (o *normalOperations) chestTargetID(target string, legacy bool) string {
	if !isTreasureFloor(o.run.Floor) || target == "" {
		return ""
	}
	for _, box := range o.run.Boxes {
		if !box.isUnopened() {
			continue
		}
		if box.ID == target {
			return box.ID
		}
		if legacy && (box.Name == target || "@"+box.Name == target) {
			return box.ID
		}
	}
	return ""
}

func (o *normalOperations) catalogItemID(target string, legacy bool) string {
	catalog, err := item.DefaultCatalog()
	if err != nil {
		return ""
	}
	var def item.Definition
	if legacy {
		def, err = catalog.FindByName(target)
	} else {
		def, err = catalog.FindByID(target)
	}
	if err != nil {
		return ""
	}
	return def.ID
}

func (o *normalOperations) registered(c runOperationCandidate) bool {
	entry, ok := o.ctx.Definitions.Invocations.Entries[c.binding.Invocation.EntryID]
	return ok && (entry.Invocation.Kind != "" || entry.Validate != nil && entry.Execute != nil)
}

func (o *normalOperations) banned(c runOperationCandidate) bool {
	return c.Kind == RunOperationBattle && o.run.Battle.SkillBans[c.binding.Invocation.EntryID] > 0
}

func takesSelector(c runOperationCandidate) bool {
	return toolSelector(c.heldTool)
}

func toolSelector(id string) bool {
	switch id {
	case "item-066", "item-067", "item-068", "item-069", "item-070", "item-071", "item-083", "item-104", "item-107", "item-150", "item-168", "item-184":
		return true
	}
	return false
}
