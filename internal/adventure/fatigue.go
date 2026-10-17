package adventure

import (
	"errors"
	"fmt"
	"strings"
)

const (
	CrawlModeNormal = "normal"
	CrawlModeTry    = "try"
	CrawlModeCalm   = "calm"
	CrawlModeHard   = "hard"
)

var (
	ErrInvalidCrawlMode = errors.New("invalid adventure crawl mode")
)

// ValidateCrawlMode validates that a crawl mode is one of the recognized modes.
// Empty string defaults to CrawlModeNormal ("normal").
func ValidateCrawlMode(mode string) (string, error) {
	trimmed := strings.ToLower(strings.TrimSpace(mode))
	if trimmed == "" || trimmed == CrawlModeNormal {
		return CrawlModeNormal, nil
	}
	switch trimmed {
	case CrawlModeCalm, CrawlModeTry, CrawlModeHard:
		return trimmed, nil
	default:
		return "", fmt.Errorf("%w: %s", ErrInvalidCrawlMode, trimmed)
	}
}

// CalculateExitFatigue computes post-adventure exit fatigue (lib/_battle.cgi:1187-1188).
// Formula: add_tired = round * 3 + 1; halved with integer truncation (int(add_tired * 0.5)) when mode == "calm".
func CalculateExitFatigue(round int, mode string) int {
	if round < 0 {
		round = 0
	}
	addTired := round*3 + 1
	if strings.ToLower(strings.TrimSpace(mode)) == CrawlModeCalm {
		addTired /= 2
	}
	return addTired
}

// SetMode configures the crawl mode (e.g. "normal", "calm", "try", "hard") on the session.
func (s *CrawlSession) SetMode(mode string) error {
	validated, err := ValidateCrawlMode(mode)
	if err != nil {
		return err
	}
	s.Mode = validated
	return nil
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
