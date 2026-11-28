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
