package battle

import (
	"regexp"

	"github.com/witchcraze/party2re/internal/core/random"
)

// ArithmeticContext is operation-local. Discovered is the caller's complete,
// ordered admitted name list, including unsupported descendants for gating.
// Defeat must already admit the complete ordered defeat/drop/reset closure;
// nil leaves reachable defeats unsupported. It shares the damage owner's hook.
// Divisor survives nested callbacks within an operation, never a saved turn.
type ArithmeticContext struct {
	TargetContext
	SubmittedText string
	Discovered    []ArithmeticEntry
	Divisor       int
	Defeat        func(string) error
}

type ArithmeticResult struct {
	CastFailed     bool
	TargetConsumed bool
	Applications   []EffectApplication
}

var arithmeticCommand = regexp.MustCompile(`＠(.+?)(?:(?: |　)?＠(×.+?)(?: |　)| |　)`)

func arithmeticDivisor(id string) int {
	switch id {
	case "job-72-divisor-3":
		return 3
	case "job-31-gravity", "job-72-divisor-4":
		return 4
	case "job-31-death", "job-72-divisor-5":
		return 5
	}
	return 0
}

func arithmeticLeaf(id string) bool {
	switch id {
	case "job-72-damage", "job-72-reflect", "job-72-heal", "job-72-revive", "job-72-death":
		return true
	}
	return false
}

// ArithmeticSelection uses the submitted text and last-write discovered name
// context (_skill.cgi:35–104,260–309,5450–5581). Positional random/first-match
// callers instead pass their captured concrete ID without doing this lookup.
// An empty callback is an accepted failed cast, not an unsupported operation.
func (c *ArithmeticContext) ArithmeticSelection(id string) (string, int, error) {
	if c == nil {
		return "", 0, ErrInvalidRequest
	}
	divisor := arithmeticDivisor(id)
	switch id {
	case "job-31-gravity", "job-31-death", "job-31-reverse":
		return id, divisor, nil
	case "job-72-divisor-3", "job-72-divisor-4", "job-72-divisor-5":
		match := arithmeticCommand.FindStringSubmatch(c.SubmittedText)
		if len(match) == 0 || match[2] == "" {
			return "", divisor, nil
		}
		selected := ""
		found := false
		for _, entry := range c.Discovered {
			if entry.Name == match[2] {
				selected = entry.ID
				found = true
			}
		}
		if found && !arithmeticLeaf(selected) {
			return "", 0, ErrUnsupportedEffect
		}
		return selected, divisor, nil
	default:
		if !arithmeticLeaf(id) {
			return "", 0, ErrUnsupportedEffect
		}
		if c.Divisor == 0 {
			return "", 0, nil
		}
		if c.Divisor < 3 || c.Divisor > 5 {
			return "", 0, ErrUnsupportedEffect
		}
		return id, c.Divisor, nil
	}
}

func arithmeticEligible(a *LifetimeActor, id string, divisor int) bool {
	return a.MP != 0 && a.MP%divisor == 0 &&
		(a.HP > 0) == (id != "job-72-revive") &&
		((id != "job-31-gravity" && id != "job-31-death" && id != "job-72-death") || a.HP <= 999)
}

