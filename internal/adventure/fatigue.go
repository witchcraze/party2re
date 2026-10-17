package adventure

import "strings"

const (
	CrawlModeNormal = "normal"
	CrawlModeTry    = "try"
	CrawlModeCalm   = "calm"
	CrawlModeHard   = "hard"
)

// NormalizeCrawlMode normalizes a quest/crawl mode identifier, defaulting to CrawlModeNormal.
func NormalizeCrawlMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case CrawlModeCalm:
		return CrawlModeCalm
	case CrawlModeTry:
		return CrawlModeTry
	case CrawlModeHard:
		return CrawlModeHard
	default:
		return CrawlModeNormal
	}
}

// CalculateExitFatigue computes post-adventure exit fatigue (lib/_battle.cgi:1187-1188).
// Formula: add_tired = round * 3 + 1; halved with integer truncation (int(add_tired * 0.5)) when mode == "calm".
func CalculateExitFatigue(round int, mode string) int {
	if round < 0 {
		round = 0
	}
	addTired := round*3 + 1
	if NormalizeCrawlMode(mode) == CrawlModeCalm {
		addTired /= 2
	}
	return addTired
}

// SetMode configures the crawl mode (e.g. "normal", "calm", "try", "hard") on the session.
func (s *CrawlSession) SetMode(mode string) {
	s.Mode = NormalizeCrawlMode(mode)
}

// SetRound sets the current/exit floor (legacy $round) directly and aligns CurrentFloor
// for subsequent floor advances (lib/_battle.cgi:772, 811, 838).
func (s *CrawlSession) SetRound(round int) {
	if round < 0 {
		round = 0
	}
	s.Round = round
	s.CurrentFloor = round + 1
}

// AdvanceRoundIfBelow9 increments Round by 1 only when Round < 9 (tool 83 in lib/_battle.cgi:799).
func (s *CrawlSession) AdvanceRoundIfBelow9() {
	if s.Round < 9 {
		s.SetRound(s.Round + 1)
	}
}

// ExitFatigue returns the exit fatigue for the session's current Round and Mode.
func (s *CrawlSession) ExitFatigue() int {
	return CalculateExitFatigue(s.Round, s.Mode)
}

// EffectiveExitFatigue returns the exit fatigue to apply during post-battle settlement.
func (r DungeonCrawlResult) EffectiveExitFatigue(fallbackMode string) int {
	mode := r.Mode
	if mode == "" {
		mode = fallbackMode
	}
	if mode != "" || r.Round > 0 {
		return CalculateExitFatigue(r.Round, mode)
	}
	if r.ExitFatigue > 0 {
		return r.ExitFatigue
	}
	return CalculateExitFatigue(r.Round, CrawlModeNormal)
}
