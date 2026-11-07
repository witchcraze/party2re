package replay

import (
	"bytes"
	"encoding/json"
	"errors"
	"regexp"
	"unicode"
)

// SceneVersion identifies the persisted scene encoding; old scenes are never re-encoded.
const SceneVersion = 1

// MaxSourceBytes is the accumulated encoded size above which capture stops (docs/design/replay.md).
const MaxSourceBytes int64 = 2097152

const hiddenNumberAbove = 999

var ErrInvalidScene = errors.New("invalid replay scene")

var (
	presentationKey = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)
	displayColor    = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)
	markupTag       = regexp.MustCompile(`<[A-Za-z!/?]`)
)

// SceneCapture is the trusted producer's view of one eligible operation. It may hold raw facts
// (exact HP, defeated/substitute flags, whispers); Project removes everything outside the allowlist.
type SceneCapture struct {
	Title      string
	Floor      int
	Background string
	Field      string
	Overlays   []string
	ActorColor string
	Members    []MemberCapture
	Log        []LogEntry // newest first
}

type MemberCapture struct {
	Name          string
	Color         string
	Appearance    string
	Effects       []string
	HP            int
	MaxHP         int
	Substitute    bool
	State         string
	Temporary     string
	Extended      string
	ExtendedTurns int
	Tension       string
}

type LogEntry struct {
	Speaker string
	Color   string
	Text    string
	Date    string
	Whisper bool
}

// Scene is the versioned display projection persisted and returned to the archive owner.
type Scene struct {
	Version    int            `json:"v"`
	Title      string         `json:"title"`
	Floor      int            `json:"floor,omitempty"`
	Background string         `json:"background,omitempty"`
	Field      string         `json:"field,omitempty"`
	Overlays   []string       `json:"overlays,omitempty"`
	Members    []SceneMember  `json:"members"`
	Comments   []SceneComment `json:"comments"`
}

type SceneMember struct {
	Name          string   `json:"name"`
	Color         string   `json:"color"`
	Appearance    string   `json:"appearance,omitempty"`
	Effects       []string `json:"effects,omitempty"`
	Fallen        bool     `json:"fallen,omitempty"`
	HP            *int     `json:"hp,omitempty"`     // absent when displayed as hidden
	MaxHP         *int     `json:"max_hp,omitempty"` // absent when displayed as hidden
	HPPercent     int      `json:"hp_percent"`
	State         string   `json:"state,omitempty"`
	Temporary     string   `json:"temporary,omitempty"`
	Extended      string   `json:"extended,omitempty"`
	ExtendedTurns int      `json:"extended_turns,omitempty"`
	Tension       string   `json:"tension,omitempty"`
}

type SceneComment struct {
	Speaker string `json:"speaker"`
	Color   string `json:"color"`
	Text    string `json:"text"`
	Date    string `json:"date"`
}

// Project builds the allowlisted display scene from the capturing actor's perspective.
func Project(c SceneCapture) (Scene, error) {
	s := Scene{Version: SceneVersion, Title: c.Title, Floor: c.Floor, Background: c.Background, Field: c.Field,
		Overlays: c.Overlays, Members: []SceneMember{}, Comments: []SceneComment{}}
	if !text(c.Title) || !optionalKey(c.Background) || !optionalKey(c.Field) || !keys(c.Overlays) || !displayColor.MatchString(c.ActorColor) {
		return Scene{}, ErrInvalidScene
	}
	for _, m := range c.Members {
		if m.MaxHP <= 0 || !text(m.Name) || !displayColor.MatchString(m.Color) {
			return Scene{}, ErrInvalidScene
		}
		fallen := m.HP <= 0
		if fallen && (m.Color != c.ActorColor || m.Substitute) {
			continue // defeated opponents and substitutes leave the actor's view
		}
		sm := SceneMember{Name: m.Name, Color: m.Color, Fallen: fallen, HPPercent: m.HP * 100 / m.MaxHP,
			HP: shown(m.HP), MaxHP: shown(m.MaxHP)}
		if !fallen {
			if !optionalKey(m.Appearance) || !keys(m.Effects) {
				return Scene{}, ErrInvalidScene
			}
			sm.Appearance, sm.Effects = m.Appearance, m.Effects
		}
		if m.State != "" || m.Temporary != "" || m.Extended != "" {
			sm.State, sm.Temporary = m.State, m.Temporary
			if m.Extended != "" || m.ExtendedTurns > 0 {
				sm.Extended, sm.ExtendedTurns = m.Extended, m.ExtendedTurns
			}
		} else {
			sm.Tension = m.Tension
		}
		if !text(sm.State) || !text(sm.Temporary) || !text(sm.Extended) || !text(sm.Tension) {
			return Scene{}, ErrInvalidScene
		}
		s.Members = append(s.Members, sm)
	}
	for _, e := range c.Log {
		if len(s.Comments) == 3 {
			break
		}
		if e.Whisper {
			continue
		}
		if !text(e.Speaker) || !text(e.Text) || !text(e.Date) || !displayColor.MatchString(e.Color) {
			return Scene{}, ErrInvalidScene
		}
		s.Comments = append(s.Comments, SceneComment{Speaker: e.Speaker, Color: e.Color, Text: e.Text, Date: e.Date})
	}
	return s, nil
}

// EncodeScene returns the stable persisted UTF-8 encoding whose length counts toward MaxSourceBytes.
func EncodeScene(s Scene) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		return "", err
	}
	return string(bytes.TrimSuffix(buf.Bytes(), []byte("\n"))), nil
}

// DecodeStoredScene is the single read/publish check for a persisted scene: corrupt or
// unsupported encodings are source failures, never an empty or partial archive.
func DecodeStoredScene(raw string) (Scene, error) {
	s, err := DecodeJSON[Scene](raw)
	if err != nil || s.Version != SceneVersion {
		return Scene{}, ErrArchiveSourceUnavailable
	}
	return s, nil
}

func shown(n int) *int {
	if n > hiddenNumberAbove {
		return nil
	}
	return &n
}

func text(s string) bool {
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return !markupTag.MatchString(s)
}

func optionalKey(s string) bool { return s == "" || presentationKey.MatchString(s) }

func keys(list []string) bool {
	for _, k := range list {
		if !presentationKey.MatchString(k) {
			return false
		}
	}
	return true
}