// ValidateArithmetic is shared by discovery/submission and never draws or
// changes state. Formulas below bypass spell target/modifier helpers entirely.
func (s *LifetimeState) ValidateArithmetic(c *ArithmeticContext, id, targetID string, rng random.Generator) error {
	callback, divisor, err := c.ArithmeticSelection(id)
	if err != nil {
		return err
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	if callback == "job-31-reverse" {
		candidates := []TargetEffect{{TargetID: targetID}}
		if s.actor(targetID) == nil {
			e := TargetEffect{Payload: CallbackPayload{Symbol: "digit_reversal"}}
			if err := s.validatePartyTarget(c.TargetContext, e, rng); err != nil {
				return err
			}
			candidates = s.partyTargetCandidates(c.TargetContext, e, nil)
		}
		for _, candidate := range candidates {
			// Negative textual HP is not a decimal-digit reversal contract.
			hp := s.actor(candidate.TargetID).HP
			if hp < 0 && hp > -100 {
				return ErrUnsupportedEffect
			}
		}
		return nil
	}
	if callback == "" {
		return nil
	}
	for i := range s.Actors {
		a := &s.Actors[i]
		if !arithmeticEligible(a, callback, divisor) {
			continue
		}
		lethal := callback == "job-31-death" || callback == "job-72-death"
		if callback == "job-72-damage" {
			maximum := 311
			if a.BaseStats.Defense > 999 {
				maximum = 2
			}
			lethal = a.HP <= maximum
		}
		if lethal && c.Defeat == nil {
			return ErrUnsupportedEffect
		}
	}
	return nil
}

// ApplyArithmetic reconstructs _skill.cgi:2653–2713,5450–5581. Ordinary
// admission/costs, random MP restoration and command receipts belong to the
// invoking owner. No ordinary damage/heal/status counter is incremented.
func (s *LifetimeState) ApplyArithmetic(c *ArithmeticContext, id, targetID string, rng random.Generator) (ArithmeticResult, error) {
	r := ArithmeticResult{}
	if err := s.ValidateArithmetic(c, id, targetID, rng); err != nil {
		return r, err
	}
	allowed, err := s.actor(c.ActorID).CheckCategorySeal(CategoryMagic, rng)
	if err != nil || !allowed {
		return r, err
	}
	callback, divisor, err := c.ArithmeticSelection(id)
	if err != nil {
		return r, err
	}
	if callback == "" {
		r.CastFailed = true
		return r, nil
	}
	if callback != id {
		c.Divisor = divisor
		allowed, err = s.actor(c.ActorID).CheckCategorySeal(CategoryMagic, rng)
		if err != nil || !allowed {
			return r, err
		}
	}
	if callback == "job-31-reverse" {
		return s.reverseHP(c, targetID, rng)
	}
	// Snapshot order, then re-read eligibility after every defeat/reset hook.
	ids := make([]string, len(s.Actors))
	for i, a := range s.Actors {
		ids[i] = a.ID
	}
	for _, targetID := range ids {
		a := s.actor(targetID)
		if a == nil || !arithmeticEligible(a, callback, divisor) {
			continue
		}
		before, amount := a.HP, 0
		beforeTemporary := a.Temporary
		switch callback {
		case "job-31-death", "job-72-death":
			a.HP = 0
		case "job-72-damage":
			amount = int(260 * (.9 + rng.Float64()*.3))
			if a.BaseStats.Defense > 999 {
				amount = 1 + int(rng.Float64()*2)
			}
			a.HP = max(0, a.HP-amount)
		case "job-72-heal":
			amount = int(500 * (.9 + rng.Float64()*.3))
			a.HP = min(a.MaxHP, a.HP+amount)
		default:
			applyArithmeticValue(a, callback)
		}
		if a.HP == 0 && (callback == "job-31-death" || callback == "job-72-death" || callback == "job-72-damage") {
			if err := c.Defeat(targetID); err != nil {
				return r, err
			}
			a = s.actor(targetID)
		}
		after := 0
		if a != nil {
			after = a.HP
		}
		application := arithmeticApplication(targetID, before, after, amount)
		if callback == "job-72-reflect" {
			application.Applied = beforeTemporary != a.Temporary
		}
		r.Applications = append(r.Applications, application)
	}
	return r, nil
}

func arithmeticApplication(id string, before, after, amount int) EffectApplication {
	return EffectApplication{TargetResolution: TargetResolution{TargetEffect: TargetEffect{TargetID: id}, Resolved: true}, Returned: true, Applied: after != before, Amount: amount, Delta: after - before}
}

func (s *LifetimeState) reverseHP(c *ArithmeticContext, targetID string, rng random.Generator) (ArithmeticResult, error) {
	r := ArithmeticResult{TargetConsumed: true}
	if s.actor(targetID) == nil {
		target, err := s.ResolvePartyTarget(c.TargetContext, TargetEffect{Payload: CallbackPayload{Symbol: "digit_reversal"}}, rng)
		if err != nil || !target.Resolved {
			return r, err
		}
		targetID = target.TargetID
	}
	a := s.actor(targetID)
	before := a.HP
	if !reverseArithmeticHP(a) {
		return r, nil
	}
	r.Applications = []EffectApplication{arithmeticApplication(targetID, before, a.HP, 0)}
	return r, nil
}

func reverseArithmeticHP(a *LifetimeActor) bool {
	before := a.HP
	if before >= 10 && before < 100 {
		a.HP = before%10*10 + before/10
	} else if before >= 100 && before < 1000 {
		a.HP = before%10*100 + before/10%10*10 + before/100
	} else {
		return false
	}
	return true
}

func applyArithmeticValue(a *LifetimeActor, id string) {
	switch id {
	case "job-31-gravity":
		a.HP = max(1, a.HP/4)
	case "job-72-reflect":
		a.Temporary = TemporaryReflectMagic
	case "job-72-revive":
		a.HP = a.MaxHP
	}
}
